package telefonistka

import (
	"net/http"
	"os"
	"time"

	"github.com/alexliesenfeld/health"
	"github.com/commercetools/telefonistka/internal/pkg/githubapi"
	"github.com/commercetools/telefonistka/internal/pkg/gitlabapi"
	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func getCrucialEnv(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	log.Fatalf("%s environment variable is required", key)
	os.Exit(3)
	return ""
}

var serveCmd = &cobra.Command{
	Use:   "server",
	Short: "Runs the web server that listens to GitHub and GitLab webhooks",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

// This is still(https://github.com/spf13/cobra/issues/1862) the documented way to use cobra
func init() { //nolint:gochecknoinits
	rootCmd.AddCommand(serveCmd)
}

func handleWebhook(
	githubWebhookSecret []byte,
	gitlabWebhookSecret []byte,
	mainGhClientCache *lru.Cache[string, githubapi.GhClientPair],
	prApproverGhClientCache *lru.Cache[string, githubapi.GhClientPair],
	mainProviderCache *lru.Cache[string, gitprovider.GitProvider],
	approverProviderCache *lru.Cache[string, gitprovider.GitProvider],
) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Detect provider from webhook headers
		providerType := gitprovider.DetectProviderFromWebhook(r.Header)

		log.Infof("Received webhook from provider: %s", providerType)

		var err error

		switch providerType {
		case gitprovider.ProviderTypeGitLab:
			// Handle GitLab webhook
			err = gitlabapi.ReciveGitLabWebhook(r, mainProviderCache, approverProviderCache, gitlabWebhookSecret)

		case gitprovider.ProviderTypeGitHub:
			// Handle GitHub webhook
			err = githubapi.ReciveWebhook(r, mainGhClientCache, prApproverGhClientCache, githubWebhookSecret)

		default:
			// Default to GitHub for backward compatibility
			log.Warn("Unknown provider type, defaulting to GitHub")
			err = githubapi.ReciveWebhook(r, mainGhClientCache, prApproverGhClientCache, githubWebhookSecret)
		}

		if err != nil {
			log.Errorf("error handling webhook: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func serve() {
	// GitHub webhook secret (optional for backward compatibility)
	githubWebhookSecret := []byte(os.Getenv("GITHUB_WEBHOOK_SECRET"))
	if len(githubWebhookSecret) == 0 {
		log.Warn("GITHUB_WEBHOOK_SECRET not set, webhook signature validation disabled for GitHub")
	}

	// GitLab webhook secret (optional)
	gitlabWebhookSecret := []byte(os.Getenv("GITLAB_WEBHOOK_SECRET"))
	if len(gitlabWebhookSecret) == 0 {
		log.Warn("GITLAB_WEBHOOK_SECRET not set, webhook signature validation disabled for GitLab")
	}

	livenessChecker := health.NewChecker() // No checks for the moment, other then the http server availability
	readinessChecker := health.NewChecker()

	// GitHub client caches (for backward compatibility)
	mainGhClientCache, _ := lru.New[string, githubapi.GhClientPair](128)
	prApproverGhClientCache, _ := lru.New[string, githubapi.GhClientPair](128)

	// GitProvider caches (for GitLab and future providers)
	mainProviderCache, _ := lru.New[string, gitprovider.GitProvider](128)
	approverProviderCache, _ := lru.New[string, gitprovider.GitProvider](128)

	go githubapi.MainGhMetricsLoop(mainGhClientCache)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", handleWebhook(
		githubWebhookSecret,
		gitlabWebhookSecret,
		mainGhClientCache,
		prApproverGhClientCache,
		mainProviderCache,
		approverProviderCache,
	))
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/live", health.NewHandler(livenessChecker))
	mux.Handle("/ready", health.NewHandler(readinessChecker))

	srv := &http.Server{
		Handler:      mux,
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Infoln("Server started on :8080")
	log.Infoln("Webhook endpoint: http://localhost:8080/webhook")
	log.Infoln("Supports both GitHub and GitLab webhooks")
	log.Fatal(srv.ListenAndServe())
}
