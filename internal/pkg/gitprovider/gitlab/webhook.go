package gitlab

import (
	"fmt"
	"io"
	"net/http"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// ParseWebhook parses a GitLab webhook payload
func (g *GitLabProvider) ParseWebhook(req *http.Request, secret []byte) (gitprovider.Event, error) {
	// Read body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, &gitprovider.WebhookValidationError{
			Message: fmt.Sprintf("failed to read request body: %v", err),
		}
	}
	defer req.Body.Close()

	payload, err := gitlab.ParseWebhook(gitlab.HookEventType(req), body)
	if err != nil {
		return nil, &gitprovider.WebhookValidationError{
			Message: fmt.Sprintf("failed to parse webhook: %v", err),
		}
	}

	// Convert GitLab event to provider-agnostic event
	switch event := payload.(type) {
	case *gitlab.MergeEvent:
		return &GitLabMergeRequestEvent{event: event}, nil
	case *gitlab.PushEvent:
		return &GitLabPushEvent{event: event}, nil
	case *gitlab.MergeCommentEvent:
		return &GitLabNoteEvent{event: event}, nil
	default:
		return &GitLabUnknownEvent{}, nil
	}
}

// ValidateWebhookSignature validates the webhook signature
func (g *GitLabProvider) ValidateWebhookSignature(req *http.Request, secret []byte) error {
	// GitLab uses X-Gitlab-Token header
	token := req.Header.Get("X-Gitlab-Token")
	if token != string(secret) {
		return &gitprovider.WebhookValidationError{
			Message: "invalid webhook token",
		}
	}
	return nil
}

// GitLabMergeRequestEvent wraps GitLab's MergeEvent
type GitLabMergeRequestEvent struct {
	event *gitlab.MergeEvent
}

func (e *GitLabMergeRequestEvent) Type() gitprovider.EventType {
	return gitprovider.EventTypeMergeRequest
}

func (e *GitLabMergeRequestEvent) Repository() *gitprovider.Repository {
	return &gitprovider.Repository{
		ID:            int64(e.event.Project.ID),
		Name:          e.event.Project.Name,
		FullName:      e.event.Project.PathWithNamespace,
		Owner:         e.event.Project.Namespace,
		DefaultBranch: e.event.Project.DefaultBranch,
		Private:       e.event.Project.Visibility != gitlab.PublicVisibility,
		HTMLURL:       e.event.Project.WebURL,
		CloneURL:      e.event.Project.GitHTTPURL,
	}
}

func (e *GitLabMergeRequestEvent) Action() string {
	// Map GitLab actions to GitHub-style actions
	switch e.event.ObjectAttributes.Action {
	case "open":
		return "opened"
	case "reopen":
		return "reopened"
	case "update":
		return "synchronize"
	case "merge":
		return "closed"
	case "close":
		return "closed"
	default:
		return e.event.ObjectAttributes.Action
	}
}

func (e *GitLabMergeRequestEvent) PullRequest() *gitprovider.PullRequest {
	merged := e.event.ObjectAttributes.State == "merged"

	return &gitprovider.PullRequest{
		Number:      int(e.event.ObjectAttributes.IID),
		Title:       e.event.ObjectAttributes.Title,
		Body:        e.event.ObjectAttributes.Description,
		State:       e.event.ObjectAttributes.State,
		Author:      e.event.User.Username,
		HeadRef:     e.event.ObjectAttributes.SourceBranch,
		BaseRef:     e.event.ObjectAttributes.TargetBranch,
		HeadSHA:     e.event.ObjectAttributes.LastCommit.ID,
		Labels:      []string{}, // GitLab webhook doesn't include labels inline
		HTMLURL:     e.event.ObjectAttributes.URL,
		Merged:      merged,
		Mergeable:   true, // GitLab doesn't provide this in webhook
		MergeCommit: e.event.ObjectAttributes.MergeCommitSHA,
	}
}

func (e *GitLabMergeRequestEvent) Sender() string {
	return e.event.User.Username
}

// GetGitLabEvent returns the underlying GitLab event (for backward compatibility)
func (e *GitLabMergeRequestEvent) GetGitLabEvent() *gitlab.MergeEvent {
	return e.event
}

// GitLabPushEvent wraps GitLab's PushEvent
type GitLabPushEvent struct {
	event *gitlab.PushEvent
}

func (e *GitLabPushEvent) Type() gitprovider.EventType {
	return gitprovider.EventTypePush
}

func (e *GitLabPushEvent) Repository() *gitprovider.Repository {
	return &gitprovider.Repository{
		ID:            int64(e.event.ProjectID),
		Name:          e.event.Project.Name,
		FullName:      e.event.Project.PathWithNamespace,
		Owner:         e.event.Project.Namespace,
		DefaultBranch: e.event.Project.DefaultBranch,
		Private:       e.event.Project.Visibility != gitlab.PublicVisibility,
		HTMLURL:       e.event.Project.WebURL,
		CloneURL:      e.event.Project.GitHTTPURL,
	}
}

func (e *GitLabPushEvent) Ref() string {
	return e.event.Ref
}

func (e *GitLabPushEvent) Before() string {
	return e.event.Before
}

func (e *GitLabPushEvent) After() string {
	return e.event.After
}

func (e *GitLabPushEvent) Commits() []gitprovider.Commit {
	var commits []gitprovider.Commit
	for _, c := range e.event.Commits {
		commits = append(commits, gitprovider.Commit{
			SHA:     c.ID,
			Message: c.Message,
			Author:  c.Author.Name,
			HTMLURL: c.URL,
		})
	}
	return commits
}

func (e *GitLabPushEvent) Sender() string {
	return e.event.UserUsername
}

// GetGitLabEvent returns the underlying GitLab event (for backward compatibility)
func (e *GitLabPushEvent) GetGitLabEvent() *gitlab.PushEvent {
	return e.event
}

// GitLabNoteEvent wraps GitLab's MergeCommentEvent
type GitLabNoteEvent struct {
	event *gitlab.MergeCommentEvent
}

func (e *GitLabNoteEvent) Type() gitprovider.EventType {
	return gitprovider.EventTypeNote
}

func (e *GitLabNoteEvent) Repository() *gitprovider.Repository {
	return &gitprovider.Repository{
		ID:            int64(e.event.ProjectID),
		Name:          e.event.Project.Name,
		FullName:      e.event.Project.PathWithNamespace,
		Owner:         e.event.Project.Namespace,
		DefaultBranch: e.event.Project.DefaultBranch,
		Private:       e.event.Project.Visibility != gitlab.PublicVisibility,
		HTMLURL:       e.event.Project.WebURL,
		CloneURL:      e.event.Project.GitHTTPURL,
	}
}

func (e *GitLabNoteEvent) Action() string {
	// GitLab doesn't have action in note events, just creation
	return "created"
}

func (e *GitLabNoteEvent) Issue() *gitprovider.PullRequest {
	return &gitprovider.PullRequest{
		Number:  int(e.event.MergeRequest.IID),
		Title:   e.event.MergeRequest.Title,
		Body:    e.event.MergeRequest.Description,
		State:   e.event.MergeRequest.State,
		Author:  e.event.User.Username,
		HeadRef: e.event.MergeRequest.SourceBranch,
		BaseRef: e.event.MergeRequest.TargetBranch,
		HTMLURL: e.event.MergeRequest.URL,
	}
}

func (e *GitLabNoteEvent) Comment() *gitprovider.Comment {
	return &gitprovider.Comment{
		ID:      int64(e.event.ObjectAttributes.ID),
		Body:    e.event.ObjectAttributes.Note,
		Author:  e.event.User.Username,
		HTMLURL: e.event.ObjectAttributes.URL,
	}
}

func (e *GitLabNoteEvent) Sender() string {
	return e.event.User.Username
}

// GetGitLabEvent returns the underlying GitLab event (for backward compatibility)
func (e *GitLabNoteEvent) GetGitLabEvent() *gitlab.MergeCommentEvent {
	return e.event
}

// GitLabUnknownEvent represents an unknown event type
type GitLabUnknownEvent struct{}

func (e *GitLabUnknownEvent) Type() gitprovider.EventType {
	return gitprovider.EventTypeUnknown
}

func (e *GitLabUnknownEvent) Repository() *gitprovider.Repository {
	return nil
}
