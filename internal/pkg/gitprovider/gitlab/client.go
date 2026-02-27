package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	log "github.com/sirupsen/logrus"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// GitLabProvider implements the GitProvider interface for GitLab
type GitLabProvider struct {
	client       *gitlab.Client
	config       *gitprovider.ProviderConfig
	projectID    interface{} // Can be int or string ("group/project")
	lastResponse *gitprovider.APIResponse
}

// NewGitLabProvider creates a new GitLab provider instance
func NewGitLabProvider(config *gitprovider.ProviderConfig) (*GitLabProvider, error) {
	if config.Token == "" {
		return nil, fmt.Errorf("GitLab provider requires access token")
	}

	// Determine base URL
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create GitLab client
	client, err := gitlab.NewClient(
		config.Token,
		gitlab.WithBaseURL(baseURL),
		gitlab.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	// Parse project ID (can be numeric or "group/project")
	var projectID interface{}
	if config.ProjectID != "" {
		// Try to parse as integer first
		if id, err := strconv.Atoi(config.ProjectID); err == nil {
			projectID = id
		} else {
			// Use as string (group/project format)
			projectID = config.ProjectID
		}
	}

	log.Infof("GitLab client created for %s", baseURL)

	return &GitLabProvider{
		client:    client,
		config:    config,
		projectID: projectID,
	}, nil
}

// Type returns the provider type
func (g *GitLabProvider) Type() gitprovider.ProviderType {
	return gitprovider.ProviderTypeGitLab
}

// GetBotIdentity returns the bot's identity
func (g *GitLabProvider) GetBotIdentity(ctx context.Context) (*gitprovider.User, error) {
	user, resp, err := g.client.Users.CurrentUser()
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	return &gitprovider.User{
		ID:      int64(user.ID),
		Login:   user.Username,
		Name:    user.Name,
		Email:   user.Email,
		HTMLURL: user.WebURL,
		IsBot:   user.Bot,
	}, nil
}

// GetRepository returns repository information
func (g *GitLabProvider) GetRepository(ctx context.Context, owner, repo string) (*gitprovider.Repository, error) {
	projectPath := fmt.Sprintf("%s/%s", owner, repo)

	project, resp, err := g.client.Projects.GetProject(projectPath, nil)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	return &gitprovider.Repository{
		ID:            int64(project.ID),
		Name:          project.Name,
		FullName:      project.PathWithNamespace,
		Owner:         project.Namespace.FullPath,
		DefaultBranch: project.DefaultBranch,
		Private:       project.Visibility != gitlab.PublicVisibility,
		HTMLURL:       project.WebURL,
		CloneURL:      project.HTTPURLToRepo,
	}, nil
}

// GetDefaultBranch returns the default branch name
func (g *GitLabProvider) GetDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	repository, err := g.GetRepository(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	return repository.DefaultBranch, nil
}

// GetFileContent returns the content of a file
func (g *GitLabProvider) GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	projectPath := fmt.Sprintf("%s/%s", owner, repo)

	opts := &gitlab.GetFileOptions{
		Ref: gitlab.Ptr(ref),
	}

	file, resp, err := g.client.RepositoryFiles.GetFile(projectPath, path, opts)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get file content: %w", err)
	}

	// GitLab returns base64-encoded content by default
	// The library automatically decodes it
	return []byte(file.Content), nil
}

// GetDirectoryContent returns the content of a directory
func (g *GitLabProvider) GetDirectoryContent(ctx context.Context, owner, repo, path, ref string) ([]*gitprovider.FileNode, error) {
	projectPath := fmt.Sprintf("%s/%s", owner, repo)

	opts := &gitlab.ListTreeOptions{
		Path: gitlab.Ptr(path),
		Ref:  gitlab.Ptr(ref),
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var allNodes []*gitprovider.FileNode

	for {
		tree, resp, err := g.client.Repositories.ListTree(projectPath, opts)
		g.updateLastResponse(resp)

		if err != nil {
			return nil, fmt.Errorf("failed to list tree: %w", err)
		}

		for _, item := range tree {
			nodeType := "file"
			switch item.Type {
			case "tree":
				nodeType = "dir"
			case "blob":
				nodeType = "file"
			case "commit":
				nodeType = "submodule"
			}

			allNodes = append(allNodes, &gitprovider.FileNode{
				Name: item.Name,
				Path: item.Path,
				Type: nodeType,
				SHA:  item.ID,
				// Size is not available in tree listing
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allNodes, nil
}

// updateLastResponse stores metadata from the last API call
func (g *GitLabProvider) updateLastResponse(resp *gitlab.Response) {
	if resp == nil {
		return
	}

	g.lastResponse = &gitprovider.APIResponse{
		StatusCode: resp.StatusCode,
		RequestID:  resp.Header.Get("X-Request-Id"),
	}

	// GitLab rate limit headers
	if limit := resp.Header.Get("RateLimit-Limit"); limit != "" {
		if limitInt, err := strconv.Atoi(limit); err == nil {
			if g.lastResponse.RateLimit == nil {
				g.lastResponse.RateLimit = &gitprovider.RateLimitInfo{}
			}
			g.lastResponse.RateLimit.Limit = limitInt
		}
	}

	if remaining := resp.Header.Get("RateLimit-Remaining"); remaining != "" {
		if remainingInt, err := strconv.Atoi(remaining); err == nil {
			if g.lastResponse.RateLimit == nil {
				g.lastResponse.RateLimit = &gitprovider.RateLimitInfo{}
			}
			g.lastResponse.RateLimit.Remaining = remainingInt
		}
	}

	if reset := resp.Header.Get("RateLimit-Reset"); reset != "" {
		if resetInt, err := strconv.ParseInt(reset, 10, 64); err == nil {
			if g.lastResponse.RateLimit == nil {
				g.lastResponse.RateLimit = &gitprovider.RateLimitInfo{}
			}
			g.lastResponse.RateLimit.Reset = time.Unix(resetInt, 0)
		}
	}
}

// GetAPIResponse returns metadata about the last API call
func (g *GitLabProvider) GetAPIResponse() *gitprovider.APIResponse {
	return g.lastResponse
}

// SupportsTreeAPI returns false (GitLab doesn't have GitHub-style tree API)
func (g *GitLabProvider) SupportsTreeAPI() bool {
	return false
}

// SupportsGraphQL returns true (GitLab supports GraphQL)
func (g *GitLabProvider) SupportsGraphQL() bool {
	return true
}

// Helper function to build project path
func (g *GitLabProvider) getProjectPath(owner, repo string) string {
	if g.projectID != nil {
		// Use configured project ID if available
		switch v := g.projectID.(type) {
		case int:
			return strconv.Itoa(v)
		case string:
			return v
		}
	}
	return fmt.Sprintf("%s/%s", owner, repo)
}

// GetClient returns the underlying GitLab client (for advanced use cases)
func (g *GitLabProvider) GetClient() *gitlab.Client {
	return g.client
}
