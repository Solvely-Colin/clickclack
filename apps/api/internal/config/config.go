package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/openclaw/clickclack/apps/api/internal/authpolicy"
)

var githubAppSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Config struct {
	Addr                      string   `json:"addr"`
	Data                      string   `json:"data"`
	DB                        string   `json:"db"`
	Uploads                   string   `json:"uploads"`
	Environment               string   `json:"environment"`
	MetricsEnabled            bool     `json:"metrics_enabled"`
	PublicURL                 string   `json:"public_url"`
	PublicAPIURL              string   `json:"public_api_url"`
	EmbedFrameAncestors       []string `json:"embed_frame_ancestors"`
	CookieNamespace           string   `json:"cookie_namespace"`
	DevBootstrap              bool     `json:"dev_bootstrap"`
	GitHubClientID            string   `json:"github_client_id"`
	GitHubClientSecret        string   `json:"github_client_secret"`
	GitHubAllowedOrg          string   `json:"github_allowed_org"`
	GitHubModeratorOrg        string   `json:"github_moderator_org"`
	GitHubAppID               int64    `json:"github_app_id"`
	GitHubAppSlug             string   `json:"github_app_slug"`
	GitHubAppClientID         string   `json:"github_app_client_id"`
	GitHubAppClientSecret     string   `json:"github_app_client_secret"`
	GitHubAppPrivateKeyBase64 string   `json:"github_app_private_key_base64"`
	GitHubAppWebhookSecret    string   `json:"github_app_webhook_secret"`
	AccessTeamDomain          string   `json:"access_team_domain"`
	AccessAUD                 string   `json:"access_aud"`
	PushoverAPIToken          string   `json:"pushover_api_token"`
	R2AccountID               string   `json:"r2_account_id"`
	R2AccessKeyID             string   `json:"r2_access_key_id"`
	R2SecretAccessKey         string   `json:"r2_secret_access_key"`
	R2Endpoint                string   `json:"r2_endpoint"`
}

func Defaults() Config {
	return Config{Addr: ":8080", Data: "./data", DevBootstrap: false}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	fileHasDevBootstrap := false
	if path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			return Config{}, err
		}
		_, fileHasDevBootstrap = fields["dev_bootstrap"]
		if err := json.Unmarshal(body, &cfg); err != nil {
			return Config{}, err
		}
	}
	if env := os.Getenv("CLICKCLACK_ADDR"); env != "" {
		cfg.Addr = env
	}
	if env := os.Getenv("CLICKCLACK_DATA"); env != "" {
		cfg.Data = env
	}
	if env := os.Getenv("CLICKCLACK_DB"); env != "" {
		cfg.DB = env
	}
	if env := os.Getenv("CLICKCLACK_UPLOADS"); env != "" {
		cfg.Uploads = env
	}
	if env := os.Getenv("CLICKCLACK_ENVIRONMENT"); env != "" {
		cfg.Environment = env
	}
	if env := os.Getenv("CLICKCLACK_METRICS_ENABLED"); env != "" {
		value, err := strconv.ParseBool(env)
		if err != nil {
			return Config{}, err
		}
		cfg.MetricsEnabled = value
	}
	if env := os.Getenv("CLICKCLACK_PUBLIC_URL"); env != "" {
		cfg.PublicURL = env
	}
	if env := os.Getenv("CLICKCLACK_PUBLIC_API_URL"); env != "" {
		cfg.PublicAPIURL = env
	}
	if env := os.Getenv("CLICKCLACK_EMBED_FRAME_ANCESTORS"); env != "" {
		cfg.EmbedFrameAncestors = ParseEmbedFrameAncestors(env)
	}
	if env := os.Getenv("CLICKCLACK_COOKIE_NAMESPACE"); env != "" {
		cfg.CookieNamespace = env
	}
	if env := os.Getenv("CLICKCLACK_DEV_BOOTSTRAP"); env != "" && !fileHasDevBootstrap {
		value, err := strconv.ParseBool(env)
		if err != nil {
			return Config{}, err
		}
		cfg.DevBootstrap = value
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_CLIENT_ID"); env != "" {
		cfg.GitHubClientID = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_CLIENT_SECRET"); env != "" {
		cfg.GitHubClientSecret = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_ALLOWED_ORG"); env != "" {
		cfg.GitHubAllowedOrg = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_MODERATOR_ORG"); env != "" {
		cfg.GitHubModeratorOrg = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_APP_ID"); env != "" {
		value, err := strconv.ParseInt(env, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("CLICKCLACK_GITHUB_APP_ID: %w", err)
		}
		cfg.GitHubAppID = value
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_APP_SLUG"); env != "" {
		cfg.GitHubAppSlug = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_APP_CLIENT_ID"); env != "" {
		cfg.GitHubAppClientID = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_APP_CLIENT_SECRET"); env != "" {
		cfg.GitHubAppClientSecret = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_APP_PRIVATE_KEY_BASE64"); env != "" {
		cfg.GitHubAppPrivateKeyBase64 = env
	}
	if env := os.Getenv("CLICKCLACK_GITHUB_APP_WEBHOOK_SECRET"); env != "" {
		cfg.GitHubAppWebhookSecret = env
	}
	if env := os.Getenv("CLICKCLACK_ACCESS_TEAM_DOMAIN"); env != "" {
		cfg.AccessTeamDomain = env
	}
	if env := os.Getenv("CLICKCLACK_ACCESS_AUD"); env != "" {
		cfg.AccessAUD = env
	}
	if env := os.Getenv("CLICKCLACK_PUSHOVER_API_TOKEN"); env != "" {
		cfg.PushoverAPIToken = env
	}
	if env := os.Getenv("CLICKCLACK_R2_ACCOUNT_ID"); env != "" {
		cfg.R2AccountID = env
	}
	if env := os.Getenv("CLICKCLACK_R2_ACCESS_KEY_ID"); env != "" {
		cfg.R2AccessKeyID = env
	}
	if env := os.Getenv("CLICKCLACK_R2_SECRET_ACCESS_KEY"); env != "" {
		cfg.R2SecretAccessKey = env
	}
	if env := os.Getenv("CLICKCLACK_R2_ENDPOINT"); env != "" {
		cfg.R2Endpoint = env
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.Data == "" {
		cfg.Data = "./data"
	}
	return cfg, nil
}

func (c *Config) ValidateServe() error {
	if err := normalizeAccessConfig(c); err != nil {
		return err
	}
	namespace, err := authpolicy.ParseCookieNamespace(c.CookieNamespace)
	if err != nil {
		return fmt.Errorf("CLICKCLACK_COOKIE_NAMESPACE: %w", err)
	}
	publicURL, err := authpolicy.CanonicalPublicURL(c.PublicURL)
	if err != nil {
		return fmt.Errorf("CLICKCLACK_PUBLIC_URL: %w", err)
	}
	publicAPIURL, err := authpolicy.CanonicalPublicAPIURL(c.PublicAPIURL)
	if err != nil {
		return fmt.Errorf("CLICKCLACK_PUBLIC_API_URL: %w", err)
	}
	if publicAPIURL == "" {
		publicAPIURL = publicURL
	}
	embedFrameAncestors, err := normalizeEmbedFrameAncestors(c.EmbedFrameAncestors)
	if err != nil {
		return fmt.Errorf("CLICKCLACK_EMBED_FRAME_ANCESTORS: %w", err)
	}
	if err := validatePublicURLPair(publicURL, publicAPIURL); err != nil {
		return err
	}
	clientID := strings.TrimSpace(c.GitHubClientID)
	clientSecret := strings.TrimSpace(c.GitHubClientSecret)
	allowedOrg := strings.TrimSpace(c.GitHubAllowedOrg)
	moderatorOrg := strings.TrimSpace(c.GitHubModeratorOrg)
	githubAppSlug := strings.TrimSpace(c.GitHubAppSlug)
	githubAppClientID := strings.TrimSpace(c.GitHubAppClientID)
	githubAppClientSecret := strings.TrimSpace(c.GitHubAppClientSecret)
	githubAppPrivateKeyBase64 := strings.TrimSpace(c.GitHubAppPrivateKeyBase64)
	githubAppWebhookSecret := strings.TrimSpace(c.GitHubAppWebhookSecret)
	hasClientID := clientID != ""
	hasClientSecret := clientSecret != ""
	if hasClientID != hasClientSecret {
		return errors.New("CLICKCLACK_GITHUB_CLIENT_ID and CLICKCLACK_GITHUB_CLIENT_SECRET must be configured together")
	}
	if hasClientID && publicURL == "" {
		return errors.New("GitHub OAuth requires CLICKCLACK_PUBLIC_URL")
	}
	if (allowedOrg != "" || moderatorOrg != "") && !hasClientID {
		return errors.New("GitHub organization settings require GitHub OAuth credentials")
	}
	githubAppFields := []bool{
		c.GitHubAppID > 0, githubAppSlug != "", githubAppClientID != "",
		githubAppClientSecret != "", githubAppPrivateKeyBase64 != "", githubAppWebhookSecret != "",
	}
	githubAppConfigured := c.GitHubAppID != 0
	for _, configured := range githubAppFields {
		githubAppConfigured = githubAppConfigured || configured
	}
	if githubAppConfigured {
		for _, configured := range githubAppFields {
			if !configured {
				return errors.New("GitHub App requires ID, slug, client credentials, private key, and webhook secret")
			}
		}
		if publicAPIURL == "" {
			return errors.New("GitHub App requires CLICKCLACK_PUBLIC_URL or CLICKCLACK_PUBLIC_API_URL")
		}
		if !githubAppSlugPattern.MatchString(githubAppSlug) {
			return errors.New("CLICKCLACK_GITHUB_APP_SLUG must use lowercase letters, numbers, and single hyphens")
		}
		if len(githubAppWebhookSecret) < 32 {
			return errors.New("CLICKCLACK_GITHUB_APP_WEBHOOK_SECRET must be at least 32 characters")
		}
		privateKeyPEM, err := base64.StdEncoding.DecodeString(githubAppPrivateKeyBase64)
		if err != nil {
			return errors.New("CLICKCLACK_GITHUB_APP_PRIVATE_KEY_BASE64 must be valid base64")
		}
		if err := validateGitHubAppPrivateKey(privateKeyPEM); err != nil {
			return fmt.Errorf("CLICKCLACK_GITHUB_APP_PRIVATE_KEY_BASE64: %w", err)
		}
	}
	if _, err := authpolicy.NewCookieNames(namespace, publicURL, publicAPIURL); err != nil {
		return fmt.Errorf("cookie policy: %w", err)
	}
	c.PublicAPIURL = publicAPIURL
	c.EmbedFrameAncestors = embedFrameAncestors
	c.CookieNamespace = namespace
	c.PublicURL = publicURL
	c.GitHubClientID = clientID
	c.GitHubClientSecret = clientSecret
	c.GitHubAllowedOrg = allowedOrg
	c.GitHubModeratorOrg = moderatorOrg
	c.GitHubAppSlug = githubAppSlug
	c.GitHubAppClientID = githubAppClientID
	c.GitHubAppClientSecret = githubAppClientSecret
	c.GitHubAppPrivateKeyBase64 = githubAppPrivateKeyBase64
	c.GitHubAppWebhookSecret = githubAppWebhookSecret
	return nil
}

func validateGitHubAppPrivateKey(value []byte) error {
	block, _ := pem.Decode(value)
	if block == nil {
		return errors.New("decoded value must contain a PEM private key")
	}
	if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return errors.New("decoded value must contain an RSA private key")
	}
	if _, ok := key.(*rsa.PrivateKey); !ok {
		return errors.New("decoded value must contain an RSA private key")
	}
	return nil
}

// ParseEmbedFrameAncestors parses the comma- or whitespace-separated format
// accepted by CLICKCLACK_EMBED_FRAME_ANCESTORS and --embed-frame-ancestors.
func ParseEmbedFrameAncestors(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

func normalizeEmbedFrameAncestors(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Hostname() == "" ||
			strings.Contains(parsed.Hostname(), "*") ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") ||
			(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("%q must be an HTTP(S) origin without a path, query, or fragment", value)
		}
		origin := (&url.URL{Scheme: strings.ToLower(parsed.Scheme), Host: strings.ToLower(parsed.Host)}).String()
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		normalized = append(normalized, origin)
	}
	return normalized, nil
}

func validatePublicURLPair(publicURL, publicAPIURL string) error {
	if publicURL == "" || publicAPIURL == "" {
		return nil
	}
	frontend, err := authpolicy.CanonicalPublicURL(publicURL)
	if err != nil {
		return fmt.Errorf("CLICKCLACK_PUBLIC_URL: %w", err)
	}
	api, err := authpolicy.CanonicalPublicAPIURL(publicAPIURL)
	if err != nil {
		return fmt.Errorf("CLICKCLACK_PUBLIC_API_URL: %w", err)
	}
	if strings.HasPrefix(frontend, "http://") || strings.HasPrefix(api, "http://") {
		frontendURL, _ := url.Parse(frontend)
		apiURL, _ := url.Parse(api)
		if frontendURL.Scheme != "http" || apiURL.Scheme != "http" || !strings.EqualFold(frontendURL.Hostname(), apiURL.Hostname()) {
			return errors.New("loopback split origins must both use HTTP and the same hostname")
		}
	}
	return nil
}
