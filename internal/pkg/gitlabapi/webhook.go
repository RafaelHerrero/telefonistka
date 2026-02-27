package gitlabapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	_ "github.com/commercetools/telefonistka/internal/pkg/gitprovider/gitlab"
	"github.com/commercetools/telefonistka/internal/pkg/prometheus"
	lru "github.com/hashicorp/golang-lru/v2"
	log "github.com/sirupsen/logrus"
)

// ProviderClientDetails wraps a GitProvider with context and metadata
// This is similar to githubapi.GhPrClientDetails but provider-agnostic
type ProviderClientDetails struct {
	Ctx      context.Context
	Provider gitprovider.GitProvider
	Owner    string
	Repo     string
	PrNumber int
	PrSHA    string
	Ref      string
	RepoURL  string
	PrAuthor string
	PrLogger *log.Entry
	Labels   []string
}

// ReciveGitLabWebhook parses and validates GitLab webhook, then handles the event
func ReciveGitLabWebhook(
	r *http.Request,
	mainProviderCache *lru.Cache[string, gitprovider.GitProvider],
	approverProviderCache *lru.Cache[string, gitprovider.GitProvider],
	webhookSecret []byte,
) error {
	// Create provider factory
	factory, err := gitprovider.NewDefaultProviderFactory(10)
	if err != nil {
		log.Errorf("Failed to create provider factory: %v", err)
		prometheus.InstrumentWebhookHit("factory_creation_failed")
		return err
	}

	// Get or create GitLab provider
	gitlabURL := os.Getenv("GITLAB_URL")
	if gitlabURL == "" {
		gitlabURL = "https://gitlab.com"
	}

	gitlabToken := os.Getenv("GITLAB_TOKEN")
	if gitlabToken == "" {
		log.Error("GITLAB_TOKEN environment variable is required")
		prometheus.InstrumentWebhookHit("missing_token")
		return fmt.Errorf("GITLAB_TOKEN is required")
	}

	config := &gitprovider.ProviderConfig{
		Type:    gitprovider.ProviderTypeGitLab,
		Token:   gitlabToken,
		BaseURL: gitlabURL,
	}

	// Get provider from cache or create new
	cacheKey := fmt.Sprintf("gitlab:%s", gitlabURL)
	var provider gitprovider.GitProvider

	if cached, ok := mainProviderCache.Get(cacheKey); ok {
		provider = cached
		log.Debug("Using cached GitLab provider")
	} else {
		provider, err = factory.Create(config)
		if err != nil {
			log.Errorf("Failed to create GitLab provider: %v", err)
			prometheus.InstrumentWebhookHit("provider_creation_failed")
			return err
		}
		mainProviderCache.Add(cacheKey, provider)
		log.Info("Created new GitLab provider")
	}

	// Validate webhook signature if secret is provided
	if len(webhookSecret) > 0 {
		err = provider.ValidateWebhookSignature(r, webhookSecret)
		if err != nil {
			log.Errorf("Webhook signature validation failed: %v", err)
			prometheus.InstrumentWebhookHit("validation_failed")
			return err
		}
	}

	// Parse webhook event
	event, err := provider.ParseWebhook(r, webhookSecret)
	if err != nil {
		log.Errorf("Failed to parse webhook: %v", err)
		prometheus.InstrumentWebhookHit("parsing_failed")
		return err
	}

	prometheus.InstrumentWebhookHit("successful")

	// Handle event asynchronously
	go HandleGitLabEvent(event, provider, nil, mainProviderCache, approverProviderCache)

	return nil
}

// HandleGitLabEvent processes different types of GitLab events
func HandleGitLabEvent(
	event gitprovider.Event,
	mainProvider gitprovider.GitProvider,
	approverProvider gitprovider.GitProvider,
	mainProviderCache *lru.Cache[string, gitprovider.GitProvider],
	approverProviderCache *lru.Cache[string, gitprovider.GitProvider],
) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Recovered from panic in HandleGitLabEvent: %v", r)
		}
	}()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Infof("Handling GitLab event type: %s", event.Type())

	switch e := event.(type) {
	case gitprovider.PullRequestEvent:
		handleMergeRequestEvent(ctx, e, mainProvider, approverProvider)
	case gitprovider.PushEvent:
		handlePushEvent(ctx, e, mainProvider)
	case gitprovider.IssueCommentEvent:
		handleCommentEvent(ctx, e, mainProvider)
	default:
		log.Warnf("Unhandled event type: %T", event)
	}
}

// handleMergeRequestEvent handles GitLab merge request events
func handleMergeRequestEvent(
	ctx context.Context,
	event gitprovider.PullRequestEvent,
	mainProvider gitprovider.GitProvider,
	approverProvider gitprovider.GitProvider,
) {
	pr := event.PullRequest()
	repo := event.Repository()
	action := event.Action()

	log.Infof("Handling MR event: action=%s, MR#%d, repo=%s", action, pr.Number, repo.FullName)

	prLogger := log.WithFields(log.Fields{
		"repo":       repo.FullName,
		"mrNumber":   pr.Number,
		"event_type": "merge_request",
		"action":     action,
	})

	// Extract owner and repo from FullName
	owner, repoName := parseRepoFullName(repo.FullName)

	clientDetails := ProviderClientDetails{
		Ctx:      ctx,
		Provider: mainProvider,
		Owner:    owner,
		Repo:     repoName,
		PrNumber: pr.Number,
		PrSHA:    pr.HeadSHA,
		Ref:      pr.HeadRef,
		RepoURL:  repo.HTMLURL,
		PrAuthor: pr.Author,
		PrLogger: prLogger,
		Labels:   pr.Labels,
	}

	switch action {
	case "opened", "synchronize", "update":
		// Check for drift, add warnings
		prLogger.Info("MR opened or updated - checking for drift")
		err := handleMROpenedOrUpdated(ctx, clientDetails)
		if err != nil {
			prLogger.Errorf("Error handling MR opened/updated: %v", err)
		}

	case "merged", "closed":
		if pr.Merged {
			// Handle promotion workflow
			prLogger.Info("MR merged - starting promotion workflow")
			err := handleMRMerged(ctx, clientDetails, approverProvider)
			if err != nil {
				prLogger.Errorf("Error handling MR merged: %v", err)
			}
		} else {
			prLogger.Info("MR closed without merge - no action needed")
		}

	default:
		prLogger.Debugf("Ignoring MR action: %s", action)
	}
}

// handlePushEvent handles GitLab push events
func handlePushEvent(
	ctx context.Context,
	event gitprovider.PushEvent,
	provider gitprovider.GitProvider,
) {
	repo := event.Repository()
	log.Infof("Handling push event: repo=%s, ref=%s", repo.FullName, event.Ref())

	// Push events can be used for webhook multiplexing or other features
	// For now, just log it
	log.Debugf("Push event: %d commits to %s", len(event.Commits()), event.Ref())
}

// handleCommentEvent handles GitLab comment/note events
func handleCommentEvent(
	ctx context.Context,
	event gitprovider.IssueCommentEvent,
	provider gitprovider.GitProvider,
) {
	pr := event.Issue()
	comment := event.Comment()
	repo := event.Repository()

	log.Infof("Handling comment event: MR#%d, comment=%s", pr.Number, comment.Body[:min(50, len(comment.Body))])

	// Extract owner and repo
	owner, repoName := parseRepoFullName(repo.FullName)

	prLogger := log.WithFields(log.Fields{
		"repo":       repo.FullName,
		"mrNumber":   pr.Number,
		"event_type": "comment",
	})

	clientDetails := ProviderClientDetails{
		Ctx:      ctx,
		Provider: provider,
		Owner:    owner,
		Repo:     repoName,
		PrNumber: pr.Number,
		PrLogger: prLogger,
	}

	// Handle bot commands in comments (like /promote, /sync, etc.)
	err := handleBotCommands(ctx, clientDetails, comment.Body, event.Sender())
	if err != nil {
		prLogger.Errorf("Error handling bot command: %v", err)
	}
}

// handleMROpenedOrUpdated checks for drift and adds warnings
func handleMROpenedOrUpdated(ctx context.Context, details ProviderClientDetails) error {
	// TODO: Implement drift detection
	// For now, just log
	details.PrLogger.Info("Drift detection not yet implemented for GitLab")
	return nil
}

// handleMRMerged handles the promotion workflow when an MR is merged
func handleMRMerged(
	ctx context.Context,
	details ProviderClientDetails,
	approverProvider gitprovider.GitProvider,
) error {
	details.PrLogger.Info("Starting promotion workflow for merged MR")

	// TODO: Implement promotion logic using the provider abstraction
	// This would be similar to githubapi.handleMergedPrEvent but using GitProvider interface

	// For now, just create a test comment to verify webhook works
	comment := fmt.Sprintf("🎉 MR #%d was merged! Promotion workflow would start here.", details.PrNumber)

	_, err := details.Provider.CommentOnPullRequest(
		ctx,
		details.Owner,
		details.Repo,
		details.PrNumber,
		comment,
	)

	if err != nil {
		details.PrLogger.Errorf("Failed to add comment: %v", err)
		return err
	}

	details.PrLogger.Info("Promotion workflow completed successfully")
	return nil
}

// handleBotCommands processes bot commands from MR comments
func handleBotCommands(
	ctx context.Context,
	details ProviderClientDetails,
	commentBody string,
	sender string,
) error {
	// TODO: Implement bot command handling
	// Examples: /promote, /sync, /approve, etc.
	details.PrLogger.Debugf("Bot command handling not yet implemented: %s", commentBody)
	return nil
}

// parseRepoFullName splits "owner/repo" into separate strings
func parseRepoFullName(fullName string) (owner, repo string) {
	// GitLab can have nested groups like "group/subgroup/repo"
	// For now, we'll treat everything before the last slash as owner
	// and everything after as repo

	// Simple approach: split by last /
	for i := len(fullName) - 1; i >= 0; i-- {
		if fullName[i] == '/' {
			return fullName[:i], fullName[i+1:]
		}
	}

	// If no slash found, return as-is
	return "", fullName
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
