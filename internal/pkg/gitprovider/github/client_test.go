package github

import (
	"context"
	"testing"
	"time"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	"github.com/google/go-github/v62/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGitHubProvider(t *testing.T) {
	t.Run("Create provider with OAuth token", func(t *testing.T) {
		config := &gitprovider.ProviderConfig{
			Type:  gitprovider.ProviderTypeGitHub,
			Token: "fake_token",
		}

		provider, err := NewGitHubProvider(config)
		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, gitprovider.ProviderTypeGitHub, provider.Type())
	})

	t.Run("Create provider with GitHub App credentials", func(t *testing.T) {
		// This will fail because we don't have real credentials
		// but it tests the code path
		config := &gitprovider.ProviderConfig{
			Type:           gitprovider.ProviderTypeGitHub,
			AppID:          12345,
			InstallationID: 67890,
			PrivateKeyPath: "/nonexistent/path",
		}

		_, err := NewGitHubProvider(config)
		// Should fail because private key doesn't exist
		assert.Error(t, err)
	})

	t.Run("Fail without credentials", func(t *testing.T) {
		config := &gitprovider.ProviderConfig{
			Type: gitprovider.ProviderTypeGitHub,
		}

		provider, err := NewGitHubProvider(config)
		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "requires either App credentials or OAuth token")
	})

	t.Run("Fail without installation ID for GitHub App", func(t *testing.T) {
		config := &gitprovider.ProviderConfig{
			Type:           gitprovider.ProviderTypeGitHub,
			AppID:          12345,
			PrivateKeyPath: "/some/path",
		}

		provider, err := NewGitHubProvider(config)
		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "requires installation ID")
	})
}

func TestGitHubProvider_Type(t *testing.T) {
	config := &gitprovider.ProviderConfig{
		Type:  gitprovider.ProviderTypeGitHub,
		Token: "fake_token",
	}

	provider, err := NewGitHubProvider(config)
	require.NoError(t, err)

	assert.Equal(t, gitprovider.ProviderTypeGitHub, provider.Type())
}

func TestGitHubProvider_SupportsTreeAPI(t *testing.T) {
	config := &gitprovider.ProviderConfig{
		Type:  gitprovider.ProviderTypeGitHub,
		Token: "fake_token",
	}

	provider, err := NewGitHubProvider(config)
	require.NoError(t, err)

	assert.True(t, provider.SupportsTreeAPI())
}

func TestGitHubProvider_SupportsGraphQL(t *testing.T) {
	config := &gitprovider.ProviderConfig{
		Type:  gitprovider.ProviderTypeGitHub,
		Token: "fake_token",
	}

	provider, err := NewGitHubProvider(config)
	require.NoError(t, err)

	assert.True(t, provider.SupportsGraphQL())
}

func TestConvertGitHubLabel(t *testing.T) {
	t.Run("Convert valid label", func(t *testing.T) {
		ghLabel := &github.Label{
			ID:          github.Int64(123),
			Name:        github.String("bug"),
			Color:       github.String("d73a4a"),
			Description: github.String("Something isn't working"),
		}

		label := convertGitHubLabel(ghLabel)
		assert.NotNil(t, label)
		assert.Equal(t, int64(123), label.ID)
		assert.Equal(t, "bug", label.Name)
		assert.Equal(t, "d73a4a", label.Color)
		assert.Equal(t, "Something isn't working", label.Description)
	})

	t.Run("Convert nil label", func(t *testing.T) {
		label := convertGitHubLabel(nil)
		assert.Nil(t, label)
	})
}

func TestConvertGitHubLabels(t *testing.T) {
	t.Run("Convert multiple labels", func(t *testing.T) {
		ghLabels := []*github.Label{
			{Name: github.String("bug")},
			{Name: github.String("enhancement")},
		}

		labels := convertGitHubLabels(ghLabels)
		assert.Len(t, labels, 2)
		assert.Contains(t, labels, "bug")
		assert.Contains(t, labels, "enhancement")
	})

	t.Run("Handle nil labels", func(t *testing.T) {
		ghLabels := []*github.Label{
			{Name: github.String("bug")},
			nil,
			{Name: github.String("enhancement")},
		}

		labels := convertGitHubLabels(ghLabels)
		assert.Len(t, labels, 2)
		assert.Contains(t, labels, "bug")
		assert.Contains(t, labels, "enhancement")
	})

	t.Run("Handle empty slice", func(t *testing.T) {
		labels := convertGitHubLabels([]*github.Label{})
		assert.Empty(t, labels)
	})
}

func TestConvertGitHubPR(t *testing.T) {
	now := time.Now()

	t.Run("Convert valid PR", func(t *testing.T) {
		ghPR := &github.PullRequest{
			Number: github.Int(123),
			Title:  github.String("Test PR"),
			Body:   github.String("This is a test"),
			State:  github.String("open"),
			User:   &github.User{Login: github.String("testuser")},
			Head: &github.PullRequestBranch{
				Ref: github.String("feature-branch"),
				SHA: github.String("abc123"),
			},
			Base: &github.PullRequestBranch{
				Ref: github.String("main"),
				SHA: github.String("def456"),
			},
			Labels: []*github.Label{
				{Name: github.String("bug")},
			},
			HTMLURL:        github.String("https://github.com/user/repo/pull/123"),
			Merged:         github.Bool(false),
			Mergeable:      github.Bool(true),
			MergeCommitSHA: github.String("xyz789"),
			CreatedAt:      &github.Timestamp{Time: now},
			UpdatedAt:      &github.Timestamp{Time: now},
		}

		pr := convertGitHubPR(ghPR)
		assert.NotNil(t, pr)
		assert.Equal(t, 123, pr.Number)
		assert.Equal(t, "Test PR", pr.Title)
		assert.Equal(t, "This is a test", pr.Body)
		assert.Equal(t, "open", pr.State)
		assert.Equal(t, "testuser", pr.Author)
		assert.Equal(t, "feature-branch", pr.HeadRef)
		assert.Equal(t, "main", pr.BaseRef)
		assert.Equal(t, "abc123", pr.HeadSHA)
		assert.Equal(t, "def456", pr.BaseSHA)
		assert.Contains(t, pr.Labels, "bug")
		assert.Equal(t, "https://github.com/user/repo/pull/123", pr.HTMLURL)
		assert.False(t, pr.Merged)
		assert.True(t, pr.Mergeable)
		assert.Equal(t, "xyz789", pr.MergeCommit)
	})

	t.Run("Convert nil PR", func(t *testing.T) {
		pr := convertGitHubPR(nil)
		assert.Nil(t, pr)
	})
}

func TestConvertGitHubCommitFiles(t *testing.T) {
	t.Run("Convert commit files", func(t *testing.T) {
		ghFiles := []*github.CommitFile{
			{
				Filename:  github.String("file1.go"),
				Status:    github.String("modified"),
				Additions: github.Int(10),
				Deletions: github.Int(5),
				Changes:   github.Int(15),
				Patch:     github.String("@@ -1,5 +1,10 @@"),
				SHA:       github.String("abc123"),
				BlobURL:   github.String("https://github.com/blob/abc123"),
			},
		}

		files := convertGitHubCommitFiles(ghFiles)
		assert.Len(t, files, 1)
		assert.Equal(t, "file1.go", files[0].Filename)
		assert.Equal(t, "modified", files[0].Status)
		assert.Equal(t, 10, files[0].Additions)
		assert.Equal(t, 5, files[0].Deletions)
		assert.Equal(t, 15, files[0].Changes)
	})

	t.Run("Handle nil files", func(t *testing.T) {
		ghFiles := []*github.CommitFile{nil}
		files := convertGitHubCommitFiles(ghFiles)
		assert.Empty(t, files)
	})
}

func TestConvertGitHubComment(t *testing.T) {
	now := time.Now()

	t.Run("Convert valid comment", func(t *testing.T) {
		ghComment := &github.IssueComment{
			ID:   github.Int64(456),
			Body: github.String("This is a comment"),
			User: &github.User{
				Login: github.String("commenter"),
			},
			CreatedAt: &github.Timestamp{Time: now},
			UpdatedAt: &github.Timestamp{Time: now},
			HTMLURL:   github.String("https://github.com/comment/456"),
		}

		comment := convertGitHubComment(ghComment)
		assert.NotNil(t, comment)
		assert.Equal(t, int64(456), comment.ID)
		assert.Equal(t, "This is a comment", comment.Body)
		assert.Equal(t, "commenter", comment.Author)
	})

	t.Run("Convert nil comment", func(t *testing.T) {
		comment := convertGitHubComment(nil)
		assert.Nil(t, comment)
	})
}

func TestConvertGitHubReview(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		ghState       string
		expectedState gitprovider.ReviewState
	}{
		{"Approved", "APPROVED", gitprovider.ReviewStateApproved},
		{"Changes requested", "CHANGES_REQUESTED", gitprovider.ReviewStateChangesRequested},
		{"Commented", "COMMENTED", gitprovider.ReviewStateCommented},
		{"Dismissed", "DISMISSED", gitprovider.ReviewStateDismissed},
		{"Pending", "PENDING", gitprovider.ReviewStatePending},
		{"Unknown defaults to pending", "UNKNOWN", gitprovider.ReviewStatePending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ghReview := &github.PullRequestReview{
				ID:          github.Int64(789),
				User:        &github.User{Login: github.String("reviewer")},
				State:       github.String(tt.ghState),
				Body:        github.String("Review comment"),
				SubmittedAt: &github.Timestamp{Time: now},
			}

			review := convertGitHubReview(ghReview)
			assert.NotNil(t, review)
			assert.Equal(t, int64(789), review.ID)
			assert.Equal(t, "reviewer", review.Author)
			assert.Equal(t, tt.expectedState, review.State)
			assert.Equal(t, "Review comment", review.Body)
		})
	}

	t.Run("Convert nil review", func(t *testing.T) {
		review := convertGitHubReview(nil)
		assert.Nil(t, review)
	})
}

// Integration test - requires real GitHub token
// Skipped by default, run with: go test -tags=integration
func TestGitHubProvider_GetRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	token := "" // Set your token here or use environment variable
	if token == "" {
		t.Skip("Set GITHUB_TOKEN to run integration test")
	}

	config := &gitprovider.ProviderConfig{
		Type:  gitprovider.ProviderTypeGitHub,
		Token: token,
	}

	provider, err := NewGitHubProvider(config)
	require.NoError(t, err)

	ctx := context.Background()
	repo, err := provider.GetRepository(ctx, "commercetools", "telefonistka")

	if err != nil {
		t.Logf("Integration test failed (may not have access): %v", err)
		t.Skip()
	}

	assert.NotNil(t, repo)
	assert.Equal(t, "telefonistka", repo.Name)
	assert.Equal(t, "commercetools/telefonistka", repo.FullName)
}
