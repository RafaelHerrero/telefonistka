package gitlabci

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/commercetools/telefonistka/internal/pkg/gitlabapi"
	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	_ "github.com/commercetools/telefonistka/internal/pkg/gitprovider/gitlab"
	log "github.com/sirupsen/logrus"
)

// BuildEventFromEnvironment constructs a GitProvider event from GitLab CI environment variables
func BuildEventFromEnvironment() (gitprovider.Event, error) {
	// Check if we're running in GitLab CI
	if os.Getenv("GITLAB_CI") == "" {
		return nil, fmt.Errorf("not running in GitLab CI environment")
	}

	// Determine event type based on CI_PIPELINE_SOURCE
	pipelineSource := os.Getenv("CI_PIPELINE_SOURCE")

	log.Infof("Building event from GitLab CI environment: pipeline_source=%s", pipelineSource)

	switch pipelineSource {
	case "merge_request_event":
		return buildMergeRequestEvent()
	case "push":
		return buildPushEvent()
	default:
		return nil, fmt.Errorf("unsupported pipeline source: %s", pipelineSource)
	}
}

// buildMergeRequestEvent creates a MergeRequestEvent from environment variables
func buildMergeRequestEvent() (gitprovider.Event, error) {
	// Extract MR details from environment
	mrIID := os.Getenv("CI_MERGE_REQUEST_IID")
	if mrIID == "" {
		return nil, fmt.Errorf("CI_MERGE_REQUEST_IID not set")
	}

	mrNumber, err := strconv.Atoi(mrIID)
	if err != nil {
		return nil, fmt.Errorf("invalid CI_MERGE_REQUEST_IID: %v", err)
	}

	projectPath := os.Getenv("CI_PROJECT_PATH")
	if projectPath == "" {
		return nil, fmt.Errorf("CI_PROJECT_PATH not set")
	}

	// Parse owner/repo from project path
	owner, repo := parseProjectPath(projectPath)

	// Construct event
	event := &GitLabCIMergeRequestEvent{
		MRNumber:     mrNumber,
		ProjectPath:  projectPath,
		Owner:        owner,
		Repo:         repo,
		Title:        os.Getenv("CI_MERGE_REQUEST_TITLE"),
		SourceBranch: os.Getenv("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"),
		TargetBranch: os.Getenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME"),
		HeadSHA:      os.Getenv("CI_COMMIT_SHA"),
		BaseSHA:      os.Getenv("CI_MERGE_REQUEST_DIFF_BASE_SHA"),
		Author:       os.Getenv("GITLAB_USER_LOGIN"),
		ServerURL:    os.Getenv("CI_SERVER_URL"),
		ProjectID:    os.Getenv("CI_PROJECT_ID"),
		// Determine action from merge status
		MergeStatus: os.Getenv("CI_MERGE_REQUEST_EVENT_TYPE"),
	}

	// Set default action if not specified
	if event.MergeStatus == "" {
		event.MergeStatus = "opened"
	}

	log.Infof("Built MR event: MR#%d in %s", mrNumber, projectPath)
	return event, nil
}

// buildPushEvent creates a PushEvent from environment variables
func buildPushEvent() (gitprovider.Event, error) {
	projectPath := os.Getenv("CI_PROJECT_PATH")
	if projectPath == "" {
		return nil, fmt.Errorf("CI_PROJECT_PATH not set")
	}

	owner, repo := parseProjectPath(projectPath)

	event := &GitLabCIPushEvent{
		ProjectPath:   projectPath,
		Owner:         owner,
		Repo:          repo,
		CommitSHA:     os.Getenv("CI_COMMIT_SHA"),
		BeforeSHA:     os.Getenv("CI_COMMIT_BEFORE_SHA"),
		RefName:       os.Getenv("CI_COMMIT_REF_NAME"),
		CommitAuthor:  os.Getenv("GITLAB_USER_LOGIN"),
		ServerURL:     os.Getenv("CI_SERVER_URL"),
		ProjectID:     os.Getenv("CI_PROJECT_ID"),
		CommitMessage: os.Getenv("CI_COMMIT_MESSAGE"),
	}

	log.Infof("Built push event: ref=%s in %s", event.RefName, projectPath)
	return event, nil
}

// WriteEventFile writes the event to a JSON file for compatibility with event file mode
func WriteEventFile(outputPath string) error {
	event, err := BuildEventFromEnvironment()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event: %v", err)
	}

	err = os.WriteFile(outputPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write event file: %v", err)
	}

	log.Infof("Wrote event file to %s", outputPath)
	return nil
}

// parseProjectPath splits "group/subgroup/project" into owner and repo
func parseProjectPath(path string) (owner, repo string) {
	// For GitLab, we treat everything before the last / as owner
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i], path[i+1:]
		}
	}
	return "", path
}

// GitLabCIMergeRequestEvent implements gitprovider.PullRequestEvent
type GitLabCIMergeRequestEvent struct {
	MRNumber     int
	ProjectPath  string
	Owner        string
	Repo         string
	Title        string
	SourceBranch string
	TargetBranch string
	HeadSHA      string
	BaseSHA      string
	Author       string
	ServerURL    string
	ProjectID    string
	MergeStatus  string
}

func (e *GitLabCIMergeRequestEvent) Type() gitprovider.EventType {
	return gitprovider.EventTypeMergeRequest
}

func (e *GitLabCIMergeRequestEvent) Repository() *gitprovider.Repository {
	return &gitprovider.Repository{
		Name:          e.Repo,
		FullName:      e.ProjectPath,
		Owner:         e.Owner,
		DefaultBranch: "main", // We don't have this info from CI env
		HTMLURL:       fmt.Sprintf("%s/%s", e.ServerURL, e.ProjectPath),
	}
}

func (e *GitLabCIMergeRequestEvent) Action() string {
	return e.MergeStatus
}

func (e *GitLabCIMergeRequestEvent) PullRequest() *gitprovider.PullRequest {
	return &gitprovider.PullRequest{
		Number:    e.MRNumber,
		Title:     e.Title,
		HeadRef:   e.SourceBranch,
		BaseRef:   e.TargetBranch,
		HeadSHA:   e.HeadSHA,
		BaseSHA:   e.BaseSHA,
		Author:    e.Author,
		State:     "open",
		HTMLURL:   fmt.Sprintf("%s/%s/-/merge_requests/%d", e.ServerURL, e.ProjectPath, e.MRNumber),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (e *GitLabCIMergeRequestEvent) Sender() string {
	return e.Author
}

// GitLabCIPushEvent implements gitprovider.PushEvent
type GitLabCIPushEvent struct {
	ProjectPath   string
	Owner         string
	Repo          string
	CommitSHA     string
	BeforeSHA     string
	RefName       string
	CommitAuthor  string
	ServerURL     string
	ProjectID     string
	CommitMessage string
}

func (e *GitLabCIPushEvent) Type() gitprovider.EventType {
	return gitprovider.EventTypePush
}

func (e *GitLabCIPushEvent) Repository() *gitprovider.Repository {
	return &gitprovider.Repository{
		Name:          e.Repo,
		FullName:      e.ProjectPath,
		Owner:         e.Owner,
		DefaultBranch: "main",
		HTMLURL:       fmt.Sprintf("%s/%s", e.ServerURL, e.ProjectPath),
	}
}

func (e *GitLabCIPushEvent) Ref() string {
	return e.RefName
}

func (e *GitLabCIPushEvent) Before() string {
	return e.BeforeSHA
}

func (e *GitLabCIPushEvent) After() string {
	return e.CommitSHA
}

func (e *GitLabCIPushEvent) Commits() []gitprovider.Commit {
	// In CI mode, we don't have access to the full commit list
	// Return single commit
	return []gitprovider.Commit{
		{
			SHA:     e.CommitSHA,
			Message: "Commit from CI pipeline",
			Author:  e.CommitAuthor,
			Date:    time.Now(),
		},
	}
}

func (e *GitLabCIPushEvent) Sender() string {
	return e.CommitAuthor
}

// ProcessEventInCI is the main entry point for handling events in GitLab CI
func ProcessEventInCI(ctx context.Context) error {
	// Build event from environment
	event, err := BuildEventFromEnvironment()
	if err != nil {
		return fmt.Errorf("failed to build event from environment: %v", err)
	}

	// Get GitLab token
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		token = os.Getenv("CI_JOB_TOKEN") // Fallback to CI job token
	}
	if token == "" {
		return fmt.Errorf("GITLAB_TOKEN or CI_JOB_TOKEN required")
	}

	// Get GitLab URL
	gitlabURL := os.Getenv("GITLAB_URL")
	if gitlabURL == "" {
		gitlabURL = os.Getenv("CI_SERVER_URL")
	}
	if gitlabURL == "" {
		gitlabURL = "https://gitlab.com"
	}

	// Create GitLab provider
	factory, err := gitprovider.NewDefaultProviderFactory(10)
	if err != nil {
		return fmt.Errorf("failed to create provider factory: %v", err)
	}

	config := &gitprovider.ProviderConfig{
		Type:    gitprovider.ProviderTypeGitLab,
		Token:   token,
		BaseURL: gitlabURL,
	}

	provider, err := factory.Create(config)
	if err != nil {
		return fmt.Errorf("failed to create GitLab provider: %v", err)
	}

	log.Infof("Created GitLab provider for %s", gitlabURL)

	// Handle the event based on type
	switch e := event.(type) {
	case *GitLabCIMergeRequestEvent:
		return handleMREventInCI(ctx, e, provider)
	case *GitLabCIPushEvent:
		return handlePushEventInCI(ctx, e, provider)
	default:
		return fmt.Errorf("unsupported event type: %T", event)
	}
}

// handleMREventInCI processes MR events in CI mode
func handleMREventInCI(ctx context.Context, event *GitLabCIMergeRequestEvent, provider gitprovider.GitProvider) error {
	log.Infof("Processing MR #%d in CI mode", event.MRNumber)

	prLogger := log.WithFields(log.Fields{
		"repo":       event.ProjectPath,
		"mrNumber":   event.MRNumber,
		"event_type": "merge_request",
		"mode":       "ci",
	})

	repoURL := fmt.Sprintf("%s/%s", event.ServerURL, event.ProjectPath)
	details := gitlabapi.ProviderClientDetails{
		Ctx:      ctx,
		Provider: provider,
		Owner:    event.Owner,
		Repo:     event.Repo,
		PrNumber: event.MRNumber,
		PrSHA:    event.HeadSHA,
		Ref:      event.SourceBranch,
		RepoURL:  repoURL,
		PrAuthor: event.Author,
		PrLogger: prLogger,
	}

	// Run drift detection
	err := gitlabapi.DetectDrift(ctx, details)
	if err != nil {
		prLogger.Errorf("Drift detection failed: %v", err)
		return err
	}

	log.Info("Successfully processed MR event in CI mode")
	return nil
}

// handlePushEventInCI processes push events in CI mode.
// When a push is to the default branch, it triggers the promotion workflow.
// If the push is a merge commit, it extracts the MR number and uses MR-based
// promotion (with full MR context: files, labels, body metadata).
// Otherwise, it falls back to comparing before/after SHAs.
func handlePushEventInCI(ctx context.Context, event *GitLabCIPushEvent, provider gitprovider.GitProvider) error {
	log.Infof("Processing push event for ref %s in CI mode", event.RefName)

	// Get default branch to check if this push is to it
	defaultBranch, err := provider.GetDefaultBranch(ctx, event.Owner, event.Repo)
	if err != nil {
		log.Warnf("Failed to get default branch, falling back to 'main': %v", err)
		defaultBranch = "main"
	}

	// Only process pushes to default branch
	if event.RefName != defaultBranch {
		log.Infof("Push to %s (not default branch %s), skipping promotion", event.RefName, defaultBranch)
		return nil
	}

	log.Infof("Push to default branch %s: %s -> %s, starting promotion workflow", defaultBranch, event.BeforeSHA[:8], event.CommitSHA[:8])

	// Try to extract MR number from merge commit message
	// GitLab merge commits contain "See merge request group/project!123"
	mrNumber := extractMRNumberFromCommitMessage(event.CommitMessage)

	prLogger := log.WithFields(log.Fields{
		"repo":      event.ProjectPath,
		"ref":       event.RefName,
		"beforeSHA": event.BeforeSHA[:8],
		"afterSHA":  event.CommitSHA[:8],
		"mrNumber":  mrNumber,
	})

	if mrNumber > 0 {
		// Use MR-based promotion (has full context: changed files, labels, body metadata)
		prLogger.Infof("Detected merge commit from MR !%d, using MR-based promotion", mrNumber)

		repoURL := fmt.Sprintf("%s/%s", event.ServerURL, event.ProjectPath)
		details := gitlabapi.ProviderClientDetails{
			Ctx:      ctx,
			Provider: provider,
			Owner:    event.Owner,
			Repo:     event.Repo,
			PrNumber: mrNumber,
			Ref:      event.RefName,
			RepoURL:  repoURL,
			PrAuthor: event.CommitAuthor,
			PrLogger: prLogger,
		}

		return gitlabapi.HandleMergedMR(ctx, details, nil, defaultBranch)
	}

	// Fallback: use commit comparison for non-merge pushes
	prLogger.Info("No MR reference in commit message, using commit-based promotion")
	repoURL := fmt.Sprintf("%s/%s", event.ServerURL, event.ProjectPath)
	details := gitlabapi.ProviderClientDetails{
		Ctx:      ctx,
		Provider: provider,
		Owner:    event.Owner,
		Repo:     event.Repo,
		Ref:      event.RefName,
		RepoURL:  repoURL,
		PrAuthor: event.CommitAuthor,
		PrLogger: prLogger,
	}

	return gitlabapi.HandlePushPromotion(ctx, details, event.BeforeSHA, event.CommitSHA, defaultBranch)
}

// extractMRNumberFromCommitMessage parses the MR IID from a GitLab merge commit message.
// GitLab merge commits contain "See merge request group/project!123".
func extractMRNumberFromCommitMessage(message string) int {
	re := regexp.MustCompile(`See merge request .+!(\d+)`)
	matches := re.FindStringSubmatch(message)
	if len(matches) == 2 {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			return n
		}
	}
	return 0
}
