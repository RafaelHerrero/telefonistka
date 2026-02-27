package github

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	"github.com/google/go-github/v62/github"
	"github.com/shurcooL/githubv4"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

// GitHubProvider implements the GitProvider interface for GitHub
type GitHubProvider struct {
	v3Client     *github.Client
	v4Client     *githubv4.Client
	config       *gitprovider.ProviderConfig
	lastResponse *gitprovider.APIResponse
}

// NewGitHubProvider creates a new GitHub provider instance
func NewGitHubProvider(config *gitprovider.ProviderConfig) (*GitHubProvider, error) {
	provider := &GitHubProvider{
		config: config,
	}

	ctx := context.Background()

	// Determine authentication method
	if config.AppID != 0 {
		// GitHub App authentication
		if config.PrivateKeyPath == "" {
			return nil, fmt.Errorf("GitHub App authentication requires private key path")
		}

		// If installation ID is not provided, we'll need to discover it per-org
		// For now, require it
		if config.InstallationID == 0 {
			return nil, fmt.Errorf("GitHub App authentication requires installation ID")
		}

		v3Client, v4Client, err := createGitHubAppClients(
			config.AppID,
			config.InstallationID,
			config.PrivateKeyPath,
			config.BaseURL,
			ctx,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitHub App clients: %w", err)
		}

		provider.v3Client = v3Client
		provider.v4Client = v4Client
	} else if config.Token != "" {
		// OAuth token authentication
		v3Client, v4Client := createGitHubTokenClients(
			config.Token,
			config.BaseURL,
			ctx,
		)

		provider.v3Client = v3Client
		provider.v4Client = v4Client
	} else {
		return nil, fmt.Errorf("GitHub provider requires either App credentials or OAuth token")
	}

	return provider, nil
}

// createGitHubAppClients creates REST and GraphQL clients for GitHub App
func createGitHubAppClients(appID, installationID int64, privateKeyPath, baseURL string, ctx context.Context) (*github.Client, *githubv4.Client, error) {
	itr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, appID, installationID, privateKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GitHub installation transport: %w", err)
	}

	var v3Client *github.Client
	var v4Client *githubv4.Client

	if baseURL != "" {
		// GitHub Enterprise Server
		restURL := fmt.Sprintf("https://%s/api/v3", baseURL)
		graphqlURL := fmt.Sprintf("https://%s/api/graphql", baseURL)

		itr.BaseURL = restURL
		v3Client, _ = github.NewClient(&http.Client{Transport: itr}).WithEnterpriseURLs(restURL, restURL)
		v4Client = githubv4.NewEnterpriseClient(graphqlURL, &http.Client{Transport: itr})

		log.Infof("GitHub REST API endpoint: %s", restURL)
		log.Infof("GitHub GraphQL API endpoint: %s", graphqlURL)
	} else {
		// GitHub.com
		v3Client = github.NewClient(&http.Client{Transport: itr})
		v4Client = githubv4.NewClient(&http.Client{Transport: itr})
		log.Debug("Using public GitHub API endpoints")
	}

	return v3Client, v4Client, nil
}

// createGitHubTokenClients creates REST and GraphQL clients for OAuth token
func createGitHubTokenClients(token, baseURL string, ctx context.Context) (*github.Client, *githubv4.Client) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)

	var v3Client *github.Client
	var v4Client *githubv4.Client

	if baseURL != "" {
		// GitHub Enterprise Server
		restURL := fmt.Sprintf("https://%s/api/v3", baseURL)
		graphqlURL := fmt.Sprintf("https://%s/api/graphql", baseURL)

		v3Client, _ = github.NewClient(tc).WithEnterpriseURLs(restURL, restURL)
		v4Client = githubv4.NewEnterpriseClient(graphqlURL, tc)

		log.Infof("GitHub REST API endpoint: %s", restURL)
		log.Infof("GitHub GraphQL API endpoint: %s", graphqlURL)
	} else {
		// GitHub.com
		v3Client = github.NewClient(tc)
		v4Client = githubv4.NewClient(tc)
		log.Debug("Using public GitHub API endpoints")
	}

	return v3Client, v4Client
}

// Type returns the provider type
func (g *GitHubProvider) Type() gitprovider.ProviderType {
	return gitprovider.ProviderTypeGitHub
}

// GetBotIdentity returns the bot's identity (username)
func (g *GitHubProvider) GetBotIdentity(ctx context.Context) (*gitprovider.User, error) {
	var query struct {
		Viewer struct {
			Login githubv4.String
			ID    githubv4.ID
			Name  githubv4.String
			Email githubv4.String
			URL   githubv4.URI
			Type  githubv4.String
		}
	}

	err := g.v4Client.Query(ctx, &query, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get bot identity: %w", err)
	}

	return &gitprovider.User{
		Login:   string(query.Viewer.Login),
		Name:    string(query.Viewer.Name),
		Email:   string(query.Viewer.Email),
		HTMLURL: query.Viewer.URL.String(),
		Type:    string(query.Viewer.Type),
		IsBot:   string(query.Viewer.Type) == "Bot",
	}, nil
}

// GetRepository returns repository information
func (g *GitHubProvider) GetRepository(ctx context.Context, owner, repo string) (*gitprovider.Repository, error) {
	ghRepo, resp, err := g.v3Client.Repositories.Get(ctx, owner, repo)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return &gitprovider.Repository{
		ID:            ghRepo.GetID(),
		Name:          ghRepo.GetName(),
		FullName:      ghRepo.GetFullName(),
		Owner:         ghRepo.GetOwner().GetLogin(),
		DefaultBranch: ghRepo.GetDefaultBranch(),
		Private:       ghRepo.GetPrivate(),
		HTMLURL:       ghRepo.GetHTMLURL(),
		CloneURL:      ghRepo.GetCloneURL(),
	}, nil
}

// GetDefaultBranch returns the default branch name
func (g *GitHubProvider) GetDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	repository, err := g.GetRepository(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	return repository.DefaultBranch, nil
}

// GetFileContent returns the content of a file
func (g *GitHubProvider) GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	opts := &github.RepositoryContentGetOptions{
		Ref: ref,
	}

	fileContent, _, resp, err := g.v3Client.Repositories.GetContents(ctx, owner, repo, path, opts)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get file content: %w", err)
	}

	if fileContent == nil {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	content, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("failed to decode file content: %w", err)
	}

	return []byte(content), nil
}

// GetDirectoryContent returns the content of a directory
func (g *GitHubProvider) GetDirectoryContent(ctx context.Context, owner, repo, path, ref string) ([]*gitprovider.FileNode, error) {
	opts := &github.RepositoryContentGetOptions{
		Ref: ref,
	}

	_, dirContent, resp, err := g.v3Client.Repositories.GetContents(ctx, owner, repo, path, opts)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get directory content: %w", err)
	}

	var nodes []*gitprovider.FileNode
	for _, item := range dirContent {
		nodeType := "file"
		if item.GetType() == "dir" {
			nodeType = "dir"
		} else if item.GetType() == "symlink" {
			nodeType = "symlink"
		} else if item.GetType() == "submodule" {
			nodeType = "submodule"
		}

		nodes = append(nodes, &gitprovider.FileNode{
			Name:    item.GetName(),
			Path:    item.GetPath(),
			Type:    nodeType,
			Size:    item.GetSize(),
			SHA:     item.GetSHA(),
			HTMLURL: item.GetHTMLURL(),
		})
	}

	return nodes, nil
}

// updateLastResponse stores metadata from the last API call
func (g *GitHubProvider) updateLastResponse(resp *github.Response) {
	if resp == nil {
		return
	}

	g.lastResponse = &gitprovider.APIResponse{
		StatusCode: resp.StatusCode,
		RequestID:  resp.Header.Get("X-Request-Id"),
	}

	if resp.Rate.Limit > 0 {
		g.lastResponse.RateLimit = &gitprovider.RateLimitInfo{
			Limit:     resp.Rate.Limit,
			Remaining: resp.Rate.Remaining,
			Reset:     resp.Rate.Reset.Time,
		}
	}
}

// GetAPIResponse returns metadata about the last API call
func (g *GitHubProvider) GetAPIResponse() *gitprovider.APIResponse {
	return g.lastResponse
}

// SupportsTreeAPI returns true (GitHub supports tree API)
func (g *GitHubProvider) SupportsTreeAPI() bool {
	return true
}

// SupportsGraphQL returns true (GitHub supports GraphQL)
func (g *GitHubProvider) SupportsGraphQL() bool {
	return true
}

// Helper function to convert GitHub label to provider label
func convertGitHubLabel(label *github.Label) *gitprovider.Label {
	if label == nil {
		return nil
	}
	return &gitprovider.Label{
		ID:          label.GetID(),
		Name:        label.GetName(),
		Color:       label.GetColor(),
		Description: label.GetDescription(),
	}
}

// Helper function to convert GitHub labels to provider labels
func convertGitHubLabels(labels []*github.Label) []string {
	var result []string
	for _, label := range labels {
		if label != nil {
			result = append(result, label.GetName())
		}
	}
	return result
}

// Helper function to convert time pointer
func convertTimePtr(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// GetV3Client returns the underlying GitHub REST client (for backward compatibility)
func (g *GitHubProvider) GetV3Client() *github.Client {
	return g.v3Client
}

// GetV4Client returns the underlying GitHub GraphQL client (for backward compatibility)
func (g *GitHubProvider) GetV4Client() *githubv4.Client {
	return g.v4Client
}
