package gitlab

import (
	"context"
	"fmt"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// SetCommitStatus sets a commit status (GitLab uses pipelines/commit statuses)
func (g *GitLabProvider) SetCommitStatus(ctx context.Context, owner, repo, sha string, status *gitprovider.Status) error {
	projectPath := g.getProjectPath(owner, repo)

	// Map gitprovider states to GitLab states
	var glState gitlab.BuildStateValue
	switch status.State {
	case "pending":
		glState = gitlab.Pending
	case "success":
		glState = gitlab.Success
	case "failure":
		glState = gitlab.Failed
	case "error":
		glState = gitlab.Failed
	default:
		glState = gitlab.Pending
	}

	opts := &gitlab.SetCommitStatusOptions{
		State:       glState,
		TargetURL:   gitlab.Ptr(status.TargetURL),
		Description: gitlab.Ptr(status.Description),
		Name:        gitlab.Ptr(status.Context),
	}

	_, resp, err := g.client.Commits.SetCommitStatus(projectPath, sha, opts)
	g.updateLastResponse(resp)

	if err != nil {
		return fmt.Errorf("failed to set commit status: %w", err)
	}

	return nil
}

// GetCommitStatus returns all statuses for a commit
func (g *GitLabProvider) GetCommitStatus(ctx context.Context, owner, repo, sha string) ([]*gitprovider.Status, error) {
	projectPath := g.getProjectPath(owner, repo)

	statuses, resp, err := g.client.Commits.GetCommitStatuses(projectPath, sha, nil)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get commit statuses: %w", err)
	}

	var result []*gitprovider.Status
	for _, s := range statuses {
		// Map GitLab states back to gitprovider states
		state := "pending"
		switch s.Status {
		case "success":
			state = "success"
		case "failed":
			state = "failure"
		case "canceled":
			state = "error"
		case "pending":
			state = "pending"
		case "running":
			state = "pending"
		}

		result = append(result, &gitprovider.Status{
			State:       state,
			TargetURL:   s.TargetURL,
			Description: s.Description,
			Context:     s.Name,
		})
	}

	return result, nil
}

// GetCombinedStatus returns the combined status for a commit
func (g *GitLabProvider) GetCombinedStatus(ctx context.Context, owner, repo, sha string) (string, error) {
	statuses, err := g.GetCommitStatus(ctx, owner, repo, sha)
	if err != nil {
		return "", err
	}

	if len(statuses) == 0 {
		return "pending", nil
	}

	// Determine combined status
	// If any is failure, return failure
	// If any is pending, return pending
	// If all are success, return success
	hasFailure := false
	hasPending := false

	for _, status := range statuses {
		switch status.State {
		case "failure", "error":
			hasFailure = true
		case "pending":
			hasPending = true
		}
	}

	if hasFailure {
		return "failure", nil
	}
	if hasPending {
		return "pending", nil
	}
	return "success", nil
}
