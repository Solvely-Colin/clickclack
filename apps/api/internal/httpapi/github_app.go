package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/openclaw/clickclack/apps/api/internal/store"
)

type GitHubAppConfig struct {
	AppID         int64
	Slug          string
	ClientID      string
	ClientSecret  string
	PrivateKeyPEM []byte
	WebhookSecret string
	PublicURL     string
	APIURL        string
	AuthURL       string
	TokenURL      string
	HTTPClient    *http.Client
}

type GitHubAppRepository struct {
	InstallationID int64  `json:"installation_id"`
	ID             int64  `json:"id"`
	FullName       string `json:"full_name"`
	HTMLURL        string `json:"html_url"`
	Private        bool   `json:"private"`
}

type githubAppStateClaims struct {
	Stage          string `json:"stage"`
	WorkspaceID    string `json:"workspace_id"`
	UserID         string `json:"user_id"`
	InstallationID int64  `json:"installation_id,omitempty"`
	jwt.RegisteredClaims
}

type githubAppInstallationResponse struct {
	ID                  int64  `json:"id"`
	RepositorySelection string `json:"repository_selection"`
	Account             struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
}

const githubAppStateTTL = 10 * time.Minute

func (c GitHubAppConfig) withDefaults() GitHubAppConfig {
	c.Slug = strings.TrimSpace(c.Slug)
	c.ClientID = strings.TrimSpace(c.ClientID)
	c.ClientSecret = strings.TrimSpace(c.ClientSecret)
	c.WebhookSecret = strings.TrimSpace(c.WebhookSecret)
	c.PublicURL = strings.TrimRight(strings.TrimSpace(c.PublicURL), "/")
	c.APIURL = strings.TrimRight(strings.TrimSpace(c.APIURL), "/")
	if c.APIURL == "" {
		c.APIURL = "https://api.github.com"
	}
	if c.AuthURL == "" {
		c.AuthURL = "https://github.com/login/oauth/authorize"
	}
	if c.TokenURL == "" {
		c.TokenURL = "https://github.com/login/oauth/access_token"
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultGitHubHTTPTimeout}
	}
	return c
}

func (c GitHubAppConfig) configured() bool {
	return c.AppID > 0 && c.Slug != "" && c.ClientID != "" && c.ClientSecret != "" &&
		len(c.PrivateKeyPEM) > 0 && c.WebhookSecret != "" && c.PublicURL != ""
}

func (s *Server) getWorkspaceGitHubApp(w http.ResponseWriter, r *http.Request) {
	act, workspaceID, err := s.githubAppWorkspaceActor(r)
	if err != nil {
		writeGitHubAppActorError(w, act, err)
		return
	}
	if !s.githubApp.configured() {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": false, "installations": []store.GitHubAppInstallation{},
			"repositories": []GitHubAppRepository{},
		})
		return
	}
	if err := s.requireWorkspaceManager(r.Context(), workspaceID, act.user.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	installations, err := s.store.ListGitHubAppInstallations(r.Context(), workspaceID, act.user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	repositories := make([]GitHubAppRepository, 0)
	for _, installation := range installations {
		items, err := s.listGitHubAppRepositories(r.Context(), installation.InstallationID)
		if err != nil {
			writeError(w, http.StatusBadGateway, errors.New("could not load GitHub App repositories"))
			return
		}
		repositories = append(repositories, items...)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true, "slug": s.githubApp.Slug,
		"installations": installations, "repositories": repositories,
	})
}

func (s *Server) startGitHubAppInstall(w http.ResponseWriter, r *http.Request) {
	act, workspaceID, err := s.githubAppWorkspaceActor(r)
	if err != nil {
		writeGitHubAppActorError(w, act, err)
		return
	}
	if act.botTokenID != "" {
		writeError(w, http.StatusForbidden, errors.New("bot tokens cannot install GitHub Apps"))
		return
	}
	if !s.githubApp.configured() {
		writeError(w, http.StatusNotImplemented, errors.New("GitHub App is not configured"))
		return
	}
	if err := s.requireWorkspaceManager(r.Context(), workspaceID, act.user.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	state, err := s.signGitHubAppState("install", workspaceID, act.user.ID, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	installURL := "https://github.com/apps/" + url.PathEscape(s.githubApp.Slug) + "/installations/new"
	query := url.Values{"state": []string{state}}
	http.Redirect(w, r, installURL+"?"+query.Encode(), http.StatusFound)
}

func (s *Server) githubAppSetup(w http.ResponseWriter, r *http.Request) {
	if !s.githubApp.configured() {
		writeError(w, http.StatusNotFound, errors.New("GitHub App is not configured"))
		return
	}
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	claims, err := s.parseGitHubAppState(r.URL.Query().Get("state"), "install")
	if err != nil || claims.UserID != act.user.ID {
		writeError(w, http.StatusBadRequest, errors.New("invalid GitHub App installation state"))
		return
	}
	if err := s.requireWorkspaceManager(r.Context(), claims.WorkspaceID, act.user.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	installationID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("installation_id")), 10, 64)
	if err != nil || installationID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("GitHub installation ID is required"))
		return
	}
	state, err := s.signGitHubAppState("verify", claims.WorkspaceID, act.user.ID, installationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	callbackURL := s.githubApp.PublicURL + "/api/github/app/callback"
	query := url.Values{
		"client_id":    []string{s.githubApp.ClientID},
		"redirect_uri": []string{callbackURL},
		"state":        []string{state},
	}
	http.Redirect(w, r, s.githubApp.AuthURL+"?"+query.Encode(), http.StatusFound)
}

func (s *Server) githubAppCallback(w http.ResponseWriter, r *http.Request) {
	if !s.githubApp.configured() {
		writeError(w, http.StatusNotFound, errors.New("GitHub App is not configured"))
		return
	}
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	claims, err := s.parseGitHubAppState(r.URL.Query().Get("state"), "verify")
	if err != nil || claims.UserID != act.user.ID || claims.InstallationID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid GitHub App verification state"))
		return
	}
	if err := s.requireWorkspaceManager(r.Context(), claims.WorkspaceID, act.user.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, http.StatusBadRequest, errors.New("GitHub App authorization code is required"))
		return
	}
	token, err := s.exchangeGitHubAppUserCode(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusBadGateway, errors.New("GitHub App authorization failed"))
		return
	}
	installation, err := s.getGitHubUserInstallation(r.Context(), token, claims.InstallationID)
	if err != nil {
		writeError(w, http.StatusForbidden, errors.New("GitHub App installation is not accessible to this user"))
		return
	}
	if _, err := s.store.UpsertGitHubAppInstallation(r.Context(), store.UpsertGitHubAppInstallationInput{
		InstallationID: installation.ID, WorkspaceID: claims.WorkspaceID,
		AccountLogin: installation.Account.Login, AccountType: installation.Account.Type,
		RepositorySelection: installation.RepositorySelection, InstalledBy: act.user.ID,
	}); err != nil {
		writeStoreError(w, err)
		return
	}
	destination := strings.TrimRight(firstNonEmpty(s.frontendURL, s.githubApp.PublicURL), "/") +
		"/app/" + url.PathEscape(claims.WorkspaceID) + "/projects?github_app=connected"
	http.Redirect(w, r, destination, http.StatusFound)
}

func (s *Server) githubAppWorkspaceActor(r *http.Request) (actor, string, error) {
	act, err := s.currentActor(r)
	if err != nil {
		return actor{}, "", err
	}
	if err := act.requireScope("workspaces:read"); err != nil {
		return act, "", err
	}
	workspaceID := chi.URLParam(r, "workspace_id")
	if err := act.requireWorkspace(workspaceID); err != nil {
		return act, "", err
	}
	if _, err := s.store.GetWorkspace(r.Context(), workspaceID, act.user.ID); err != nil {
		return act, "", err
	}
	return act, workspaceID, nil
}

func writeGitHubAppActorError(w http.ResponseWriter, act actor, err error) {
	if act.user.ID == "" {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeError(w, http.StatusForbidden, err)
}

func (s *Server) requireWorkspaceManager(ctx context.Context, workspaceID, userID string) error {
	workspace, err := s.store.GetWorkspace(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if workspace.Role != "owner" && workspace.Role != "moderator" {
		return store.ErrNotWorkspaceManager
	}
	return nil
}

func (s *Server) signGitHubAppState(stage, workspaceID, userID string, installationID int64) (string, error) {
	now := time.Now().UTC()
	claims := githubAppStateClaims{
		Stage: stage, WorkspaceID: workspaceID, UserID: userID, InstallationID: installationID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "clickclack-github-app", IssuedAt: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(githubAppStateTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.githubApp.WebhookSecret))
}

func (s *Server) parseGitHubAppState(raw, stage string) (githubAppStateClaims, error) {
	var claims githubAppStateClaims
	token, err := jwt.ParseWithClaims(strings.TrimSpace(raw), &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("invalid state signing method")
		}
		return []byte(s.githubApp.WebhookSecret), nil
	}, jwt.WithIssuer("clickclack-github-app"), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || claims.Stage != stage || claims.WorkspaceID == "" || claims.UserID == "" {
		return githubAppStateClaims{}, errors.New("invalid GitHub App state")
	}
	return claims, nil
}

func (s *Server) githubAppJWT() (string, error) {
	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(s.githubApp.PrivateKeyPEM)
	if err != nil {
		return "", errors.New("invalid GitHub App private key")
	}
	now := time.Now().UTC()
	claims := jwt.RegisteredClaims{
		Issuer:    strconv.FormatInt(s.githubApp.AppID, 10),
		IssuedAt:  jwt.NewNumericDate(now.Add(-30 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(privateKey)
}

func (s *Server) githubAppRequest(ctx context.Context, method, endpoint, token string, body any, output any) error {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.githubApp.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub App request failed: %s", resp.Status)
	}
	if output == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(output)
}

func (s *Server) installationAccessToken(ctx context.Context, installationID int64) (string, error) {
	appJWT, err := s.githubAppJWT()
	if err != nil {
		return "", err
	}
	var response struct {
		Token string `json:"token"`
	}
	endpoint := fmt.Sprintf("%s/app/installations/%d/access_tokens", s.githubApp.APIURL, installationID)
	if err := s.githubAppRequest(ctx, http.MethodPost, endpoint, appJWT, map[string]any{}, &response); err != nil {
		return "", err
	}
	if response.Token == "" {
		return "", errors.New("GitHub App returned an empty installation token")
	}
	return response.Token, nil
}

func (s *Server) listGitHubAppRepositories(ctx context.Context, installationID int64) ([]GitHubAppRepository, error) {
	token, err := s.installationAccessToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	result := make([]GitHubAppRepository, 0)
	for page := 1; ; page++ {
		var response struct {
			TotalCount   int `json:"total_count"`
			Repositories []struct {
				ID       int64  `json:"id"`
				FullName string `json:"full_name"`
				HTMLURL  string `json:"html_url"`
				Private  bool   `json:"private"`
			} `json:"repositories"`
		}
		endpoint := fmt.Sprintf("%s/installation/repositories?per_page=100&page=%d", s.githubApp.APIURL, page)
		if err := s.githubAppRequest(ctx, http.MethodGet, endpoint, token, nil, &response); err != nil {
			return nil, err
		}
		for _, repository := range response.Repositories {
			result = append(result, GitHubAppRepository{
				InstallationID: installationID, ID: repository.ID,
				FullName: strings.ToLower(repository.FullName), HTMLURL: repository.HTMLURL, Private: repository.Private,
			})
		}
		if len(response.Repositories) < 100 || (response.TotalCount > 0 && len(result) >= response.TotalCount) {
			break
		}
		if page >= 100 {
			return nil, errors.New("GitHub App repository list exceeds the supported limit")
		}
	}
	return result, nil
}

func (s *Server) exchangeGitHubAppUserCode(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"client_id": []string{s.githubApp.ClientID}, "client_secret": []string{s.githubApp.ClientSecret},
		"code": []string{code}, "redirect_uri": []string{s.githubApp.PublicURL + "/api/github/app/callback"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.githubApp.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.githubApp.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub App token exchange failed: %s", resp.Status)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.AccessToken == "" || result.Error != "" {
		return "", errors.New("GitHub App token exchange returned no token")
	}
	return result.AccessToken, nil
}

func (s *Server) getGitHubUserInstallation(ctx context.Context, token string, installationID int64) (githubAppInstallationResponse, error) {
	var installation githubAppInstallationResponse
	endpoint := fmt.Sprintf("%s/user/installations/%d", s.githubApp.APIURL, installationID)
	err := s.githubAppRequest(ctx, http.MethodGet, endpoint, token, nil, &installation)
	return installation, err
}

func (s *Server) githubAppWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.githubApp.configured() {
		writeError(w, http.StatusNotFound, errors.New("GitHub App is not configured"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	payloadBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !validGitHubSignature(payloadBytes, r.Header.Get("X-Hub-Signature-256"), s.githubApp.WebhookSecret) {
		writeError(w, http.StatusUnauthorized, errors.New("invalid GitHub webhook signature"))
		return
	}
	var payload githubProjectPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid GitHub webhook payload"))
		return
	}
	eventType := strings.TrimSpace(r.Header.Get("X-GitHub-Event"))
	if eventType == "ping" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "pong"})
		return
	}
	deliveryID := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery"))
	if deliveryID == "" || eventType == "" {
		writeError(w, http.StatusBadRequest, errors.New("GitHub delivery and event headers are required"))
		return
	}
	if payload.Installation == nil || payload.Installation.ID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("GitHub App installation is required"))
		return
	}
	if strings.TrimSpace(payload.Repository.FullName) == "" {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status": "ignored", "delivery_id": deliveryID, "targets": 0,
		})
		return
	}
	targets, err := s.store.ListGitHubAppWebhookTargets(
		r.Context(), payload.Installation.ID, payload.Repository.FullName,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if len(targets) == 0 {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status": "ignored", "delivery_id": deliveryID, "targets": 0,
		})
		return
	}
	updates := 0
	duplicates := 0
	processing := false
	for _, target := range targets {
		result, err := s.processGitHubProjectDelivery(r, target, deliveryID, eventType, payload)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		switch result.status {
		case store.GitHubDeliveryComplete:
			duplicates++
		case store.GitHubDeliveryProcessing:
			processing = true
		default:
			updates += result.updates
		}
	}
	if processing {
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusServiceUnavailable, errors.New("GitHub delivery is still processing"))
		return
	}
	status := "accepted"
	if duplicates == len(targets) {
		status = "duplicate"
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": status, "delivery_id": deliveryID, "targets": len(targets), "updates": updates,
	})
}
