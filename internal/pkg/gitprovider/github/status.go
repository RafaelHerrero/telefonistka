package github

import (
	"context"
	"fmt"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	"github.com/google/go-github/v62/github"
)

// SetCommitStatus sets a commit status
func (g *GitHubProvider) SetCommitStatus(ctx context.Context, owner, repo, sha string, status *gitprovider.Status) error {
	ghStatus := &github.RepoStatus{
		State:       github.String(status.State),
		TargetURL:   github.String(status.TargetURL),
		Description: github.String(status.Description),
		Context:     github.String(status.Context),
	}

	_, resp, err := g.v3Client.Repositories.CreateStatus(ctx, owner, repo, sha, ghStatus)
	g.updateLastResponse(resp)

	if err != nil {
		return fmt.Errorf("failed to set commit status: %w", err)
	}

	return nil
}

// GetCommitStatus returns all statuses for a commit
func (g *GitHubProvider) GetCommitStatus(ctx context.Context, owner, repo, sha string) ([]*gitprovider.Status, error) {
	statuses, resp, err := g.v3Client.Repositories.ListStatuses(ctx, owner, repo, sha, nil)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get commit statuses: %w", err)
	}

	var result []*gitprovider.Status
	for _, s := range statuses {
		result = append(result, &gitprovider.Status{
			State:       s.GetState(),
			TargetURL:   s.GetTargetURL(),
			Description: s.GetDescription(),
			Context:     s.GetContext(),
		})
	}

	return result, nil
}

// GetCombinedStatus returns the combined status for a commit
func (g *GitHubProvider) GetCombinedStatus(ctx context.Context, owner, repo, sha string) (string, error) {
	combined, resp, err := g.v3Client.Repositories.GetCombinedStatus(ctx, owner, repo, sha, nil)
	g.updateLastResponse(resp)

	if err != nil {
		return "", fmt.Errorf("failed to get combined status: %w", err)
	}

	return combined.GetState(), nil
}
