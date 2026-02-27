package githubapi

import (
	"context"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	ghprovider "github.com/commercetools/telefonistka/internal/pkg/gitprovider/github"
	"github.com/google/go-github/v62/github"
	lru "github.com/hashicorp/golang-lru/v2"
	log "github.com/sirupsen/logrus"
)

// NewGhPrClientDetailsFromProvider creates a GhPrClientDetails from a GitProvider
// This is a bridge function to help with migration
func NewGhPrClientDetailsFromProvider(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo string,
	prNumber int,
	prSHA, ref string,
	prAuthor string,
	repoURL string,
	labels []*github.Label,
	prLogger *log.Entry,
) GhPrClientDetails {
	// For GitHub provider, we need to extract the underlying clients
	var ghClientPair *GhClientPair

	if ghProvider, ok := provider.(*ghprovider.GitHubProvider); ok {
		// Extract the actual GitHub clients
		ghClientPair = &GhClientPair{
			v3Client: ghProvider.GetV3Client(),
			v4Client: ghProvider.GetV4Client(),
		}
	} else {
		// For non-GitHub providers, we can't create GhPrClientDetails
		// This should not happen in practice during migration
		log.Fatalf("Cannot create GhPrClientDetails from non-GitHub provider")
	}

	defaultBranch, _ := provider.GetDefaultBranch(ctx, owner, repo)

	return GhPrClientDetails{
		GhClientPair:  ghClientPair,
		Ctx:           ctx,
		DefaultBranch: defaultBranch,
		Owner:         owner,
		Repo:          repo,
		PrAuthor:      prAuthor,
		PrNumber:      prNumber,
		PrSHA:         prSHA,
		Ref:           ref,
		RepoURL:       repoURL,
		PrLogger:      prLogger,
		Labels:        labels,
	}
}

// GetProviderFromCache gets or creates a GitProvider with caching
func GetProviderFromCache(
	ctx context.Context,
	providerCache *lru.Cache[string, gitprovider.GitProvider],
	repoOwner string,
	ghAppIdEnvVarName string,
	ghAppPKeyPathEnvVarName string,
	ghOauthTokenEnvVarName string,
) (gitprovider.GitProvider, error) {
	// Check cache first
	cacheKey := repoOwner
	if provider, ok := providerCache.Get(cacheKey); ok {
		log.Debugf("Found cached provider for %s", cacheKey)
		return provider, nil
	}

	// Create factory
	factory, err := gitprovider.NewDefaultProviderFactory(128)
	if err != nil {
		return nil, err
	}

	// Use legacy env var pattern
	provider, err := gitprovider.CreateProviderFromEnvLegacy(
		ctx,
		factory,
		repoOwner,
		ghAppIdEnvVarName,
		ghAppPKeyPathEnvVarName,
		ghOauthTokenEnvVarName,
	)
	if err != nil {
		return nil, err
	}

	// Cache it
	providerCache.Add(cacheKey, provider)
	return provider, nil
}

// MigrateGhClientCacheToProviderCache converts old GhClientPair cache to new provider cache
func MigrateGhClientCacheToProviderCache(
	ghClientCache *lru.Cache[string, GhClientPair],
) *lru.Cache[string, gitprovider.GitProvider] {
	providerCache, _ := lru.New[string, gitprovider.GitProvider](128)

	// We can't directly migrate because we'd need to wrap each GhClientPair
	// Instead, the new cache will be populated as needed

	return providerCache
}
