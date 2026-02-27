package github

import (
	"fmt"
	"net/http"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	"github.com/google/go-github/v62/github"
)

// ParseWebhook parses a GitHub webhook payload
func (g *GitHubProvider) ParseWebhook(req *http.Request, secret []byte) (gitprovider.Event, error) {
	payload, err := github.ValidatePayload(req, secret)
	if err != nil {
		return nil, &gitprovider.WebhookValidationError{
			Message: fmt.Sprintf("webhook validation failed: %v", err),
		}
	}

	eventType := github.WebHookType(req)
	eventPayload, err := github.ParseWebHook(eventType, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse webhook: %w", err)
	}

	// Convert GitHub event to provider-agnostic event
	switch event := eventPayload.(type) {
	case *github.PullRequestEvent:
		return &GitHubPullRequestEvent{event: event}, nil
	case *github.PushEvent:
		return &GitHubPushEvent{event: event}, nil
	case *github.IssueCommentEvent:
		return &GitHubIssueCommentEvent{event: event}, nil
	default:
		return &GitHubUnknownEvent{}, nil
	}
}

// ValidateWebhookSignature validates the webhook signature
func (g *GitHubProvider) ValidateWebhookSignature(req *http.Request, secret []byte) error {
	_, err := github.ValidatePayload(req, secret)
	if err != nil {
		return &gitprovider.WebhookValidationError{
			Message: fmt.Sprintf("webhook signature validation failed: %v", err),
		}
	}
	return nil
}

// GitHubPullRequestEvent wraps GitHub's PullRequestEvent
type GitHubPullRequestEvent struct {
	event *github.PullRequestEvent
}

func (e *GitHubPullRequestEvent) Type() gitprovider.EventType {
	return gitprovider.EventTypePullRequest
}

func (e *GitHubPullRequestEvent) Repository() *gitprovider.Repository {
	repo := e.event.GetRepo()
	return &gitprovider.Repository{
		ID:            repo.GetID(),
		Name:          repo.GetName(),
		FullName:      repo.GetFullName(),
		Owner:         repo.GetOwner().GetLogin(),
		DefaultBranch: repo.GetDefaultBranch(),
		Private:       repo.GetPrivate(),
		HTMLURL:       repo.GetHTMLURL(),
		CloneURL:      repo.GetCloneURL(),
	}
}

func (e *GitHubPullRequestEvent) Action() string {
	return e.event.GetAction()
}

func (e *GitHubPullRequestEvent) PullRequest() *gitprovider.PullRequest {
	return convertGitHubPR(e.event.GetPullRequest())
}

func (e *GitHubPullRequestEvent) Sender() string {
	return e.event.GetSender().GetLogin()
}

// GetGitHubEvent returns the underlying GitHub event (for backward compatibility)
func (e *GitHubPullRequestEvent) GetGitHubEvent() *github.PullRequestEvent {
	return e.event
}

// GitHubPushEvent wraps GitHub's PushEvent
type GitHubPushEvent struct {
	event *github.PushEvent
}

func (e *GitHubPushEvent) Type() gitprovider.EventType {
	return gitprovider.EventTypePush
}

func (e *GitHubPushEvent) Repository() *gitprovider.Repository {
	repo := e.event.GetRepo()
	return &gitprovider.Repository{
		ID:            repo.GetID(),
		Name:          repo.GetName(),
		FullName:      repo.GetFullName(),
		Owner:         repo.GetOwner().GetLogin(),
		DefaultBranch: repo.GetDefaultBranch(),
		Private:       repo.GetPrivate(),
		HTMLURL:       repo.GetHTMLURL(),
		CloneURL:      repo.GetCloneURL(),
	}
}

func (e *GitHubPushEvent) Ref() string {
	return e.event.GetRef()
}

func (e *GitHubPushEvent) Before() string {
	return e.event.GetBefore()
}

func (e *GitHubPushEvent) After() string {
	return e.event.GetAfter()
}

func (e *GitHubPushEvent) Commits() []gitprovider.Commit {
	var commits []gitprovider.Commit
	for _, c := range e.event.Commits {
		commits = append(commits, gitprovider.Commit{
			SHA:     c.GetSHA(),
			Message: c.GetMessage(),
			Author:  c.GetAuthor().GetName(),
			HTMLURL: c.GetURL(),
		})
	}
	return commits
}

func (e *GitHubPushEvent) Sender() string {
	return e.event.GetSender().GetLogin()
}

// GetGitHubEvent returns the underlying GitHub event (for backward compatibility)
func (e *GitHubPushEvent) GetGitHubEvent() *github.PushEvent {
	return e.event
}

// GitHubIssueCommentEvent wraps GitHub's IssueCommentEvent
type GitHubIssueCommentEvent struct {
	event *github.IssueCommentEvent
}

func (e *GitHubIssueCommentEvent) Type() gitprovider.EventType {
	return gitprovider.EventTypeIssueComment
}

func (e *GitHubIssueCommentEvent) Repository() *gitprovider.Repository {
	repo := e.event.GetRepo()
	return &gitprovider.Repository{
		ID:            repo.GetID(),
		Name:          repo.GetName(),
		FullName:      repo.GetFullName(),
		Owner:         repo.GetOwner().GetLogin(),
		DefaultBranch: repo.GetDefaultBranch(),
		Private:       repo.GetPrivate(),
		HTMLURL:       repo.GetHTMLURL(),
		CloneURL:      repo.GetCloneURL(),
	}
}

func (e *GitHubIssueCommentEvent) Action() string {
	return e.event.GetAction()
}

func (e *GitHubIssueCommentEvent) Issue() *gitprovider.PullRequest {
	issue := e.event.GetIssue()
	// GitHub treats PRs as issues for comments, so we need to convert
	return &gitprovider.PullRequest{
		Number:    issue.GetNumber(),
		Title:     issue.GetTitle(),
		Body:      issue.GetBody(),
		State:     issue.GetState(),
		Author:    issue.GetUser().GetLogin(),
		Labels:    convertGitHubLabelsFromIssue(issue.Labels),
		HTMLURL:   issue.GetHTMLURL(),
		CreatedAt: issue.GetCreatedAt().Time,
		UpdatedAt: issue.GetUpdatedAt().Time,
	}
}

func (e *GitHubIssueCommentEvent) Comment() *gitprovider.Comment {
	comment := e.event.GetComment()
	return &gitprovider.Comment{
		ID:        comment.GetID(),
		Body:      comment.GetBody(),
		Author:    comment.GetUser().GetLogin(),
		CreatedAt: comment.GetCreatedAt().Time,
		UpdatedAt: comment.GetUpdatedAt().Time,
		HTMLURL:   comment.GetHTMLURL(),
	}
}

func (e *GitHubIssueCommentEvent) Sender() string {
	return e.event.GetSender().GetLogin()
}

// GetGitHubEvent returns the underlying GitHub event (for backward compatibility)
func (e *GitHubIssueCommentEvent) GetGitHubEvent() *github.IssueCommentEvent {
	return e.event
}

// GitHubUnknownEvent represents an unknown event type
type GitHubUnknownEvent struct{}

func (e *GitHubUnknownEvent) Type() gitprovider.EventType {
	return gitprovider.EventTypeUnknown
}

func (e *GitHubUnknownEvent) Repository() *gitprovider.Repository {
	return nil
}

// Helper to convert issue labels (GitHub API returns different types for issues vs PRs)
func convertGitHubLabelsFromIssue(labels []*github.Label) []string {
	var result []string
	for _, label := range labels {
		if label != nil {
			result = append(result, label.GetName())
		}
	}
	return result
}
