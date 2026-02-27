package gitprovider

import (
	"context"
	"time"
)

// ProviderType represents the Git provider type
type ProviderType string

const (
	ProviderTypeGitHub ProviderType = "github"
	ProviderTypeGitLab ProviderType = "gitlab"
)

// PullRequest represents a provider-agnostic pull/merge request
type PullRequest struct {
	Number      int
	Title       string
	Body        string
	State       string // "open", "closed", "merged"
	Author      string
	HeadRef     string // Branch name
	BaseRef     string // Base branch name
	HeadSHA     string
	BaseSHA     string
	Labels      []string
	HTMLURL     string
	Merged      bool
	Mergeable   bool
	MergeCommit string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// Provider-specific metadata that doesn't fit common model
	ProviderMetadata map[string]interface{}
}

// NewPullRequest contains parameters for creating a new pull/merge request
type NewPullRequest struct {
	Title               string
	Body                string
	Head                string   // Source branch
	Base                string   // Target branch
	Labels              []string // Labels to add
	Assignees           []string // Users to assign
	MaintainerCanModify bool     // Allow maintainer to modify (GitHub)
}

// Comment represents a comment on a pull/merge request
type Comment struct {
	ID        int64
	Body      string
	Author    string
	CreatedAt time.Time
	UpdatedAt time.Time
	HTMLURL   string
}

// CommitFile represents a file changed in a commit or PR
type CommitFile struct {
	Filename  string
	Status    string // "added", "modified", "removed", "renamed"
	Additions int
	Deletions int
	Changes   int
	Patch     string
	SHA       string
	BlobURL   string
}

// Reference represents a Git reference (branch, tag)
type Reference struct {
	Ref    string
	SHA    string
	NodeID string
}

// Commit represents a Git commit
type Commit struct {
	SHA       string
	Message   string
	Author    string
	Date      time.Time
	TreeSHA   string
	Parents   []string
	HTMLURL   string
	Committer string
}

// TreeEntry represents an entry in a Git tree
type TreeEntry struct {
	Path string
	Mode string // "100644" (file), "100755" (executable), "040000" (dir), "160000" (submodule)
	Type string // "blob", "tree", "commit"
	SHA  string
	Size int
	URL  string
}

// CommitOptions contains parameters for creating a commit
type CommitOptions struct {
	Message       string
	Branch        string
	ParentSHA     string
	TreeEntries   []*TreeEntry
	Author        *CommitAuthor
	Committer     *CommitAuthor
	CreateBranch  bool // Create branch if it doesn't exist
	UpdateBranch  bool // Update branch to point to new commit
	AllowEmpty    bool // Allow commit with no changes
	CommitActions []*CommitAction
}

// CommitAuthor represents commit author/committer information
type CommitAuthor struct {
	Name  string
	Email string
	Date  time.Time
}

// CommitAction represents a file action in a commit (GitLab-style)
type CommitAction struct {
	Action   string // "create", "update", "delete", "move"
	FilePath string
	Content  string // For create/update
	Encoding string // "text", "base64"
	// For move action
	PreviousPath string
}

// Status represents a commit status
type Status struct {
	State       string // "pending", "success", "failure", "error"
	TargetURL   string
	Description string
	Context     string // Status identifier (e.g., "telefonistka")
}

// FileNode represents a file or directory in the repository
type FileNode struct {
	Name    string
	Path    string
	Type    string // "file", "dir", "submodule", "symlink"
	Size    int
	SHA     string
	Content []byte // Only populated for files when requested
	HTMLURL string
}

// Repository represents a Git repository
type Repository struct {
	ID            int64
	Name          string
	FullName      string
	Owner         string
	DefaultBranch string
	Private       bool
	HTMLURL       string
	CloneURL      string
}

// EventType represents the type of webhook event
type EventType string

const (
	EventTypePullRequest  EventType = "pull_request"
	EventTypePush         EventType = "push"
	EventTypeIssueComment EventType = "issue_comment"
	EventTypeMergeRequest EventType = "merge_request" // GitLab
	EventTypeNote         EventType = "note"          // GitLab comment
	EventTypeUnknown      EventType = "unknown"
)

// Event is the base interface for all webhook events
type Event interface {
	Type() EventType
	Repository() *Repository
}

// PullRequestEvent represents a pull/merge request event
type PullRequestEvent interface {
	Event
	Action() string // "opened", "closed", "synchronize", "labeled", etc.
	PullRequest() *PullRequest
	Sender() string
}

// PushEvent represents a push event
type PushEvent interface {
	Event
	Ref() string // Branch/tag ref
	Before() string
	After() string
	Commits() []Commit
	Sender() string
}

// IssueCommentEvent represents a comment on a PR/issue
type IssueCommentEvent interface {
	Event
	Action() string // "created", "edited", "deleted"
	Issue() *PullRequest
	Comment() *Comment
	Sender() string
}

// DiffResultLine represents a line in a diff
type DiffResultLine struct {
	Type    string // "context", "add", "delete"
	Content string
	OldLine int
	NewLine int
}

// DiffResult represents the diff between two commits/branches
type DiffResult struct {
	FromSHA   string
	ToSHA     string
	Files     []*CommitFile
	Additions int
	Deletions int
	Changes   int
}

// Label represents a label/tag on a PR/issue
type Label struct {
	ID          int64
	Name        string
	Color       string
	Description string
}

// User represents a user/bot
type User struct {
	ID      int64
	Login   string
	Name    string
	Email   string
	HTMLURL string
	Type    string // "User", "Bot", "Organization"
	IsBot   bool
}

// ReviewState represents the state of a PR review
type ReviewState string

const (
	ReviewStateApproved         ReviewState = "approved"
	ReviewStateChangesRequested ReviewState = "changes_requested"
	ReviewStateCommented        ReviewState = "commented"
	ReviewStateDismissed        ReviewState = "dismissed"
	ReviewStatePending          ReviewState = "pending"
)

// Review represents a pull request review
type Review struct {
	ID        int64
	Author    string
	State     ReviewState
	Body      string
	CreatedAt time.Time
}

// MergeMethod represents how a PR should be merged
type MergeMethod string

const (
	MergeMethodMerge  MergeMethod = "merge"
	MergeMethodSquash MergeMethod = "squash"
	MergeMethodRebase MergeMethod = "rebase"
)

// MergeOptions contains options for merging a PR
type MergeOptions struct {
	CommitMessage string
	CommitTitle   string
	MergeMethod   MergeMethod
	SHA           string // Expected SHA (for safety)
}

// WebhookValidationError is returned when webhook signature validation fails
type WebhookValidationError struct {
	Message string
}

func (e *WebhookValidationError) Error() string {
	return e.Message
}

// ProviderNotSupportedError is returned when an operation is not supported by the provider
type ProviderNotSupportedError struct {
	Provider  ProviderType
	Operation string
	Message   string
}

func (e *ProviderNotSupportedError) Error() string {
	return e.Message
}

// RateLimitInfo contains information about API rate limits
type RateLimitInfo struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

// APIResponse contains metadata about an API call
type APIResponse struct {
	StatusCode int
	RateLimit  *RateLimitInfo
	RequestID  string
	// Provider-specific metadata
	Metadata map[string]interface{}
}

// ListOptions contains options for paginated list operations
type ListOptions struct {
	Page    int
	PerPage int
}

// BranchProtection represents branch protection rules
type BranchProtection struct {
	RequiredReviewers   int
	RequireCodeOwners   bool
	DismissStaleReviews bool
	RequireStatusChecks []string
	EnforceAdmins       bool
}

// WebhookConfig contains webhook configuration
type WebhookConfig struct {
	URL         string
	Secret      string
	Events      []EventType
	ContentType string
	Insecure    bool
}

// ProviderConfig contains provider-specific configuration
type ProviderConfig struct {
	Type           ProviderType
	BaseURL        string // For self-hosted instances
	Token          string
	AppID          int64  // For GitHub App
	PrivateKeyPath string // For GitHub App
	InstallationID int64  // For GitHub App
	ProjectID      string // For GitLab (can be numeric or "group/project")
	WebhookSecret  string
	Timeout        time.Duration
	MaxRetries     int
	// Provider-specific settings
	CustomHeaders map[string]string
	ProxyURL      string
	Insecure      bool // Skip TLS verification
}

// ProviderContext contains the context for a specific PR operation
// Note: Use ProviderClientDetails from interface.go instead of this
// This is kept for backward compatibility during migration
type ProviderContext struct {
	Ctx           context.Context
	Owner         string
	Repo          string
	PrNumber      int
	PrSHA         string
	Ref           string
	DefaultBranch string
	RepoURL       string
	// Metadata parsed from PR body (Telefonistka-specific)
	Metadata map[string]interface{}
}
