package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaw/clickclack/apps/api/internal/realtime"
	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func TestGitHubAppInstallProjectAndWebhookFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newEmptyHTTPStore(t)
	owner, err := st.EnsureBootstrap(ctx, "Owner", "github-app-owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces[0]
	member, err := st.CreateUser(ctx, store.CreateUserInput{
		DisplayName: "Member", Email: "github-app-member@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddWorkspaceMember(ctx, workspace.ID, member.ID, "member"); err != nil {
		t.Fatal(err)
	}

	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/99/access_tokens":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				t.Errorf("installation token request missing app JWT")
			}
			writeJSON(w, http.StatusCreated, map[string]string{"token": "installation-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			if r.Header.Get("Authorization") != "Bearer installation-token" {
				t.Errorf("repository request used wrong token")
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"total_count": 1,
				"repositories": []map[string]any{{
					"id": 7, "full_name": "OpenClaw/ClickClack",
					"html_url": "https://github.com/openclaw/clickclack", "private": true,
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/login/oauth/access_token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token form: %v", err)
			}
			if r.Form.Get("code") != "verified-code" {
				t.Errorf("unexpected OAuth code: %q", r.Form.Get("code"))
			}
			writeJSON(w, http.StatusOK, map[string]string{"access_token": "user-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/user/installations/99":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Errorf("installation verification used wrong token")
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"id": 99, "repository_selection": "selected",
				"account": map[string]string{"login": "openclaw", "type": "Organization"},
			})
		default:
			http.Error(w, "unexpected GitHub request", http.StatusNotFound)
		}
	}))
	t.Cleanup(github.Close)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	const webhookSecret = "github-app-webhook-secret"
	apiServer := httptest.NewServer(New(st, realtime.NewHub(), Options{
		UploadDir: filepath.Join(t.TempDir(), "uploads"),
		GitHubApp: GitHubAppConfig{
			AppID: 123, Slug: "clickclack-test", ClientID: "client-id",
			ClientSecret: "client-secret", PrivateKeyPEM: privateKeyPEM,
			WebhookSecret: webhookSecret, PublicURL: "https://clickclack.example",
			APIURL: github.URL, AuthURL: github.URL + "/login/oauth/authorize",
			TokenURL: github.URL + "/login/oauth/access_token",
		},
	}).Handler())
	t.Cleanup(apiServer.Close)
	expectStatusAsUser(
		t, "missing-user", http.MethodGet,
		apiServer.URL+"/api/workspaces/"+workspace.ID+"/github-app",
		nil, http.StatusUnauthorized,
	)

	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	installLocation := getRedirectAsUser(
		t, noRedirect, owner.ID,
		apiServer.URL+"/api/workspaces/"+workspace.ID+"/github-app/install",
	)
	installURL, err := url.Parse(installLocation)
	if err != nil {
		t.Fatal(err)
	}
	if installURL.Host != "github.com" || installURL.Path != "/apps/clickclack-test/installations/new" {
		t.Fatalf("unexpected install redirect: %s", installLocation)
	}
	installState := installURL.Query().Get("state")
	if installState == "" {
		t.Fatal("install redirect did not include state")
	}
	expectStatusAsUser(
		t, member.ID, http.MethodGet,
		apiServer.URL+"/api/workspaces/"+workspace.ID+"/github-app/install",
		nil, http.StatusForbidden,
	)

	verifyLocation := getRedirectAsUser(
		t, noRedirect, owner.ID,
		apiServer.URL+"/api/github/app/setup?installation_id=99&state="+url.QueryEscape(installState),
	)
	verifyURL, err := url.Parse(verifyLocation)
	if err != nil {
		t.Fatal(err)
	}
	if verifyURL.Host != strings.TrimPrefix(github.URL, "http://") || verifyURL.Path != "/login/oauth/authorize" {
		t.Fatalf("unexpected verification redirect: %s", verifyLocation)
	}
	verifyState := verifyURL.Query().Get("state")
	if verifyState == "" {
		t.Fatal("verification redirect did not include state")
	}

	destination := getRedirectAsUser(
		t, noRedirect, owner.ID,
		apiServer.URL+"/api/github/app/callback?code=verified-code&state="+url.QueryEscape(verifyState),
	)
	if !strings.Contains(destination, "/app/"+workspace.ID+"/projects?github_app=connected") {
		t.Fatalf("unexpected project redirect: %s", destination)
	}

	status := getJSONAsUser[struct {
		Configured    bool                          `json:"configured"`
		Installations []store.GitHubAppInstallation `json:"installations"`
		Repositories  []GitHubAppRepository         `json:"repositories"`
	}](t, owner.ID, apiServer.URL+"/api/workspaces/"+workspace.ID+"/github-app")
	if !status.Configured || len(status.Installations) != 1 || len(status.Repositories) != 1 {
		t.Fatalf("unexpected GitHub App status: %#v", status)
	}
	expectStatusAsUser(
		t, member.ID, http.MethodGet,
		apiServer.URL+"/api/workspaces/"+workspace.ID+"/github-app",
		nil, http.StatusForbidden,
	)
	if status.Repositories[0].FullName != "openclaw/clickclack" {
		t.Fatalf("repository was not normalized: %#v", status.Repositories[0])
	}

	created := postJSONAsUser[struct {
		Project store.Project      `json:"project"`
		Webhook *map[string]string `json:"webhook"`
	}](t, owner.ID, apiServer.URL+"/api/workspaces/"+workspace.ID+"/projects", map[string]any{
		"name": "App project",
		"github_repositories": []map[string]any{{
			"installation_id": 99, "full_name": "openclaw/clickclack",
		}},
	})
	if created.Webhook != nil {
		t.Fatalf("GitHub App project unexpectedly returned manual webhook credentials: %#v", created.Webhook)
	}
	if len(created.Project.Repositories) != 1 ||
		created.Project.Repositories[0].GitHubInstallationID == nil ||
		*created.Project.Repositories[0].GitHubInstallationID != 99 {
		t.Fatalf("project did not retain GitHub installation: %#v", created.Project.Repositories)
	}

	opened := map[string]any{
		"action":       "opened",
		"installation": map[string]any{"id": 99},
		"repository":   map[string]any{"full_name": "openclaw/clickclack"},
		"sender":       map[string]any{"login": "alice"},
		"issue": map[string]any{
			"number": 14, "title": "App webhook delivery",
			"html_url": "https://github.com/openclaw/clickclack/issues/14",
			"user":     map[string]any{"login": "alice"},
		},
	}
	webhookURL := apiServer.URL + "/api/hooks/github/app"
	sendProjectWebhook(t, webhookURL, webhookSecret, "issues", "app-delivery", opened, http.StatusAccepted)
	sendProjectWebhook(t, webhookURL, webhookSecret, "issues", "app-delivery", opened, http.StatusAccepted)
	sendProjectWebhook(t, webhookURL, "wrong", "issues", "bad-signature", opened, http.StatusUnauthorized)

	messages, err := st.ListMessages(ctx, created.Project.Channel.ID, owner.ID, store.MessagePageRequest{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages.Messages) != 1 || !strings.Contains(messages.Messages[0].Body, "Issue #14") {
		t.Fatalf("GitHub App event was not routed exactly once: %#v", messages.Messages)
	}
}

func getRedirectAsUser(t *testing.T, client *http.Client, userID, endpoint string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-ClickClack-User", userID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s: expected redirect, got %s %s", endpoint, resp.Status, string(body))
	}
	return resp.Header.Get("Location")
}

func TestGitHubAppStatusWhenUnconfigured(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newEmptyHTTPStore(t)
	owner, err := st.EnsureBootstrap(ctx, "Owner", "github-app-disabled@example.com")
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(st, realtime.NewHub(), Options{}).Handler())
	t.Cleanup(server.Close)
	body := getJSONAsUser[map[string]json.RawMessage](
		t, owner.ID, server.URL+"/api/workspaces/"+workspaces[0].ID+"/github-app",
	)
	var configured bool
	if err := json.Unmarshal(body["configured"], &configured); err != nil || configured {
		t.Fatalf("unexpected disabled status: %s", body["configured"])
	}
}
