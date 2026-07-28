package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsEnvAndFile(t *testing.T) {
	t.Setenv("CLICKCLACK_ADDR", ":9000")
	t.Setenv("CLICKCLACK_DATA", "/tmp/clickclack")
	t.Setenv("CLICKCLACK_DB", "sqlite:///tmp/clickclack.db")
	t.Setenv("CLICKCLACK_UPLOADS", "r2://clickclack-uploads/prod")
	t.Setenv("CLICKCLACK_ENVIRONMENT", "fakeco")
	t.Setenv("CLICKCLACK_METRICS_ENABLED", "true")
	t.Setenv("CLICKCLACK_PUBLIC_URL", "https://clickclack.test")
	t.Setenv("CLICKCLACK_PUBLIC_API_URL", "https://api.clickclack.test/services/clickclack/")
	t.Setenv("CLICKCLACK_EMBED_FRAME_ANCESTORS", "https://control.example.com, https://dock.example.com")
	t.Setenv("CLICKCLACK_COOKIE_NAMESPACE", "prod-2")
	t.Setenv("CLICKCLACK_DEV_BOOTSTRAP", "false")
	t.Setenv("CLICKCLACK_GITHUB_CLIENT_ID", "client")
	t.Setenv("CLICKCLACK_GITHUB_CLIENT_SECRET", "secret")
	t.Setenv("CLICKCLACK_GITHUB_ALLOWED_ORG", "openclaw")
	t.Setenv("CLICKCLACK_GITHUB_MODERATOR_ORG", "openclaw")
	t.Setenv("CLICKCLACK_GITHUB_APP_ID", "123")
	t.Setenv("CLICKCLACK_GITHUB_APP_SLUG", "clickclack-test")
	t.Setenv("CLICKCLACK_GITHUB_APP_CLIENT_ID", "app-client")
	t.Setenv("CLICKCLACK_GITHUB_APP_CLIENT_SECRET", "app-secret")
	t.Setenv("CLICKCLACK_GITHUB_APP_PRIVATE_KEY_BASE64", "cHJpdmF0ZS1rZXk=")
	t.Setenv("CLICKCLACK_GITHUB_APP_WEBHOOK_SECRET", "webhook-secret")
	t.Setenv("CLICKCLACK_PUSHOVER_API_TOKEN", "app-token")
	t.Setenv("CLICKCLACK_R2_ACCOUNT_ID", "account")
	t.Setenv("CLICKCLACK_R2_ACCESS_KEY_ID", "access")
	t.Setenv("CLICKCLACK_R2_SECRET_ACCESS_KEY", "secret-access")
	t.Setenv("CLICKCLACK_R2_ENDPOINT", "https://r2.example.com")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9000" || cfg.Data != "/tmp/clickclack" || cfg.DB != "sqlite:///tmp/clickclack.db" || cfg.Uploads != "r2://clickclack-uploads/prod" || cfg.Environment != "fakeco" || !cfg.MetricsEnabled || cfg.PublicURL != "https://clickclack.test" || cfg.PublicAPIURL != "https://api.clickclack.test/services/clickclack/" || len(cfg.EmbedFrameAncestors) != 2 || cfg.EmbedFrameAncestors[0] != "https://control.example.com" || cfg.CookieNamespace != "prod-2" || cfg.DevBootstrap || cfg.GitHubClientID != "client" || cfg.GitHubClientSecret != "secret" || cfg.GitHubAllowedOrg != "openclaw" || cfg.GitHubModeratorOrg != "openclaw" || cfg.GitHubAppID != 123 || cfg.GitHubAppSlug != "clickclack-test" || cfg.GitHubAppClientID != "app-client" || cfg.GitHubAppClientSecret != "app-secret" || cfg.GitHubAppPrivateKeyBase64 != "cHJpdmF0ZS1rZXk=" || cfg.GitHubAppWebhookSecret != "webhook-secret" || cfg.PushoverAPIToken != "app-token" || cfg.R2AccountID != "account" || cfg.R2AccessKeyID != "access" || cfg.R2SecretAccessKey != "secret-access" || cfg.R2Endpoint != "https://r2.example.com" {
		t.Fatalf("unexpected env config: %#v", cfg)
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"addr":":7000","data":"/data"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9000" || cfg.Data != "/tmp/clickclack" || cfg.DB != "sqlite:///tmp/clickclack.db" {
		t.Fatalf("expected env to override file config: %#v", cfg)
	}

	t.Setenv("CLICKCLACK_ADDR", "")
	t.Setenv("CLICKCLACK_DATA", "")
	t.Setenv("CLICKCLACK_DB", "")
	t.Setenv("CLICKCLACK_UPLOADS", "")
	t.Setenv("CLICKCLACK_ENVIRONMENT", "")
	t.Setenv("CLICKCLACK_METRICS_ENABLED", "")
	t.Setenv("CLICKCLACK_PUBLIC_URL", "")
	t.Setenv("CLICKCLACK_PUBLIC_API_URL", "")
	t.Setenv("CLICKCLACK_EMBED_FRAME_ANCESTORS", "")
	t.Setenv("CLICKCLACK_COOKIE_NAMESPACE", "")
	t.Setenv("CLICKCLACK_DEV_BOOTSTRAP", "")
	t.Setenv("CLICKCLACK_GITHUB_CLIENT_ID", "")
	t.Setenv("CLICKCLACK_GITHUB_CLIENT_SECRET", "")
	t.Setenv("CLICKCLACK_GITHUB_ALLOWED_ORG", "")
	t.Setenv("CLICKCLACK_GITHUB_MODERATOR_ORG", "")
	t.Setenv("CLICKCLACK_GITHUB_APP_ID", "")
	t.Setenv("CLICKCLACK_GITHUB_APP_SLUG", "")
	t.Setenv("CLICKCLACK_GITHUB_APP_CLIENT_ID", "")
	t.Setenv("CLICKCLACK_GITHUB_APP_CLIENT_SECRET", "")
	t.Setenv("CLICKCLACK_GITHUB_APP_PRIVATE_KEY_BASE64", "")
	t.Setenv("CLICKCLACK_GITHUB_APP_WEBHOOK_SECRET", "")
	t.Setenv("CLICKCLACK_PUSHOVER_API_TOKEN", "")
	t.Setenv("CLICKCLACK_R2_ACCOUNT_ID", "")
	t.Setenv("CLICKCLACK_R2_ACCESS_KEY_ID", "")
	t.Setenv("CLICKCLACK_R2_SECRET_ACCESS_KEY", "")
	t.Setenv("CLICKCLACK_R2_ENDPOINT", "")
	emptyPath := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(emptyPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(emptyPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.Data != "./data" || cfg.DevBootstrap {
		t.Fatalf("unexpected fallback config: %#v", cfg)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected missing config error")
	}
	t.Setenv("CLICKCLACK_METRICS_ENABLED", "not-bool")
	if _, err := Load(""); err == nil {
		t.Fatal("expected bad metrics bool env error")
	}
	t.Setenv("CLICKCLACK_METRICS_ENABLED", "")
	t.Setenv("CLICKCLACK_DEV_BOOTSTRAP", "not-bool")
	if _, err := Load(""); err == nil {
		t.Fatal("expected bad bool env error")
	}
	overrideBoolPath := filepath.Join(t.TempDir(), "override-bool.json")
	if err := os.WriteFile(overrideBoolPath, []byte(`{"dev_bootstrap":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(overrideBoolPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DevBootstrap {
		t.Fatalf("expected file boolean to override invalid env: %#v", cfg)
	}
	t.Setenv("CLICKCLACK_DEV_BOOTSTRAP", "")
	badPath := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badPath, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(badPath); err == nil {
		t.Fatal("expected bad json error")
	}
}

func TestValidateServe(t *testing.T) {
	t.Parallel()
	cfg := Config{
		PublicURL:          "https://Chat.Example.com:443/",
		CookieNamespace:    " prod-2 ",
		GitHubClientID:     " client ",
		GitHubClientSecret: " secret ",
		GitHubAllowedOrg:   " openclaw ",
	}
	if err := cfg.ValidateServe(); err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "https://chat.example.com" || cfg.CookieNamespace != "prod-2" || cfg.GitHubClientID != "client" || cfg.GitHubClientSecret != "secret" || cfg.GitHubAllowedOrg != "openclaw" {
		t.Fatalf("unexpected validated config: %#v", cfg)
	}

	disabled := Config{GitHubClientID: " ", GitHubClientSecret: "\t"}
	if err := disabled.ValidateServe(); err != nil {
		t.Fatal(err)
	}
	if disabled.GitHubClientID != "" || disabled.GitHubClientSecret != "" {
		t.Fatalf("expected whitespace credentials to normalize as disabled: %#v", disabled)
	}
	githubApp := Config{
		PublicURL:                 "https://chat.example.com",
		GitHubAppID:               123,
		GitHubAppSlug:             " clickclack-test ",
		GitHubAppClientID:         " app-client ",
		GitHubAppClientSecret:     " app-secret ",
		GitHubAppPrivateKeyBase64: testGitHubAppPrivateKeyBase64(t),
		GitHubAppWebhookSecret:    " 01234567890123456789012345678901 ",
	}
	if err := githubApp.ValidateServe(); err != nil {
		t.Fatal(err)
	}
	if githubApp.GitHubAppSlug != "clickclack-test" || githubApp.GitHubAppClientID != "app-client" ||
		githubApp.GitHubAppClientSecret != "app-secret" ||
		githubApp.GitHubAppWebhookSecret != "01234567890123456789012345678901" {
		t.Fatalf("unexpected normalized GitHub App config: %#v", githubApp)
	}
	sameOrigin := Config{PublicURL: "https://chat.example.com"}
	if err := sameOrigin.ValidateServe(); err != nil || sameOrigin.PublicAPIURL != sameOrigin.PublicURL {
		t.Fatalf("expected public API URL to default to public URL: %#v %v", sameOrigin, err)
	}
	splitOrigin := Config{PublicURL: "https://chat.example.com", PublicAPIURL: "https://API.Example.com:443/services/clickclack/"}
	if err := splitOrigin.ValidateServe(); err != nil || splitOrigin.PublicAPIURL != "https://api.example.com/services/clickclack" {
		t.Fatalf("expected canonical split API URL: %#v %v", splitOrigin, err)
	}
	embedOrigins := Config{EmbedFrameAncestors: []string{"HTTPS://Control.Example.com/", "https://control.example.com", "http://localhost:3000"}}
	if err := embedOrigins.ValidateServe(); err != nil {
		t.Fatal(err)
	}
	if len(embedOrigins.EmbedFrameAncestors) != 2 || embedOrigins.EmbedFrameAncestors[0] != "https://control.example.com" || embedOrigins.EmbedFrameAncestors[1] != "http://localhost:3000" {
		t.Fatalf("unexpected normalized embed origins: %#v", embedOrigins.EmbedFrameAncestors)
	}

	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{"invalid namespace", Config{CookieNamespace: "__Host-session", PublicURL: "https://chat.example.com"}},
		{"namespace without public url", Config{CookieNamespace: "prod"}},
		{"non-https remote url", Config{PublicURL: "http://chat.example.com"}},
		{"public url path", Config{PublicURL: "https://chat.example.com/app"}},
		{"public api url query", Config{PublicURL: "https://chat.example.com", PublicAPIURL: "https://api.example.com?x=1"}},
		{"mixed loopback schemes", Config{PublicURL: "http://localhost:8080", PublicAPIURL: "https://localhost:8443"}},
		{"different loopback hosts", Config{PublicURL: "http://localhost:8080", PublicAPIURL: "http://127.0.0.1:8081"}},
		{"missing client secret", Config{PublicURL: "https://chat.example.com", GitHubClientID: "client"}},
		{"oauth without public url", Config{GitHubClientID: "client", GitHubClientSecret: "secret"}},
		{"org without oauth", Config{GitHubAllowedOrg: "openclaw"}},
		{"partial GitHub App", Config{PublicURL: "https://chat.example.com", GitHubAppID: 123}},
		{"negative GitHub App ID", Config{GitHubAppID: -1}},
		{"invalid GitHub App slug", Config{
			PublicURL: "https://chat.example.com", GitHubAppID: 123, GitHubAppSlug: "ClickClack_App",
			GitHubAppClientID: "client", GitHubAppClientSecret: "secret",
			GitHubAppPrivateKeyBase64: testGitHubAppPrivateKeyBase64(t),
			GitHubAppWebhookSecret:    "01234567890123456789012345678901",
		}},
		{"invalid GitHub App private key encoding", Config{
			PublicURL: "https://chat.example.com", GitHubAppID: 123, GitHubAppSlug: "clickclack-test",
			GitHubAppClientID: "client", GitHubAppClientSecret: "secret",
			GitHubAppPrivateKeyBase64: "not-base64",
			GitHubAppWebhookSecret:    "01234567890123456789012345678901",
		}},
		{"invalid GitHub App private key", Config{
			PublicURL: "https://chat.example.com", GitHubAppID: 123, GitHubAppSlug: "clickclack-test",
			GitHubAppClientID: "client", GitHubAppClientSecret: "secret",
			GitHubAppPrivateKeyBase64: base64.StdEncoding.EncodeToString([]byte("not-pem")),
			GitHubAppWebhookSecret:    "01234567890123456789012345678901",
		}},
		{"access domain only", Config{AccessTeamDomain: "https://openclaw.cloudflareaccess.com"}},
		{"access audience only", Config{AccessAUD: "test-aud"}},
		{"access domain must use https", Config{AccessTeamDomain: "http://openclaw.cloudflareaccess.com", AccessAUD: "test-aud"}},
		{"access domain must be an origin", Config{AccessTeamDomain: "https://openclaw.cloudflareaccess.com/path", AccessAUD: "test-aud"}},
		{"invalid embed ancestor", Config{EmbedFrameAncestors: []string{"https://control.example.com/path"}}},
		{"wildcard embed ancestor", Config{EmbedFrameAncestors: []string{"https://*.example.com"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.ValidateServe(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func testGitHubAppPrivateKeyBase64(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	value := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return base64.StdEncoding.EncodeToString(value)
}

func TestValidateAccessConfig(t *testing.T) {
	t.Parallel()
	cfg := Config{AccessTeamDomain: " https://OpenClaw.cloudflareaccess.com:443/ ", AccessAUD: " test-access-aud "}
	if err := cfg.ValidateServe(); err != nil {
		t.Fatal(err)
	}
	if cfg.AccessTeamDomain != "https://openclaw.cloudflareaccess.com" || cfg.AccessAUD != "test-access-aud" {
		t.Fatalf("unexpected validated Access config: %#v", cfg)
	}
}

func TestLoadAccessConfigFromEnvironmentAndJSON(t *testing.T) {
	t.Setenv("CLICKCLACK_ACCESS_TEAM_DOMAIN", "https://env.cloudflareaccess.com")
	t.Setenv("CLICKCLACK_ACCESS_AUD", "test-env-aud")
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"access_team_domain":"https://file.cloudflareaccess.com","access_aud":"test-file-aud"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessTeamDomain != "https://env.cloudflareaccess.com" || cfg.AccessAUD != "test-env-aud" {
		t.Fatalf("environment did not override Access JSON config: %#v", cfg)
	}
	t.Setenv("CLICKCLACK_ACCESS_TEAM_DOMAIN", "")
	t.Setenv("CLICKCLACK_ACCESS_AUD", "")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessTeamDomain != "https://file.cloudflareaccess.com" || cfg.AccessAUD != "test-file-aud" {
		t.Fatalf("Access JSON config was not loaded: %#v", cfg)
	}
}
