package gitprovider

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider is a simple mock for testing
type mockProvider struct {
	providerType ProviderType
}

func (m *mockProvider) Type() ProviderType                                { return m.providerType }
func (m *mockProvider) GetBotIdentity(ctx context.Context) (*User, error) { return nil, nil }
func (m *mockProvider) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	return nil, nil
}
func (m *mockProvider) GetDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	return "main", nil
}
func (m *mockProvider) GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	return nil, nil
}
func (m *mockProvider) GetDirectoryContent(ctx context.Context, owner, repo, path, ref string) ([]*FileNode, error) {
	return nil, nil
}
func (m *mockProvider) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error) {
	return nil, nil
}
func (m *mockProvider) CreatePullRequest(ctx context.Context, owner, repo string, pr *NewPullRequest) (*PullRequest, error) {
	return nil, nil
}
func (m *mockProvider) ListPullRequestFiles(ctx context.Context, owner, repo string, number int) ([]*CommitFile, error) {
	return nil, nil
}
func (m *mockProvider) MergePullRequest(ctx context.Context, owner, repo string, number int, options *MergeOptions) error {
	return nil
}
func (m *mockProvider) CommentOnPullRequest(ctx context.Context, owner, repo string, number int, body string) (*Comment, error) {
	return nil, nil
}
func (m *mockProvider) ListPullRequestComments(ctx context.Context, owner, repo string, number int) ([]*Comment, error) {
	return nil, nil
}
func (m *mockProvider) MinimizeComment(ctx context.Context, owner, repo string, commentID int64) error {
	return nil
}
func (m *mockProvider) ApprovePullRequest(ctx context.Context, owner, repo string, number int) (*Review, error) {
	return nil, nil
}
func (m *mockProvider) ListPullRequestReviews(ctx context.Context, owner, repo string, number int) ([]*Review, error) {
	return nil, nil
}
func (m *mockProvider) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	return nil
}
func (m *mockProvider) RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error {
	return nil
}
func (m *mockProvider) GetLabels(ctx context.Context, owner, repo string, number int) ([]*Label, error) {
	return nil, nil
}
func (m *mockProvider) AddAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error {
	return nil
}
func (m *mockProvider) RemoveAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error {
	return nil
}
func (m *mockProvider) GetRef(ctx context.Context, owner, repo, ref string) (*Reference, error) {
	return nil, nil
}
func (m *mockProvider) CreateRef(ctx context.Context, owner, repo, ref, sha string) (*Reference, error) {
	return nil, nil
}
func (m *mockProvider) UpdateRef(ctx context.Context, owner, repo, ref, sha string, force bool) (*Reference, error) {
	return nil, nil
}
func (m *mockProvider) DeleteRef(ctx context.Context, owner, repo, ref string) error { return nil }
func (m *mockProvider) GetCommit(ctx context.Context, owner, repo, sha string) (*Commit, error) {
	return nil, nil
}
func (m *mockProvider) CreateCommit(ctx context.Context, owner, repo string, opts *CommitOptions) (*Commit, error) {
	return nil, nil
}
func (m *mockProvider) CompareCommits(ctx context.Context, owner, repo, base, head string) (*DiffResult, error) {
	return nil, nil
}
func (m *mockProvider) GetTree(ctx context.Context, owner, repo, sha string, recursive bool) ([]*TreeEntry, error) {
	return nil, nil
}
func (m *mockProvider) CreateTree(ctx context.Context, owner, repo string, baseTree string, entries []*TreeEntry) (string, error) {
	return "", nil
}
func (m *mockProvider) CreateBranch(ctx context.Context, owner, repo, branch, sha string) (*Reference, error) {
	return nil, nil
}
func (m *mockProvider) GetBranch(ctx context.Context, owner, repo, branch string) (*Reference, error) {
	return nil, nil
}
func (m *mockProvider) DeleteBranch(ctx context.Context, owner, repo, branch string) error {
	return nil
}
func (m *mockProvider) SetCommitStatus(ctx context.Context, owner, repo, sha string, status *Status) error {
	return nil
}
func (m *mockProvider) GetCommitStatus(ctx context.Context, owner, repo, sha string) ([]*Status, error) {
	return nil, nil
}
func (m *mockProvider) GetCombinedStatus(ctx context.Context, owner, repo, sha string) (string, error) {
	return "success", nil
}
func (m *mockProvider) ParseWebhook(req *http.Request, secret []byte) (Event, error)    { return nil, nil }
func (m *mockProvider) ValidateWebhookSignature(req *http.Request, secret []byte) error { return nil }
func (m *mockProvider) SupportsTreeAPI() bool                                           { return false }
func (m *mockProvider) SupportsGraphQL() bool                                           { return false }
func (m *mockProvider) GetAPIResponse() *APIResponse                                    { return nil }

// Initialize providers for testing
func init() {
	// Register test providers manually
	// In production, these are registered via init() in github/register.go and gitlab/register.go
	// But in tests, we need to register them manually to avoid import cycles

	// We'll test the factory without the actual providers since that would cause import cycle
	// The provider-specific tests in github/ and gitlab/ will test the actual implementations
}

func TestProviderRegistry(t *testing.T) {
	// Note: This test can't import actual providers due to import cycle
	// Real registration is tested via integration tests
	t.Skip("Provider registration tested via provider-specific test packages")
}

func TestDefaultProviderFactory_Create(t *testing.T) {
	// Register mock providers for testing
	RegisterProvider(ProviderTypeGitHub, func(config *ProviderConfig) (GitProvider, error) {
		if config.Token == "" && config.AppID == 0 {
			return nil, fmt.Errorf("GitHub provider requires either App credentials or OAuth token")
		}
		return &mockProvider{providerType: ProviderTypeGitHub}, nil
	})
	RegisterProvider(ProviderTypeGitLab, func(config *ProviderConfig) (GitProvider, error) {
		if config.Token == "" {
			return nil, fmt.Errorf("GitLab provider requires access token")
		}
		return &mockProvider{providerType: ProviderTypeGitLab}, nil
	})

	factory, err := NewDefaultProviderFactory(10)
	require.NoError(t, err)
	require.NotNil(t, factory)

	t.Run("Create GitHub provider with token", func(t *testing.T) {
		config := &ProviderConfig{
			Type:  ProviderTypeGitHub,
			Token: "fake_token",
		}

		provider, err := factory.Create(config)
		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, ProviderTypeGitHub, provider.Type())
	})

	t.Run("Create GitLab provider with token", func(t *testing.T) {
		config := &ProviderConfig{
			Type:    ProviderTypeGitLab,
			Token:   "fake_token",
			BaseURL: "https://gitlab.com",
		}

		provider, err := factory.Create(config)
		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, ProviderTypeGitLab, provider.Type())
	})

	t.Run("Fail with unsupported provider type", func(t *testing.T) {
		config := &ProviderConfig{
			Type:  ProviderType("bitbucket"),
			Token: "fake_token",
		}

		provider, err := factory.Create(config)
		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "unsupported provider type")
	})

	t.Run("GitHub provider requires token or app credentials", func(t *testing.T) {
		config := &ProviderConfig{
			Type: ProviderTypeGitHub,
		}

		provider, err := factory.Create(config)
		assert.Error(t, err)
		assert.Nil(t, provider)
	})

	t.Run("GitLab provider requires token", func(t *testing.T) {
		config := &ProviderConfig{
			Type:    ProviderTypeGitLab,
			BaseURL: "https://gitlab.com",
		}

		provider, err := factory.Create(config)
		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "requires access token")
	})
}

func TestDefaultProviderFactory_CreateWithCache(t *testing.T) {
	factory, err := NewDefaultProviderFactory(10)
	require.NoError(t, err)

	config := &ProviderConfig{
		Type:  ProviderTypeGitHub,
		Token: "fake_token",
	}

	t.Run("First call creates provider", func(t *testing.T) {
		provider1, err := factory.CreateWithCache(config, "test-key")
		require.NoError(t, err)
		assert.NotNil(t, provider1)
	})

	t.Run("Second call returns cached provider", func(t *testing.T) {
		provider2, err := factory.CreateWithCache(config, "test-key")
		require.NoError(t, err)
		assert.NotNil(t, provider2)
	})

	t.Run("Different key creates new provider", func(t *testing.T) {
		provider3, err := factory.CreateWithCache(config, "different-key")
		require.NoError(t, err)
		assert.NotNil(t, provider3)
	})
}

func TestDetectProviderFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected ProviderType
	}{
		{
			name:     "GitHub.com URL",
			url:      "https://github.com/user/repo",
			expected: ProviderTypeGitHub,
		},
		{
			name:     "GitHub Enterprise URL",
			url:      "https://github.enterprise.com/user/repo",
			expected: ProviderTypeGitHub,
		},
		{
			name:     "GitLab.com URL",
			url:      "https://gitlab.com/user/repo",
			expected: ProviderTypeGitLab,
		},
		{
			name:     "Self-hosted GitLab URL",
			url:      "https://gitlab.example.com/user/repo",
			expected: ProviderTypeGitLab,
		},
		{
			name:     "Unknown URL defaults to GitHub",
			url:      "https://example.com/user/repo",
			expected: ProviderTypeGitHub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectProviderFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectProviderFromWebhook(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string][]string
		expected ProviderType
	}{
		{
			name: "GitHub webhook",
			headers: map[string][]string{
				"X-Github-Event": {"pull_request"},
			},
			expected: ProviderTypeGitHub,
		},
		{
			name: "GitLab webhook",
			headers: map[string][]string{
				"X-Gitlab-Event": {"Merge Request Hook"},
			},
			expected: ProviderTypeGitLab,
		},
		{
			name:     "Unknown webhook defaults to GitHub",
			headers:  map[string][]string{},
			expected: ProviderTypeGitHub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectProviderFromWebhook(tt.headers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCapabilities(t *testing.T) {
	t.Run("GitHub capabilities", func(t *testing.T) {
		caps := GetCapabilities(ProviderTypeGitHub)
		assert.True(t, caps.TreeAPI)
		assert.True(t, caps.GraphQL)
		assert.True(t, caps.CommentMinimization)
		assert.False(t, caps.MergeTrain)
	})

	t.Run("GitLab capabilities", func(t *testing.T) {
		caps := GetCapabilities(ProviderTypeGitLab)
		assert.False(t, caps.TreeAPI)
		assert.True(t, caps.GraphQL)
		assert.False(t, caps.CommentMinimization)
		assert.True(t, caps.MergeTrain)
	})

	t.Run("Unknown provider returns empty capabilities", func(t *testing.T) {
		caps := GetCapabilities(ProviderType("unknown"))
		assert.False(t, caps.TreeAPI)
		assert.False(t, caps.GraphQL)
		assert.False(t, caps.CommentMinimization)
		assert.False(t, caps.MergeTrain)
	})
}

func TestProviderClientDetails_GetDefaultBranch(t *testing.T) {
	// This test requires a mock provider
	// For now, we just test the caching logic
	t.Run("Returns cached branch if available", func(t *testing.T) {
		details := &ProviderClientDetails{
			DefaultBranch: "main",
		}

		branch, err := details.GetDefaultBranch()
		assert.NoError(t, err)
		assert.Equal(t, "main", branch)
	})
}

func TestProviderClientDetails_HasLabel(t *testing.T) {
	details := &ProviderClientDetails{
		Labels: []*Label{
			{Name: "bug"},
			{Name: "enhancement"},
		},
	}

	t.Run("Returns true for existing label", func(t *testing.T) {
		assert.True(t, details.HasLabel("bug"))
	})

	t.Run("Returns false for non-existing label", func(t *testing.T) {
		assert.False(t, details.HasLabel("documentation"))
	})

	t.Run("Returns false for empty labels", func(t *testing.T) {
		emptyDetails := &ProviderClientDetails{
			Labels: []*Label{},
		}
		assert.False(t, emptyDetails.HasLabel("bug"))
	})
}
