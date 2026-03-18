package gitlabapi

import (
	"context"
	"fmt"

	cfg "github.com/commercetools/telefonistka/internal/pkg/configuration"
	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	prom "github.com/commercetools/telefonistka/internal/pkg/promotion"
	log "github.com/sirupsen/logrus"
)

type prMetadata = prom.PrMetadata

type PromotionInstance = prom.PromotionInstance
type PromotionInstanceMetaData = prom.PromotionInstanceMetaData

// generatePromotionPlan delegates to the shared promotion package.
func generatePromotionPlan(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo string,
	changedFiles []string,
	labels []string,
	config *cfg.Config,
	defaultBranch string,
	prLogger *log.Entry,
) (map[string]PromotionInstance, error) {
	return prom.GeneratePromotionPlan(ctx, provider, owner, repo, changedFiles, labels, config, defaultBranch, prLogger)
}

// executePromotion delegates to the shared promotion package with GitLab "!" link prefix.
func executePromotion(
	ctx context.Context,
	provider gitprovider.GitProvider,
	approverProvider gitprovider.GitProvider,
	owner, repo, defaultBranch string,
	prNumber int,
	prBranch, prAuthor, repoURL string,
	config *cfg.Config,
	promotion PromotionInstance,
	existingMetadata *prMetadata,
	prLogger *log.Entry,
) error {
	return prom.ExecutePromotion(ctx, provider, approverProvider, owner, repo, defaultBranch,
		prNumber, prBranch, prAuthor, repoURL, config, promotion, existingMetadata, "!", prLogger)
}

// setCommitStatus delegates to the shared promotion package.
func setCommitStatus(ctx context.Context, provider gitprovider.GitProvider, owner, repo, sha, state string, prLogger *log.Entry) {
	prom.SetCommitStatus(ctx, provider, owner, repo, sha, state, prLogger)
}

// commentPromotionPlan delegates to the shared promotion package.
func commentPromotionPlan(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo string,
	prNumber int,
	promotions map[string]PromotionInstance,
	prLogger *log.Entry,
) {
	prom.CommentPromotionPlan(ctx, provider, owner, repo, prNumber, promotions, prLogger)
}

// DetectDrift compares source and target directories for each promotion path
// and returns a comment body describing any drift found.
func DetectDrift(ctx context.Context, details ProviderClientDetails) error {
	details.PrLogger.Debug("Checking for drift")
	if ctx.Err() != nil {
		return ctx.Err()
	}

	defaultBranch, err := details.Provider.GetDefaultBranch(ctx, details.Owner, details.Repo)
	if err != nil {
		return fmt.Errorf("failed to get default branch: %w", err)
	}

	config, err := prom.GetRepoConfig(ctx, details.Provider, details.Owner, details.Repo, defaultBranch, details.PrLogger)
	if err != nil {
		_, _ = details.Provider.CommentOnPullRequest(ctx, details.Owner, details.Repo, details.PrNumber,
			fmt.Sprintf("Failed to get configuration\n```\n%s\n```\n", err))
		return err
	}

	mrFiles, err := details.Provider.ListPullRequestFiles(ctx, details.Owner, details.Repo, details.PrNumber)
	if err != nil {
		details.PrLogger.Errorf("Failed to list MR files for drift detection: %v", err)
		return err
	}
	changedFiles := make([]string, 0, len(mrFiles))
	for _, f := range mrFiles {
		changedFiles = append(changedFiles, f.Filename)
	}

	promotions, err := generatePromotionPlan(ctx, details.Provider, details.Owner, details.Repo, changedFiles, details.Labels, config, defaultBranch, details.PrLogger)
	if err != nil {
		return err
	}

	diffOutputMap := make(map[string]string)
	for _, promotion := range promotions {
		details.PrLogger.Debugf("Checking drift for %s", promotion.Metadata.SourcePath)
		for target, source := range promotion.ComputedSyncPaths {
			blamePrefix := fmt.Sprintf("%s/-/blame/HEAD", details.RepoURL)
			hasDiff, diffOutput, err := prom.CompareRepoDirectories(ctx, details.Provider, details.Owner, details.Repo, source, target, defaultBranch, blamePrefix, promotion.Metadata.BlockList, details.PrLogger)
			if err != nil {
				details.PrLogger.Warnf("Error comparing %s vs %s: %v", source, target, err)
				continue
			}
			if hasDiff {
				mapKey := fmt.Sprintf("`%s` ↔️  `%s`", source, target)
				diffOutputMap[mapKey] = diffOutput
				details.PrLogger.Debugf("Found diff @ %s", mapKey)
			}
		}
	}

	if len(diffOutputMap) > 0 {
		comment := prom.GenerateDriftComment(diffOutputMap)
		_, err = details.Provider.CommentOnPullRequest(ctx, details.Owner, details.Repo, details.PrNumber, comment)
		if err != nil {
			details.PrLogger.Errorf("Failed to comment drift warning: %v", err)
			return err
		}
	} else {
		details.PrLogger.Info("No drift found")
	}

	return nil
}

// HandlePushPromotion handles the promotion workflow triggered by a push to the default branch.
func HandlePushPromotion(ctx context.Context, details ProviderClientDetails, beforeSHA, afterSHA, defaultBranch string) error {
	prLogger := details.PrLogger

	config, err := prom.GetRepoConfig(ctx, details.Provider, details.Owner, details.Repo, defaultBranch, prLogger)
	if err != nil {
		prLogger.Errorf("Failed to get repo config: %v", err)
		return err
	}

	diff, err := details.Provider.CompareCommits(ctx, details.Owner, details.Repo, beforeSHA, afterSHA)
	if err != nil {
		prLogger.Errorf("Failed to compare commits %s..%s: %v", beforeSHA[:8], afterSHA[:8], err)
		return err
	}

	changedFiles := make([]string, 0, len(diff.Files))
	for _, f := range diff.Files {
		changedFiles = append(changedFiles, f.Filename)
	}

	if len(changedFiles) == 0 {
		prLogger.Info("No changed files in push, skipping promotion")
		return nil
	}

	prLogger.Infof("Found %d changed files in push", len(changedFiles))

	promotions, err := generatePromotionPlan(ctx, details.Provider, details.Owner, details.Repo, changedFiles, nil, config, defaultBranch, prLogger)
	if err != nil {
		prLogger.Errorf("Failed to generate promotion plan: %v", err)
		return err
	}

	if len(promotions) == 0 {
		prLogger.Info("No promotions needed for this push")
		return nil
	}

	for _, promotion := range promotions {
		err := executePromotion(ctx, details.Provider, nil, details.Owner, details.Repo, defaultBranch,
			0, details.Ref, details.PrAuthor, details.RepoURL, config, promotion, nil, prLogger)
		if err != nil {
			prLogger.Errorf("Promotion failed for %s: %v", promotion.Metadata.SourcePath, err)
		}
	}

	prLogger.Info("Push promotion workflow completed")
	return nil
}
