package telefonistka

import (
	"context"
	"os"

	"github.com/commercetools/telefonistka/internal/pkg/githubapi"
	"github.com/commercetools/telefonistka/internal/pkg/gitlabci"
	lru "github.com/hashicorp/golang-lru/v2"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// This is still(https://github.com/spf13/cobra/issues/1862) the documented way to use cobra
func init() { //nolint:gochecknoinits
	var eventType string
	var eventFilePath string
	var provider string

	eventCmd := &cobra.Command{
		Use:   "event",
		Short: "Handles a GitHub or GitLab event based on event JSON file or CI environment",
		Long: `Handles a GitHub or GitLab event based on event JSON file or CI environment.
This operation mode was built with GitHub Actions and GitLab CI in mind.

For GitHub Actions: Uses GITHUB_EVENT_NAME and GITHUB_EVENT_PATH
For GitLab CI: Reads from CI environment variables (CI_MERGE_REQUEST_*, etc.)`,
		Args: cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			handleEvent(provider, eventType, eventFilePath)
		},
	}

	eventCmd.Flags().StringVarP(&provider, "provider", "p", detectProvider(), "Git provider: github or gitlab (auto-detected from environment)")
	eventCmd.Flags().StringVarP(&eventType, "type", "t", detectEventType(), "Event type, defaults to GITHUB_EVENT_NAME or CI_PIPELINE_SOURCE env var")
	eventCmd.Flags().StringVarP(&eventFilePath, "file", "f", detectEventFilePath(), "File path for event JSON, defaults to GITHUB_EVENT_PATH env var")

	rootCmd.AddCommand(eventCmd)
}

// detectProvider auto-detects the Git provider from environment variables
func detectProvider() string {
	if os.Getenv("GITLAB_CI") != "" {
		return "gitlab"
	}
	if os.Getenv("GITHUB_ACTIONS") != "" {
		return "github"
	}
	// Check for explicit override
	if p := os.Getenv("TELEFONISTKA_PROVIDER"); p != "" {
		return p
	}
	// Default to GitHub for backward compatibility
	return "github"
}

// detectEventType detects the event type from environment
func detectEventType() string {
	// GitHub Actions
	if ghEvent := os.Getenv("GITHUB_EVENT_NAME"); ghEvent != "" {
		return ghEvent
	}
	// GitLab CI
	if glPipeline := os.Getenv("CI_PIPELINE_SOURCE"); glPipeline != "" {
		return glPipeline
	}
	return ""
}

// detectEventFilePath detects the event file path from environment
func detectEventFilePath() string {
	// GitHub Actions
	if ghPath := os.Getenv("GITHUB_EVENT_PATH"); ghPath != "" {
		return ghPath
	}
	// GitLab CI doesn't use event files, events are built from env vars
	return ""
}

// handleEvent routes to the appropriate provider handler
func handleEvent(provider string, eventType string, eventFilePath string) {
	log.Infof("Processing event: provider=%s, type=%s", provider, eventType)

	switch provider {
	case "gitlab":
		handleGitLabEvent(eventType, eventFilePath)
	case "github":
		handleGitHubEvent(eventType, eventFilePath)
	default:
		log.Fatalf("Unknown provider: %s (must be 'github' or 'gitlab')", provider)
	}
}

// handleGitLabEvent processes GitLab events
func handleGitLabEvent(eventType string, eventFilePath string) {
	ctx := context.Background()

	if eventFilePath != "" {
		// Event file mode (not typically used in GitLab CI, but supported)
		log.Infof("Processing GitLab event from file: %s", eventFilePath)
		// TODO: Implement file-based event handling for GitLab
		log.Fatal("GitLab event file mode not yet implemented. Use CI environment mode instead.")
	} else {
		// CI environment mode (standard for GitLab CI)
		log.Info("Processing GitLab event from CI environment variables")
		err := gitlabci.ProcessEventInCI(ctx)
		if err != nil {
			log.Fatalf("Failed to process GitLab CI event: %v", err)
		}
		log.Info("GitLab event processed successfully")
	}
}

// handleGitHubEvent processes GitHub events (original implementation)
func handleGitHubEvent(eventType string, eventFilePath string) {
	mainGhClientCache, _ := lru.New[string, githubapi.GhClientPair](128)
	prApproverGhClientCache, _ := lru.New[string, githubapi.GhClientPair](128)
	githubapi.ReciveEventFile(eventType, eventFilePath, mainGhClientCache, prApproverGhClientCache)
}

// getEnv returns environment variable value or fallback
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
