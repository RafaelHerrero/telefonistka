package github

import (
	"context"
	"fmt"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	"github.com/google/go-github/v62/github"
)

// GetPullRequest returns a pull request
func (g *GitHubProvider) GetPullRequest(ctx context.Context, owner, repo string, number int) (*gitprovider.PullRequest, error) {
	pr, resp, err := g.v3Client.PullRequests.Get(ctx, owner, repo, number)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}

	return convertGitHubPR(pr), nil
}

// CreatePullRequest creates a new pull request
func (g *GitHubProvider) CreatePullRequest(ctx context.Context, owner, repo string, pr *gitprovider.NewPullRequest) (*gitprovider.PullRequest, error) {
	newPR := &github.NewPullRequest{
		Title:               github.String(pr.Title),
		Body:                github.String(pr.Body),
		Head:                github.String(pr.Head),
		Base:                github.String(pr.Base),
		MaintainerCanModify: github.Bool(pr.MaintainerCanModify),
	}

	ghPR, resp, err := g.v3Client.PullRequests.Create(ctx, owner, repo, newPR)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	result := convertGitHubPR(ghPR)

	// Add labels if specified
	if len(pr.Labels) > 0 {
		err = g.AddLabels(ctx, owner, repo, ghPR.GetNumber(), pr.Labels)
		if err != nil {
			return result, fmt.Errorf("pull request created but failed to add labels: %w", err)
		}
		result.Labels = pr.Labels
	}

	// Add assignees if specified
	if len(pr.Assignees) > 0 {
		err = g.AddAssignees(ctx, owner, repo, ghPR.GetNumber(), pr.Assignees)
		if err != nil {
			return result, fmt.Errorf("pull request created but failed to add assignees: %w", err)
		}
	}

	return result, nil
}

// ListPullRequestFiles returns the list of files changed in a PR
func (g *GitHubProvider) ListPullRequestFiles(ctx context.Context, owner, repo string, number int) ([]*gitprovider.CommitFile, error) {
	var allFiles []*github.CommitFile
	opts := &github.ListOptions{PerPage: 100}

	for {
		files, resp, err := g.v3Client.PullRequests.ListFiles(ctx, owner, repo, number, opts)
		g.updateLastResponse(resp)

		if err != nil {
			return nil, fmt.Errorf("failed to list PR files: %w", err)
		}

		allFiles = append(allFiles, files...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return convertGitHubCommitFiles(allFiles), nil
}

// MergePullRequest merges a pull request
func (g *GitHubProvider) MergePullRequest(ctx context.Context, owner, repo string, number int, options *gitprovider.MergeOptions) error {
	var mergeMethod string
	if options != nil && options.MergeMethod != "" {
		mergeMethod = string(options.MergeMethod)
	} else {
		mergeMethod = "merge"
	}

	mergeOpts := &github.PullRequestOptions{
		MergeMethod: mergeMethod,
	}

	commitMessage := ""
	if options != nil {
		if options.CommitMessage != "" {
			commitMessage = options.CommitMessage
		}
		if options.SHA != "" {
			mergeOpts.SHA = options.SHA
		}
	}

	_, resp, err := g.v3Client.PullRequests.Merge(ctx, owner, repo, number, commitMessage, mergeOpts)
	g.updateLastResponse(resp)

	if err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	return nil
}

// CommentOnPullRequest adds a comment to a pull request
func (g *GitHubProvider) CommentOnPullRequest(ctx context.Context, owner, repo string, number int, body string) (*gitprovider.Comment, error) {
	comment := &github.IssueComment{
		Body: github.String(body),
	}

	ghComment, resp, err := g.v3Client.Issues.CreateComment(ctx, owner, repo, number, comment)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	return convertGitHubComment(ghComment), nil
}

// ListPullRequestComments returns all comments on a pull request
func (g *GitHubProvider) ListPullRequestComments(ctx context.Context, owner, repo string, number int) ([]*gitprovider.Comment, error) {
	var allComments []*github.IssueComment
	opts := &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		comments, resp, err := g.v3Client.Issues.ListComments(ctx, owner, repo, number, opts)
		g.updateLastResponse(resp)

		if err != nil {
			return nil, fmt.Errorf("failed to list comments: %w", err)
		}

		allComments = append(allComments, comments...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	var result []*gitprovider.Comment
	for _, c := range allComments {
		result = append(result, convertGitHubComment(c))
	}

	return result, nil
}

// MinimizeComment minimizes a comment (GitHub-specific feature)
func (g *GitHubProvider) MinimizeComment(ctx context.Context, owner, repo string, commentID int64) error {
	// This requires GraphQL API
	var mutation struct {
		MinimizeComment struct {
			MinimizedComment struct {
				IsMinimized bool
			}
		} `graphql:"minimizeComment(input: $input)"`
	}

	input := map[string]interface{}{
		"subjectId":  commentID,
		"classifier": "OUTDATED",
	}

	err := g.v4Client.Mutate(ctx, &mutation, input, nil)
	if err != nil {
		return fmt.Errorf("failed to minimize comment: %w", err)
	}

	return nil
}

// ApprovePullRequest approves a pull request
func (g *GitHubProvider) ApprovePullRequest(ctx context.Context, owner, repo string, number int) (*gitprovider.Review, error) {
	review := &github.PullRequestReviewRequest{
		Event: github.String("APPROVE"),
	}

	ghReview, resp, err := g.v3Client.PullRequests.CreateReview(ctx, owner, repo, number, review)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to approve pull request: %w", err)
	}

	return convertGitHubReview(ghReview), nil
}

// ListPullRequestReviews returns all reviews on a pull request
func (g *GitHubProvider) ListPullRequestReviews(ctx context.Context, owner, repo string, number int) ([]*gitprovider.Review, error) {
	var allReviews []*github.PullRequestReview
	opts := &github.ListOptions{PerPage: 100}

	for {
		reviews, resp, err := g.v3Client.PullRequests.ListReviews(ctx, owner, repo, number, opts)
		g.updateLastResponse(resp)

		if err != nil {
			return nil, fmt.Errorf("failed to list reviews: %w", err)
		}

		allReviews = append(allReviews, reviews...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	var result []*gitprovider.Review
	for _, r := range allReviews {
		result = append(result, convertGitHubReview(r))
	}

	return result, nil
}

// AddLabels adds labels to a pull request
func (g *GitHubProvider) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	_, resp, err := g.v3Client.Issues.AddLabelsToIssue(ctx, owner, repo, number, labels)
	g.updateLastResponse(resp)

	if err != nil {
		return fmt.Errorf("failed to add labels: %w", err)
	}

	return nil
}

// RemoveLabel removes a label from a pull request
func (g *GitHubProvider) RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error {
	resp, err := g.v3Client.Issues.RemoveLabelForIssue(ctx, owner, repo, number, label)
	g.updateLastResponse(resp)

	if err != nil {
		return fmt.Errorf("failed to remove label: %w", err)
	}

	return nil
}

// GetLabels returns all labels on a pull request
func (g *GitHubProvider) GetLabels(ctx context.Context, owner, repo string, number int) ([]*gitprovider.Label, error) {
	labels, resp, err := g.v3Client.Issues.ListLabelsByIssue(ctx, owner, repo, number, nil)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get labels: %w", err)
	}

	var result []*gitprovider.Label
	for _, l := range labels {
		result = append(result, convertGitHubLabel(l))
	}

	return result, nil
}

// AddAssignees adds assignees to a pull request
func (g *GitHubProvider) AddAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error {
	_, resp, err := g.v3Client.Issues.AddAssignees(ctx, owner, repo, number, assignees)
	g.updateLastResponse(resp)

	if err != nil {
		return fmt.Errorf("failed to add assignees: %w", err)
	}

	return nil
}

// RemoveAssignees removes assignees from a pull request
func (g *GitHubProvider) RemoveAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error {
	_, resp, err := g.v3Client.Issues.RemoveAssignees(ctx, owner, repo, number, assignees)
	g.updateLastResponse(resp)

	if err != nil {
		return fmt.Errorf("failed to remove assignees: %w", err)
	}

	return nil
}

// Conversion helper functions

func convertGitHubPR(pr *github.PullRequest) *gitprovider.PullRequest {
	if pr == nil {
		return nil
	}

	return &gitprovider.PullRequest{
		Number:      pr.GetNumber(),
		Title:       pr.GetTitle(),
		Body:        pr.GetBody(),
		State:       pr.GetState(),
		Author:      pr.GetUser().GetLogin(),
		HeadRef:     pr.GetHead().GetRef(),
		BaseRef:     pr.GetBase().GetRef(),
		HeadSHA:     pr.GetHead().GetSHA(),
		BaseSHA:     pr.GetBase().GetSHA(),
		Labels:      convertGitHubLabels(pr.Labels),
		HTMLURL:     pr.GetHTMLURL(),
		Merged:      pr.GetMerged(),
		Mergeable:   pr.GetMergeable(),
		MergeCommit: pr.GetMergeCommitSHA(),
		CreatedAt:   pr.GetCreatedAt().Time,
		UpdatedAt:   pr.GetUpdatedAt().Time,
	}
}

func convertGitHubCommitFiles(files []*github.CommitFile) []*gitprovider.CommitFile {
	var result []*gitprovider.CommitFile
	for _, f := range files {
		if f == nil {
			continue
		}
		result = append(result, &gitprovider.CommitFile{
			Filename:  f.GetFilename(),
			Status:    f.GetStatus(),
			Additions: f.GetAdditions(),
			Deletions: f.GetDeletions(),
			Changes:   f.GetChanges(),
			Patch:     f.GetPatch(),
			SHA:       f.GetSHA(),
			BlobURL:   f.GetBlobURL(),
		})
	}
	return result
}

func convertGitHubComment(c *github.IssueComment) *gitprovider.Comment {
	if c == nil {
		return nil
	}

	return &gitprovider.Comment{
		ID:        c.GetID(),
		Body:      c.GetBody(),
		Author:    c.GetUser().GetLogin(),
		CreatedAt: c.GetCreatedAt().Time,
		UpdatedAt: c.GetUpdatedAt().Time,
		HTMLURL:   c.GetHTMLURL(),
	}
}

func convertGitHubReview(r *github.PullRequestReview) *gitprovider.Review {
	if r == nil {
		return nil
	}

	var state gitprovider.ReviewState
	switch r.GetState() {
	case "APPROVED":
		state = gitprovider.ReviewStateApproved
	case "CHANGES_REQUESTED":
		state = gitprovider.ReviewStateChangesRequested
	case "COMMENTED":
		state = gitprovider.ReviewStateCommented
	case "DISMISSED":
		state = gitprovider.ReviewStateDismissed
	case "PENDING":
		state = gitprovider.ReviewStatePending
	default:
		state = gitprovider.ReviewStatePending
	}

	return &gitprovider.Review{
		ID:        r.GetID(),
		Author:    r.GetUser().GetLogin(),
		State:     state,
		Body:      r.GetBody(),
		CreatedAt: r.GetSubmittedAt().Time,
	}
}
