package gitprovider

import (
	"context"
	"fmt"
	"os"
	"strconv"

	lru "github.com/hashicorp/golang-lru/v2"
	log "github.com/sirupsen/logrus"
)

// DefaultProviderFactory is the default implementation of ProviderFactory
type DefaultProviderFactory struct {
	cache *lru.Cache[string, GitProvider]
}

// NewDefaultProviderFactory creates a new provider factory with caching
func NewDefaultProviderFactory(cacheSize int) (*DefaultProviderFactory, error) {
	cache, err := lru.New[string, GitProvider](cacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider cache: %w", err)
	}

	return &DefaultProviderFactory{
		cache: cache,
	}, nil
}

// GitProviderConstructor is a function type for creating providers
type GitProviderConstructor func(config *ProviderConfig) (GitProvider, error)

// providerRegistry stores the constructors for each provider type
var providerRegistry = make(map[ProviderType]GitProviderConstructor)

// RegisterProvider registers a provider constructor
func RegisterProvider(providerType ProviderType, constructor GitProviderConstructor) {
	providerRegistry[providerType] = constructor
}

// Create creates a GitProvider instance based on configuration
func (f *DefaultProviderFactory) Create(config *ProviderConfig) (GitProvider, error) {
	constructor, ok := providerRegistry[config.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported provider type: %s", config.Type)
	}
	return constructor(config)
}

// CreateFromEnv creates a GitProvider from environment variables
// This maintains backward compatibility with existing deployments
func (f *DefaultProviderFactory) CreateFromEnv(providerType ProviderType) (GitProvider, error) {
	var config *ProviderConfig

	switch providerType {
	case ProviderTypeGitHub:
		config = f.getGitHubConfigFromEnv()
	case ProviderTypeGitLab:
		config = f.getGitLabConfigFromEnv()
	default:
		// Auto-detect based on environment variables
		if os.Getenv("GITLAB_TOKEN") != "" || os.Getenv("GITLAB_URL") != "" {
			config = f.getGitLabConfigFromEnv()
			config.Type = ProviderTypeGitLab
		} else {
			config = f.getGitHubConfigFromEnv()
			config.Type = ProviderTypeGitHub
		}
	}

	return f.Create(config)
}

// CreateWithCache returns a cached provider if available, otherwise creates a new one
func (f *DefaultProviderFactory) CreateWithCache(config *ProviderConfig, cacheKey string) (GitProvider, error) {
	// Check cache first
	if provider, ok := f.cache.Get(cacheKey); ok {
		log.Debugf("Found cached provider for key: %s", cacheKey)
		return provider, nil
	}

	// Create new provider
	log.Infof("Creating new provider for key: %s", cacheKey)
	provider, err := f.Create(config)
	if err != nil {
		return nil, err
	}

	// Cache it
	f.cache.Add(cacheKey, provider)
	return provider, nil
}

// getGitHubConfigFromEnv reads GitHub configuration from environment variables
func (f *DefaultProviderFactory) getGitHubConfigFromEnv() *ProviderConfig {
	config := &ProviderConfig{
		Type:          ProviderTypeGitHub,
		BaseURL:       os.Getenv("GITHUB_HOST"),
		Token:         os.Getenv("GITHUB_OAUTH_TOKEN"),
		WebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
	}

	// GitHub App configuration
	if appIDStr := os.Getenv("GITHUB_APP_ID"); appIDStr != "" {
		appID, err := strconv.ParseInt(appIDStr, 10, 64)
		if err == nil {
			config.AppID = appID
			config.PrivateKeyPath = os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")

			if installIDStr := os.Getenv("GITHUB_APP_INSTALLATION_ID"); installIDStr != "" {
				installID, err := strconv.ParseInt(installIDStr, 10, 64)
				if err == nil {
					config.InstallationID = installID
				}
			}
		}
	}

	return config
}

// getGitLabConfigFromEnv reads GitLab configuration from environment variables
func (f *DefaultProviderFactory) getGitLabConfigFromEnv() *ProviderConfig {
	config := &ProviderConfig{
		Type:          ProviderTypeGitLab,
		BaseURL:       getEnv("GITLAB_URL", "https://gitlab.com"),
		Token:         os.Getenv("GITLAB_TOKEN"),
		ProjectID:     os.Getenv("GITLAB_PROJECT_ID"),
		WebhookSecret: os.Getenv("GITLAB_WEBHOOK_SECRET"),
	}

	// Allow insecure TLS for self-hosted instances (not recommended for production)
	if os.Getenv("GITLAB_INSECURE") == "true" {
		config.Insecure = true
	}

	return config
}

// getEnv gets an environment variable with a fallback value
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// getCrucialEnv gets a required environment variable or panics
func getCrucialEnv(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	log.Fatalf("%s environment variable is required", key)
	return ""
}

// CreateProviderFromEnvLegacy creates a provider using the legacy environment variable pattern
// This maintains backward compatibility with the existing GhClientPair.GetAndCache pattern
func CreateProviderFromEnvLegacy(
	ctx context.Context,
	factory *DefaultProviderFactory,
	repoOwner string,
	ghAppIdEnvVarName string,
	ghAppPKeyPathEnvVarName string,
	ghOauthTokenEnvVarName string,
) (GitProvider, error) {
	// Check if this is GitHub App or OAuth token
	githubAppId := getEnv(ghAppIdEnvVarName, "")
	cacheKey := repoOwner

	var config *ProviderConfig

	if githubAppId != "" {
		// GitHub App authentication
		appID, err := strconv.ParseInt(githubAppId, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid GITHUB_APP_ID: %w", err)
		}

		config = &ProviderConfig{
			Type:           ProviderTypeGitHub,
			BaseURL:        os.Getenv("GITHUB_HOST"),
			AppID:          appID,
			PrivateKeyPath: os.Getenv(ghAppPKeyPathEnvVarName),
			WebhookSecret:  os.Getenv("GITHUB_WEBHOOK_SECRET"),
		}
		cacheKey = repoOwner
	} else {
		// OAuth token authentication
		token := os.Getenv(ghOauthTokenEnvVarName)
		if token == "" {
			return nil, fmt.Errorf("neither %s nor %s is set", ghAppIdEnvVarName, ghOauthTokenEnvVarName)
		}

		config = &ProviderConfig{
			Type:          ProviderTypeGitHub,
			BaseURL:       os.Getenv("GITHUB_HOST"),
			Token:         token,
			WebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		}
		cacheKey = "global"
	}

	return factory.CreateWithCache(config, cacheKey)
}

// DetectProviderFromURL attempts to detect the provider type from a repository URL
func DetectProviderFromURL(repoURL string) ProviderType {
	if containsAny(repoURL, []string{"github.com", "github.enterprise"}) {
		return ProviderTypeGitHub
	}
	if containsAny(repoURL, []string{"gitlab.com", "gitlab"}) {
		return ProviderTypeGitLab
	}
	return ProviderTypeGitHub // Default to GitHub for backward compatibility
}

// DetectProviderFromWebhook attempts to detect the provider from webhook headers
func DetectProviderFromWebhook(headers map[string][]string) ProviderType {
	// GitHub uses X-GitHub-Event header
	if _, ok := headers["X-Github-Event"]; ok {
		return ProviderTypeGitHub
	}

	// GitLab uses X-Gitlab-Event header
	if _, ok := headers["X-Gitlab-Event"]; ok {
		return ProviderTypeGitLab
	}

	return ProviderTypeGitHub // Default to GitHub for backward compatibility
}

// containsAny checks if a string contains any of the substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if contains(s, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSlowPath(s, substr))
}

func containsSlowPath(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// NewGitHubProvider and NewGitLabProvider are implemented in their respective packages
// We can't import them here due to circular dependencies, so we use reflection-like patterns
// or direct imports in the factory methods above
