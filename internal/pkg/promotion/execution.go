package promotion

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cenkalti/backoff/v4"
	cfg "github.com/commercetools/telefonistka/internal/pkg/configuration"
	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	log "github.com/sirupsen/logrus"
)

// ExecutePromotion creates a branch, commit, PR/MR and optionally approves/merges for a single promotion.
// existingMetadata is parsed from the triggering PR/MR body (nil for first-hop or push-triggered promotions).
// prLinkPrefix is "!" for GitLab MRs and "#" for GitHub PRs.
func ExecutePromotion(
	ctx context.Context,
	provider gitprovider.GitProvider,
	approverProvider gitprovider.GitProvider,
	owner, repo, defaultBranch string,
	prNumber int,
	prBranch, prAuthor, repoURL string,
	config *cfg.Config,
	promotion PromotionInstance,
	existingMetadata *PrMetadata,
	prLinkPrefix string,
	prLogger *log.Entry,
) error {
	var allActions []*gitprovider.CommitAction
	for target, source := range promotion.ComputedSyncPaths {
		actions, err := GenerateSyncCommitActions(ctx, provider, owner, repo, source, target, defaultBranch, promotion.Metadata.BlockList, prLogger)
		if err != nil {
			prLogger.Errorf("Failed to generate sync actions for %s > %s: %v", source, target, err)
			return err
		}
		allActions = append(allActions, actions...)
	}

	if len(allActions) == 0 {
		prLogger.Infof("No changes to sync for promotion from %s", promotion.Metadata.SourcePath)
		return nil
	}

	ref, err := provider.GetRef(ctx, owner, repo, "refs/heads/"+defaultBranch)
	if err != nil {
		prLogger.Errorf("Failed to get default branch ref: %v", err)
		return err
	}

	newBranchName := GenerateSafePromotionBranchName(prNumber, prBranch, promotion.Metadata.TargetPaths)
	prLogger.Infof("Creating promotion branch: %s", newBranchName)

	if _, err = provider.GetBranch(ctx, owner, repo, newBranchName); err == nil {
		prLogger.Infof("Branch %s already exists, deleting before recreating", newBranchName)
		_ = provider.DeleteBranch(ctx, owner, repo, newBranchName)
	}

	_, err = provider.CreateBranch(ctx, owner, repo, newBranchName, ref.SHA)
	if err != nil {
		prLogger.Errorf("Failed to create branch %s: %v", newBranchName, err)
		return err
	}

	commitMsg := fmt.Sprintf("Syncing from %s", promotion.Metadata.SourcePath)
	_, err = provider.CreateCommit(ctx, owner, repo, &gitprovider.CommitOptions{
		Message:       commitMsg,
		Branch:        newBranchName,
		CommitActions: allActions,
	})
	if err != nil {
		prLogger.Errorf("Failed to create commit: %v", err)
		_ = provider.DeleteBranch(ctx, owner, repo, newBranchName)
		return err
	}

	originalPrAuthor := prAuthor
	if existingMetadata != nil && existingMetadata.OriginalPrAuthor != "" {
		originalPrAuthor = existingMetadata.OriginalPrAuthor
	}

	components := strings.Join(promotion.Metadata.ComponentNames, ",")
	prTitle := fmt.Sprintf("🚀 Promotion: %s ➡️  %s", components, promotion.Metadata.TargetDescription)
	prBody := GeneratePromotionPrBody(prNumber, components, promotion, originalPrAuthor, repoURL, existingMetadata, prLinkPrefix)

	newPR := &gitprovider.NewPullRequest{
		Title:     prTitle,
		Body:      prBody,
		Head:      newBranchName,
		Base:      defaultBranch,
		Labels:    config.PromtionPrLables,
		Assignees: []string{originalPrAuthor},
	}

	createdPR, err := provider.CreatePullRequest(ctx, owner, repo, newPR)
	if err != nil {
		prLogger.Errorf("Failed to create promotion PR: %v", err)
		return err
	}

	prLogger.Infof("Created promotion PR #%d: %s", createdPR.Number, createdPR.HTMLURL)

	if config.AutoApprovePromotionPrs && approverProvider != nil {
		_, err = approverProvider.ApprovePullRequest(ctx, owner, repo, createdPR.Number)
		if err != nil {
			prLogger.Errorf("Failed to auto-approve promotion PR #%d: %v", createdPR.Number, err)
		} else {
			prLogger.Infof("Auto-approved promotion PR #%d", createdPR.Number)
		}
	}

	if promotion.Metadata.AutoMerge {
		_, _ = provider.CommentOnPullRequest(ctx, owner, repo, createdPR.Number,
			fmt.Sprintf("Auto-merging promotion PR %s%d as configured.", prLinkPrefix, createdPR.Number))

		prLogger.Infof("Auto-merging promotion PR #%d", createdPR.Number)
		err = MergePrWithRetry(ctx, provider, owner, repo, createdPR.Number, components, promotion.Metadata.TargetDescription, prLogger)
		if err != nil {
			prLogger.Errorf("Failed to auto-merge promotion PR #%d: %v", createdPR.Number, err)
			_, _ = provider.CommentOnPullRequest(ctx, owner, repo, createdPR.Number,
				fmt.Sprintf("Auto-merge failed: %v\nPlease merge manually.", err))
		}
	}

	return nil
}

// MergePrWithRetry merges a PR with exponential backoff retry on transient errors.
func MergePrWithRetry(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo string,
	mrNumber int,
	components, targetDescription string,
	prLogger *log.Entry,
) error {
	operation := func() error {
		err := provider.MergePullRequest(ctx, owner, repo, mrNumber, &gitprovider.MergeOptions{
			MergeMethod: gitprovider.MergeMethodMerge,
		})
		if err != nil {
			errMsg := err.Error()
			if IsMergeErrorRetryable(errMsg) {
				prLogger.Warnf("Transient merge error for MR #%d, will retry: %v", mrNumber, err)
				return err
			}
			prLogger.Errorf("Permanent merge error for MR #%d: %v", mrNumber, err)
			return backoff.Permanent(err)
		}
		return nil
	}

	return backoff.Retry(operation, backoff.NewExponentialBackOff())
}

// IsMergeErrorRetryable checks if a merge error is transient and worth retrying.
func IsMergeErrorRetryable(errMessage string) bool {
	retryablePatterns := []string{
		"405",
		"try the merge again",
		"not yet ready",
		"cannot be merged",
		"merge request is not mergeable",
		"pipeline",
	}
	lower := strings.ToLower(errMessage)
	for _, pattern := range retryablePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// SetCommitStatus sets the commit status on the given SHA.
func SetCommitStatus(ctx context.Context, provider gitprovider.GitProvider, owner, repo, sha, state string, prLogger *log.Entry) {
	status := &gitprovider.Status{
		State:       state,
		Context:     "telefonistka",
		Description: "Telefonistka GitOps Bot",
	}
	err := provider.SetCommitStatus(ctx, owner, repo, sha, status)
	if err != nil {
		prLogger.Warnf("Failed to set commit status to %s: %v", state, err)
	}
}

// CommentPromotionPlan comments the dry-run promotion plan on the PR/MR.
func CommentPromotionPlan(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo string,
	prNumber int,
	promotions map[string]PromotionInstance,
	prLogger *log.Entry,
) {
	if len(promotions) == 0 {
		prLogger.Info("No promotions to report")
		return
	}

	var body strings.Builder
	body.WriteString("## Promotion Plan (Dry Run)\n\n")
	body.WriteString("The following promotions would be created:\n\n")

	for _, promotion := range promotions {
		components := strings.Join(promotion.Metadata.ComponentNames, ", ")
		body.WriteString(fmt.Sprintf("### %s -> %s\n", components, promotion.Metadata.TargetDescription))
		for target, source := range promotion.ComputedSyncPaths {
			body.WriteString(fmt.Sprintf("- `%s` <- `%s`\n", target, source))
		}
		if len(promotion.Metadata.PerComponentSkippedTargetPaths) > 0 {
			body.WriteString("\n**Skipped paths:**\n")
			for comp, paths := range promotion.Metadata.PerComponentSkippedTargetPaths {
				body.WriteString(fmt.Sprintf("- %s: %s\n", comp, strings.Join(paths, ", ")))
			}
		}
		body.WriteString("\n")
	}

	_, err := provider.CommentOnPullRequest(ctx, owner, repo, prNumber, body.String())
	if err != nil {
		prLogger.Errorf("Failed to comment promotion plan: %v", err)
	}
}

// GeneratePromotionPrBody creates the body text for a promotion PR/MR, including serialized metadata
// for chained promotions. prLinkPrefix is "!" for GitLab, "#" for GitHub.
func GeneratePromotionPrBody(prNumber int, components string, promotion PromotionInstance, originalAuthor, repoURL string, existingMetadata *PrMetadata, prLinkPrefix string) string {
	newMetadata := PrMetadata{
		OriginalPrAuthor: originalAuthor,
	}

	if existingMetadata != nil && existingMetadata.PreviousPromotionMetadata != nil {
		newMetadata.PreviousPromotionMetadata = existingMetadata.PreviousPromotionMetadata
	} else {
		newMetadata.PreviousPromotionMetadata = make(map[int]PromotionPathMetadata)
	}

	newMetadata.PreviousPromotionMetadata[prNumber] = PromotionPathMetadata{
		SourcePath:  promotion.Metadata.SourcePath,
		TargetPaths: promotion.Metadata.TargetPaths,
	}

	promotedPaths := make([]string, 0, len(promotion.ComputedSyncPaths))
	for k := range promotion.ComputedSyncPaths {
		promotedPaths = append(promotedPaths, k)
	}
	newMetadata.PromotedPaths = promotedPaths

	var body strings.Builder

	body.WriteString(fmt.Sprintf("Promotion path(%s):\n\n", components))

	keys := make([]int, 0, len(newMetadata.PreviousPromotionMetadata))
	for k := range newMetadata.PreviousPromotionMetadata {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	const indent = "&nbsp;&nbsp;&nbsp;&nbsp;"
	for i, k := range keys {
		meta := newMetadata.PreviousPromotionMetadata[k]
		targetPaths := make([]string, len(meta.TargetPaths))
		copy(targetPaths, meta.TargetPaths)
		sort.Strings(targetPaths)
		tp := strings.Join(targetPaths, fmt.Sprintf("`  \n%s`", strings.Repeat(indent, i+1)))
		var prRef string
		if k == 0 {
			prRef = "push"
		} else if repoURL != "" {
			prRef = fmt.Sprintf("[%s%d](%s/-/merge_requests/%d)", prLinkPrefix, k, repoURL, k)
		} else {
			prRef = fmt.Sprintf("%s%d", prLinkPrefix, k)
		}
		body.WriteString(fmt.Sprintf("%s↘️  %s  `%s` ➡️  \n%s`%s`  \n",
			strings.Repeat(indent, i), prRef, meta.SourcePath, strings.Repeat(indent, i+1), tp))
	}

	if len(promotion.Metadata.PerComponentSkippedTargetPaths) > 0 {
		body.WriteString("\n**Skipped target paths:**\n")
		for comp, paths := range promotion.Metadata.PerComponentSkippedTargetPaths {
			body.WriteString(fmt.Sprintf("- **%s**: %s\n", comp, strings.Join(paths, ", ")))
		}
	}

	metadataString, _ := newMetadata.Serialize()
	body.WriteString("\n<!--|Telefonistka data, do not delete|" + metadataString + "|-->")

	return body.String()
}
