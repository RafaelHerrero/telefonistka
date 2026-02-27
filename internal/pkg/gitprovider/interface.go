package gitprovider

import (
	"context"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// GitProvider is the main interface that all Git providers must implement
// This interface abstracts GitHub, GitLab, and potentially other providers
type GitProvider interface {
	// Provider Information
	Type() ProviderType
	GetBotIdentity(ctx context.Context) (*User, error)

	// Repository Operations
	GetRepository(ctx context.Context, owner, repo string) (*Repository, error)
	GetDefaultBranch(ctx context.Context, owner, repo string) (string, error)
	GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
	GetDirectoryContent(ctx context.Context, owner, repo, path, ref string) ([]*FileNode, error)

	// Pull/Merge Request Operations
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error)
	CreatePullRequest(ctx context.Context, owner, repo string, pr *NewPullRequest) (*PullRequest, error)
	ListPullRequestFiles(ctx context.Context, owner, repo string, number int) ([]*CommitFile, error)
	MergePullRequest(ctx context.Context, owner, repo string, number int, options *MergeOptions) error
	CommentOnPullRequest(ctx context.Context, owner, repo string, number int, body string) (*Comment, error)
	ListPullRequestComments(ctx context.Context, owner, repo string, number int) ([]*Comment, error)
	MinimizeComment(ctx context.Context, owner, repo string, commentID int64) error // GitHub-specific but useful

	// Review Operations
	ApprovePullRequest(ctx context.Context, owner, repo string, number int) (*Review, error)
	ListPullRequestReviews(ctx context.Context, owner, repo string, number int) ([]*Review, error)

	// Label Operations
	AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error
	RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error
	GetLabels(ctx context.Context, owner, repo string, number int) ([]*Label, error)

	// Assignee Operations
	AddAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error
	RemoveAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error

	// Git Operations (Low-level)
	GetRef(ctx context.Context, owner, repo, ref string) (*Reference, error)
	CreateRef(ctx context.Context, owner, repo, ref, sha string) (*Reference, error)
	UpdateRef(ctx context.Context, owner, repo, ref, sha string, force bool) (*Reference, error)
	DeleteRef(ctx context.Context, owner, repo, ref string) error

	// Commit Operations
	GetCommit(ctx context.Context, owner, repo, sha string) (*Commit, error)
	CreateCommit(ctx context.Context, owner, repo string, opts *CommitOptions) (*Commit, error)
	CompareCommits(ctx context.Context, owner, repo, base, head string) (*DiffResult, error)

	// Tree Operations (GitHub-style, may not be available on all providers)
	GetTree(ctx context.Context, owner, repo, sha string, recursive bool) ([]*TreeEntry, error)
	CreateTree(ctx context.Context, owner, repo string, baseTree string, entries []*TreeEntry) (string, error)

	// Branch Operations
	CreateBranch(ctx context.Context, owner, repo, branch, sha string) (*Reference, error)
	GetBranch(ctx context.Context, owner, repo, branch string) (*Reference, error)
	DeleteBranch(ctx context.Context, owner, repo, branch string) error

	// Status/Check Operations
	SetCommitStatus(ctx context.Context, owner, repo, sha string, status *Status) error
	GetCommitStatus(ctx context.Context, owner, repo, sha string) ([]*Status, error)
	GetCombinedStatus(ctx context.Context, owner, repo, sha string) (string, error) // Returns overall status

	// Webhook Operations
	ParseWebhook(req *http.Request, secret []byte) (Event, error)
	ValidateWebhookSignature(req *http.Request, secret []byte) error

	// Utility Operations
	SupportsTreeAPI() bool        // Returns true if provider supports GitHub-style tree API
	SupportsGraphQL() bool        // Returns true if provider supports GraphQL
	GetAPIResponse() *APIResponse // Returns metadata about last API call
}

// ProviderClientDetails wraps a GitProvider with context and metadata
// This replaces the old GhPrClientDetails struct
type ProviderClientDetails struct {
	Provider      GitProvider
	Ctx           context.Context
	Owner         string
	Repo          string
	PrNumber      int
	PrSHA         string
	Ref           string
	DefaultBranch string
	RepoURL       string
	PrAuthor      string
	PrLogger      *log.Entry
	Labels        []*Label
	// Metadata parsed from PR body (Telefonistka-specific)
	PrMetadata map[string]interface{}
}

// GetDefaultBranch fetches the default branch for the repository
func (pcd *ProviderClientDetails) GetDefaultBranch() (string, error) {
	if pcd.DefaultBranch != "" {
		return pcd.DefaultBranch, nil
	}

	repo, err := pcd.Provider.GetRepository(pcd.Ctx, pcd.Owner, pcd.Repo)
	if err != nil {
		return "", err
	}

	pcd.DefaultBranch = repo.DefaultBranch
	return pcd.DefaultBranch, nil
}

// HasLabel checks if the PR has a specific label
func (pcd *ProviderClientDetails) HasLabel(label string) bool {
	for _, l := range pcd.Labels {
		if l.Name == label {
			return true
		}
	}
	return false
}

// ProviderFactory creates GitProvider instances based on configuration
type ProviderFactory interface {
	Create(config *ProviderConfig) (GitProvider, error)
	CreateFromEnv(providerType ProviderType) (GitProvider, error)
	// CreateWithCache returns a cached provider if available
	CreateWithCache(config *ProviderConfig, cacheKey string) (GitProvider, error)
}

// ProviderCapabilities describes what a provider supports
type ProviderCapabilities struct {
	TreeAPI             bool // GitHub-style tree API
	GraphQL             bool // GraphQL API
	CommentMinimization bool // Ability to minimize/hide comments
	ReviewApproval      bool // Formal review approval
	MultipleReviewers   bool // Require multiple reviewers
	MergeTrain          bool // GitLab merge trains
	AutoMerge           bool // Automatic merge when checks pass
	DraftPullRequests   bool // Draft PR support
	RequiredApprovals   bool // Require N approvals
	ProtectedBranches   bool // Branch protection
	CommitStatusAPI     bool // Commit status API
	ChecksAPI           bool // GitHub Checks API
	FileContentAPI      bool // API to get file content
	DirectoryListingAPI bool // API to list directory contents
}

// GetCapabilities returns the capabilities of the provider
func GetCapabilities(providerType ProviderType) *ProviderCapabilities {
	switch providerType {
	case ProviderTypeGitHub:
		return &ProviderCapabilities{
			TreeAPI:             true,
			GraphQL:             true,
			CommentMinimization: true,
			ReviewApproval:      true,
			MultipleReviewers:   true,
			MergeTrain:          false,
			AutoMerge:           true,
			DraftPullRequests:   true,
			RequiredApprovals:   true,
			ProtectedBranches:   true,
			CommitStatusAPI:     true,
			ChecksAPI:           true,
			FileContentAPI:      true,
			DirectoryListingAPI: true,
		}
	case ProviderTypeGitLab:
		return &ProviderCapabilities{
			TreeAPI:             false, // GitLab doesn't have tree API
			GraphQL:             true,
			CommentMinimization: false, // GitLab doesn't have comment minimization
			ReviewApproval:      true,
			MultipleReviewers:   true,
			MergeTrain:          true, // GitLab-specific feature
			AutoMerge:           true,
			DraftPullRequests:   true,
			RequiredApprovals:   true,
			ProtectedBranches:   true,
			CommitStatusAPI:     true,
			ChecksAPI:           false, // GitLab uses pipelines
			FileContentAPI:      true,
			DirectoryListingAPI: true,
		}
	default:
		return &ProviderCapabilities{}
	}
}
