package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func TestGitHubDeliveryRetryLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newIsolatedPostgresTestStore(t)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project := createPostgresDeliveryTestProject(t, ctx, st, "postgres-project-retries@example.com")

	claim, err := st.ClaimGitHubDelivery(ctx, project.ID, "delivery-1", "pull_request")
	if err != nil || claim != store.GitHubDeliveryClaimed {
		t.Fatalf("expected first delivery claim, claim=%q err=%v", claim, err)
	}
	claim, err = st.ClaimGitHubDelivery(ctx, project.ID, "delivery-1", "pull_request")
	if err != nil || claim != store.GitHubDeliveryProcessing {
		t.Fatalf("expected processing delivery state, claim=%q err=%v", claim, err)
	}
	if err := st.FailGitHubDelivery(ctx, project.ID, "delivery-1"); err != nil {
		t.Fatal(err)
	}
	var failedStatus string
	if err := st.db.QueryRowContext(ctx,
		`SELECT status FROM github_deliveries WHERE project_id = $1 AND delivery_id = $2`,
		project.ID, "delivery-1",
	).Scan(&failedStatus); err != nil {
		t.Fatal(err)
	}
	if failedStatus != "failed" {
		t.Fatalf("expected failed delivery state, got %q", failedStatus)
	}
	claim, err = st.ClaimGitHubDelivery(ctx, project.ID, "delivery-1", "pull_request")
	if err != nil || claim != store.GitHubDeliveryClaimed {
		t.Fatalf("expected failed delivery to be claimable, claim=%q err=%v", claim, err)
	}
	staleAt := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano)
	if _, err := st.db.ExecContext(ctx,
		`UPDATE github_deliveries SET updated_at = $1 WHERE project_id = $2 AND delivery_id = $3`,
		staleAt, project.ID, "delivery-1",
	); err != nil {
		t.Fatal(err)
	}
	claim, err = st.ClaimGitHubDelivery(ctx, project.ID, "delivery-1", "pull_request")
	if err != nil || claim != store.GitHubDeliveryClaimed {
		t.Fatalf("expected stale processing delivery to be claimable, claim=%q err=%v", claim, err)
	}
	if err := st.CompleteGitHubDelivery(ctx, project.ID, "delivery-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.FailGitHubDelivery(ctx, project.ID, "delivery-1"); err != nil {
		t.Fatal(err)
	}
	claim, err = st.ClaimGitHubDelivery(ctx, project.ID, "delivery-1", "pull_request")
	if err != nil || claim != store.GitHubDeliveryComplete {
		t.Fatalf("expected completed delivery state, claim=%q err=%v", claim, err)
	}
}

func TestGitHubDeliveryRetryMigrationUpgradesExistingRows(t *testing.T) {
	ctx := context.Background()
	st := newIsolatedPostgresTestStore(t)
	applyPostgresMigrationsBefore(t, ctx, st, "0033_github_delivery_retries.sql")
	owner, err := st.EnsureBootstrap(ctx, "Upgrade Owner", "postgres-project-upgrade@example.com")
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := st.ListChannels(ctx, workspaces[0].ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	const projectID = "prj_upgrade_delivery"
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO projects (
			id, workspace_id, name, slug, description, channel_id,
			integration_user_id, webhook_secret, created_by, created_at
		) VALUES ($1, $2, 'Upgrade', 'upgrade', '', $3, $4, 'test-secret', $4, $5)
	`, projectID, workspaces[0].ID, channels[0].ID, owner.ID, now()); err != nil {
		t.Fatal(err)
	}

	const processingAt = "2026-01-02T03:04:05Z"
	const completedAt = "2026-01-02T04:05:06Z"
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO github_deliveries (project_id, delivery_id, event_type, status, created_at, completed_at)
		VALUES ($1, 'old-processing', 'issues', 'processing', $2, NULL),
		       ($1, 'old-complete', 'pull_request', 'complete', $2, $3)
	`, projectID, processingAt, completedAt); err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		deliveryID, wantStatus, wantUpdated string
	}{
		{deliveryID: "old-processing", wantStatus: "processing", wantUpdated: processingAt},
		{deliveryID: "old-complete", wantStatus: "complete", wantUpdated: completedAt},
	} {
		var status, updatedAt string
		var failedAt sql.NullString
		if err := st.db.QueryRowContext(ctx, `
			SELECT status, updated_at, failed_at
			FROM github_deliveries
			WHERE project_id = $1 AND delivery_id = $2
		`, projectID, tc.deliveryID).Scan(&status, &updatedAt, &failedAt); err != nil {
			t.Fatal(err)
		}
		if status != tc.wantStatus || updatedAt != tc.wantUpdated || failedAt.Valid {
			t.Fatalf("%s upgrade state = %q, %q, %#v", tc.deliveryID, status, updatedAt, failedAt)
		}
	}

	if err := st.FailGitHubDelivery(ctx, projectID, "old-processing"); err != nil {
		t.Fatal(err)
	}
	claim, err := st.ClaimGitHubDelivery(ctx, projectID, "old-processing", "issues")
	if err != nil || claim != store.GitHubDeliveryClaimed {
		t.Fatalf("expected upgraded failed delivery to be retryable, claim=%q err=%v", claim, err)
	}
	claim, err = st.ClaimGitHubDelivery(ctx, projectID, "old-complete", "pull_request")
	if err != nil || claim != store.GitHubDeliveryComplete {
		t.Fatalf("expected upgraded complete delivery to stay deduplicated, claim=%q err=%v", claim, err)
	}
}

func createPostgresDeliveryTestProject(t *testing.T, ctx context.Context, st *Store, email string) store.Project {
	t.Helper()
	owner, err := st.EnsureBootstrap(ctx, "Project Owner", email)
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := st.CreateProject(ctx, store.CreateProjectInput{
		WorkspaceID: workspaces[0].ID, Name: "Delivery retries", CreatedBy: owner.ID, WebhookSecret: "test-secret",
		Repositories: []store.CreateProjectRepositoryInput{{
			Owner: "openclaw", Name: "clickclack", FullName: "openclaw/clickclack",
			URL: "https://github.com/openclaw/clickclack",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return project
}

func TestGitHubAppInstallationRoutesProjects(t *testing.T) {
	ctx := context.Background()
	st := newIsolatedPostgresTestStore(t)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	owner, err := st.EnsureBootstrap(ctx, "Owner", "postgres-github-app-owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces[0]
	member, err := st.CreateUser(ctx, store.CreateUserInput{
		DisplayName: "Member", Email: "postgres-github-app-member@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddWorkspaceMember(ctx, workspace.ID, member.ID, "member"); err != nil {
		t.Fatal(err)
	}
	input := store.UpsertGitHubAppInstallationInput{
		InstallationID: 99, WorkspaceID: workspace.ID, AccountLogin: "openclaw",
		AccountType: "Organization", RepositorySelection: "selected", InstalledBy: member.ID,
	}
	if _, err := st.UpsertGitHubAppInstallation(ctx, input); !errors.Is(err, store.ErrNotWorkspaceManager) {
		t.Fatalf("member installation error = %v, want manager denial", err)
	}
	input.InstalledBy = owner.ID
	if _, err := st.UpsertGitHubAppInstallation(ctx, input); err != nil {
		t.Fatal(err)
	}
	secondWorkspace, err := st.CreateWorkspace(ctx, store.CreateWorkspaceInput{Name: "Second"}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := input
	secondInput.WorkspaceID = secondWorkspace.ID
	if _, err := st.UpsertGitHubAppInstallation(ctx, secondInput); err != nil {
		t.Fatalf("same GitHub installation could not be linked to a second workspace: %v", err)
	}
	project, _, err := st.CreateProject(ctx, store.CreateProjectInput{
		WorkspaceID: workspace.ID, Name: "GitHub App", CreatedBy: owner.ID, WebhookSecret: "internal-secret",
		Repositories: []store.CreateProjectRepositoryInput{{
			Owner: "openclaw", Name: "clickclack", FullName: "openclaw/clickclack",
			URL: "https://github.com/openclaw/clickclack", GitHubInstallationID: 99,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	targets, err := st.ListGitHubAppWebhookTargets(ctx, 99, "OPENCLAW/CLICKCLACK")
	if err != nil || len(targets) != 1 || targets[0].ProjectID != project.ID {
		t.Fatalf("unexpected GitHub App targets: %#v, %v", targets, err)
	}
}
