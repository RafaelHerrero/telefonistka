package gitlabapi

import (
	"context"
	"crypto/sha1" //nolint:gosec // G505: not a cryptographic use case
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cenkalti/backoff/v4"
	cfg "github.com/commercetools/telefonistka/internal/pkg/configuration"
	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// prMetadata is serialized into the MR body to enable chained promotions.
// When a promotion MR is itself merged, this metadata is parsed to carry
// the original author and promotion history forward.
type prMetadata struct {
	OriginalPrAuthor          string                              `json:"originalPrAuthor"`
	OriginalPrNumber          int                                 `json:"originalPrNumber"`
	PromotedPaths             []string                            `json:"promotedPaths"`
	PreviousPromotionMetadata map[int]promotionInstanceMetaDataSe `json:"previousPromotionPaths"`
}

// promotionInstanceMetaDataSe is the serializable subset of PromotionInstanceMetaData
type promotionInstanceMetaDataSe struct {
	SourcePath  string   `json:"sourcePath"`
	TargetPaths []string `json:"targetPaths"`
}

func (pm prMetadata) serialize() (string, error) {
	pmJSON, err := json.Marshal(pm)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pmJSON), nil
}

func (pm *prMetadata) deserialize(s string) error {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, pm)
}

// parsePrMetadata extracts serialized metadata from an MR body
func parsePrMetadata(body string) *prMetadata {
	metadataRegex := regexp.MustCompile(`<!--\|.*\|(.*)\|-->`)
	matches := metadataRegex.FindStringSubmatch(body)
	if len(matches) == 2 && matches[1] != "" {
		pm := &prMetadata{}
		if err := pm.deserialize(matches[1]); err != nil {
			return nil
		}
		return pm
	}
	return nil
}

// PromotionInstance represents a single promotion operation (one PR to create)
type PromotionInstance struct {
	Metadata          PromotionInstanceMetaData
	ComputedSyncPaths map[string]string // key=target, value=source
}

// PromotionInstanceMetaData contains metadata about a promotion
type PromotionInstanceMetaData struct {
	SourcePath                     string
	TargetPaths                    []string
	TargetDescription              string
	PerComponentSkippedTargetPaths map[string][]string
	ComponentNames                 []string
	AutoMerge                      bool
}

type relevantComponent struct {
	SourcePath    string
	ComponentName string
	AutoMerge     bool
}

// generatePromotionPlan generates a map of promotions based on changed files and config.
// changedFiles is a list of file paths that changed (from MR files or commit comparison).
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
	// Step 1: identify relevant components from changed files
	relevantComponents := identifyRelevantComponents(changedFiles, config, prLogger)

	// Step 2: generate plan from components
	return generatePlanFromComponents(ctx, provider, owner, repo, config, relevantComponents, labels, defaultBranch, prLogger)
}

// identifyRelevantComponents extracts component names from changed files based on config PromotionPaths
func identifyRelevantComponents(changedFiles []string, config *cfg.Config, prLogger *log.Entry) map[relevantComponent]struct{} {
	relevantComponents := make(map[relevantComponent]struct{})

	for _, filename := range changedFiles {
		for _, promotionPathConfig := range config.PromotionPaths {
			if match, _ := regexp.MatchString("^"+promotionPathConfig.SourcePath+".*", filename); match {
				// Extract component name from nested directories
				componentPathRegexSubStrings := []string{}
				for i := 0; i <= promotionPathConfig.ComponentPathExtraDepth; i++ {
					componentPathRegexSubStrings = append(componentPathRegexSubStrings, "[^/]*")
				}
				componentPathRegexSubString := strings.Join(componentPathRegexSubStrings, "/")
				getComponentRegexString := regexp.MustCompile("^" + promotionPathConfig.SourcePath + "(" + componentPathRegexSubString + ")/.*")
				componentName := getComponentRegexString.ReplaceAllString(filename, "${1}")

				getSourcePathRegexString := regexp.MustCompile("^(" + promotionPathConfig.SourcePath + ")" + componentName + "/.*")
				compiledSourcePath := getSourcePathRegexString.ReplaceAllString(filename, "${1}")

				rc := relevantComponent{
					SourcePath:    compiledSourcePath,
					ComponentName: componentName,
					AutoMerge:     promotionPathConfig.Conditions.AutoMerge,
				}
				relevantComponents[rc] = struct{}{}
				break // a file can only be a single "source dir"
			}
		}
	}
	return relevantComponents
}

// generatePlanFromComponents creates PromotionInstances by matching components against config
func generatePlanFromComponents(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo string,
	config *cfg.Config,
	relevantComponents map[relevantComponent]struct{},
	labels []string,
	configBranch string,
	prLogger *log.Entry,
) (map[string]PromotionInstance, error) {
	promotions := make(map[string]PromotionInstance)

	for componentToPromote := range relevantComponents {
		componentConfig, err := getComponentConfig(ctx, provider, owner, repo, componentToPromote.SourcePath+componentToPromote.ComponentName, configBranch, prLogger)
		if err != nil {
			prLogger.Errorf("Failed to get in component configuration, err=%s, skipping %s", err, componentToPromote.SourcePath+componentToPromote.ComponentName)
		}

		for _, configPromotionPath := range config.PromotionPaths {
			if match, _ := regexp.MatchString(configPromotionPath.SourcePath, componentToPromote.SourcePath); match {
				// Check label conditions
				if configPromotionPath.Conditions.PrHasLabels != nil {
					hasRightLabel := false
					for _, l := range labels {
						if containsString(configPromotionPath.Conditions.PrHasLabels, l) {
							hasRightLabel = true
							break
						}
					}
					if !hasRightLabel {
						continue
					}
				}

				for _, ppr := range configPromotionPath.PromotionPrs {
					sort.Strings(ppr.TargetPaths)

					mapKey := configPromotionPath.SourcePath + ">" + strings.Join(ppr.TargetPaths, "|")
					if entry, ok := promotions[mapKey]; !ok {
						if ppr.TargetDescription == "" {
							ppr.TargetDescription = strings.Join(ppr.TargetPaths, " ")
						}
						promotions[mapKey] = PromotionInstance{
							Metadata: PromotionInstanceMetaData{
								TargetPaths:                    ppr.TargetPaths,
								TargetDescription:              ppr.TargetDescription,
								SourcePath:                     componentToPromote.SourcePath,
								ComponentNames:                 []string{componentToPromote.ComponentName},
								PerComponentSkippedTargetPaths: map[string][]string{},
								AutoMerge:                      componentToPromote.AutoMerge,
							},
							ComputedSyncPaths: map[string]string{},
						}
					} else if !containsString(entry.Metadata.ComponentNames, componentToPromote.ComponentName) {
						entry.Metadata.ComponentNames = append(entry.Metadata.ComponentNames, componentToPromote.ComponentName)
						promotions[mapKey] = entry
					}

					for _, individualPath := range ppr.TargetPaths {
						if componentConfig != nil {
							if componentConfig.PromotionTargetBlockList != nil {
								if containMatchingRegex(componentConfig.PromotionTargetBlockList, individualPath) {
									promotions[mapKey].Metadata.PerComponentSkippedTargetPaths[componentToPromote.ComponentName] = append(
										promotions[mapKey].Metadata.PerComponentSkippedTargetPaths[componentToPromote.ComponentName], individualPath)
									continue
								}
							}
							if componentConfig.PromotionTargetAllowList != nil {
								if !containMatchingRegex(componentConfig.PromotionTargetAllowList, individualPath) {
									promotions[mapKey].Metadata.PerComponentSkippedTargetPaths[componentToPromote.ComponentName] = append(
										promotions[mapKey].Metadata.PerComponentSkippedTargetPaths[componentToPromote.ComponentName], individualPath)
									continue
								}
							}
						}
						promotions[mapKey].ComputedSyncPaths[individualPath+componentToPromote.ComponentName] = componentToPromote.SourcePath + componentToPromote.ComponentName
					}
				}
				break
			}
		}
	}
	return promotions, nil
}

// getComponentConfig loads an optional per-component telefonistka.yaml
func getComponentConfig(ctx context.Context, provider gitprovider.GitProvider, owner, repo, componentPath, branch string, prLogger *log.Entry) (*cfg.ComponentConfig, error) {
	componentConfig := &cfg.ComponentConfig{}
	content, err := provider.GetFileContent(ctx, owner, repo, componentPath+"/telefonistka.yaml", branch)
	if err != nil {
		// The file is optional - if not found, return empty config
		prLogger.Debugf("No in-component config in %s: %v", componentPath, err)
		return &cfg.ComponentConfig{}, nil
	}

	err = yaml.Unmarshal(content, componentConfig)
	if err != nil {
		prLogger.Errorf("Failed to parse component configuration at %s: %v", componentPath, err)
		return nil, err
	}
	return componentConfig, nil
}

// generateSyncCommitActions creates CommitActions to sync files from source to target directory.
// It handles create/update and delete operations.
func generateSyncCommitActions(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo, sourcePath, targetPath, ref string,
	prLogger *log.Entry,
) ([]*gitprovider.CommitAction, error) {
	// List source files recursively
	sourceFiles, err := listFilesRecursive(ctx, provider, owner, repo, sourcePath, ref)
	if err != nil {
		// Source dir doesn't exist - treat as deletion of all target files
		prLogger.Infof("Source directory %s not found, assuming deletion PR", sourcePath)
		return generateDeleteActions(ctx, provider, owner, repo, targetPath, ref, prLogger)
	}

	// List target files recursively (to detect deletions and distinguish create vs update)
	targetFiles, _ := listFilesRecursive(ctx, provider, owner, repo, targetPath, ref)

	// Build a set of existing target file relative paths
	targetRelativePaths := make(map[string]struct{})
	for _, file := range targetFiles {
		relativePath := strings.TrimPrefix(file.Path, targetPath)
		relativePath = strings.TrimPrefix(relativePath, "/")
		targetRelativePaths[relativePath] = struct{}{}
	}

	// Build a map of relative paths in source
	sourceRelativePaths := make(map[string]struct{})
	var actions []*gitprovider.CommitAction

	for _, file := range sourceFiles {
		relativePath := strings.TrimPrefix(file.Path, sourcePath)
		relativePath = strings.TrimPrefix(relativePath, "/")
		sourceRelativePaths[relativePath] = struct{}{}

		// Get file content from source
		content, err := provider.GetFileContent(ctx, owner, repo, file.Path, ref)
		if err != nil {
			prLogger.Errorf("Failed to get file content for %s: %v", file.Path, err)
			return nil, err
		}

		targetFilePath := strings.TrimSuffix(targetPath, "/") + "/" + relativePath

		// Base64 encode the content for GitLab
		encodedContent := base64.StdEncoding.EncodeToString(content)

		// GitLab requires "create" for new files and "update" for existing ones
		action := "create"
		if _, exists := targetRelativePaths[relativePath]; exists {
			action = "update"
		}

		actions = append(actions, &gitprovider.CommitAction{
			Action:   action,
			FilePath: targetFilePath,
			Content:  encodedContent,
			Encoding: "base64",
		})
	}

	// Delete files in target that are not in source
	for _, file := range targetFiles {
		relativePath := strings.TrimPrefix(file.Path, targetPath)
		relativePath = strings.TrimPrefix(relativePath, "/")
		if _, exists := sourceRelativePaths[relativePath]; !exists {
			prLogger.Debugf("%s not found in source %s, marking for deletion", relativePath, sourcePath)
			actions = append(actions, &gitprovider.CommitAction{
				Action:   "delete",
				FilePath: file.Path,
			})
		}
	}

	return actions, nil
}

// generateDeleteActions creates delete CommitActions for all files under a path
func generateDeleteActions(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo, path, ref string,
	prLogger *log.Entry,
) ([]*gitprovider.CommitAction, error) {
	files, err := listFilesRecursive(ctx, provider, owner, repo, path, ref)
	if err != nil {
		prLogger.Infof("Target directory %s also not found, nothing to delete", path)
		return nil, nil
	}

	var actions []*gitprovider.CommitAction
	for _, file := range files {
		actions = append(actions, &gitprovider.CommitAction{
			Action:   "delete",
			FilePath: file.Path,
		})
	}
	return actions, nil
}

// listFilesRecursive lists all files (not dirs) recursively under a path using GetDirectoryContent
func listFilesRecursive(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo, path, ref string,
) ([]*gitprovider.FileNode, error) {
	entries, err := provider.GetDirectoryContent(ctx, owner, repo, path, ref)
	if err != nil {
		return nil, err
	}

	var files []*gitprovider.FileNode
	for _, entry := range entries {
		if entry.Type == "file" {
			files = append(files, entry)
		} else if entry.Type == "dir" {
			subFiles, err := listFilesRecursive(ctx, provider, owner, repo, entry.Path, ref)
			if err != nil {
				return nil, err
			}
			files = append(files, subFiles...)
		}
	}
	return files, nil
}

// executePromotion creates a branch, commit, MR and optionally approves/merges for a single promotion.
// existingMetadata is parsed from the triggering MR body (nil for first-hop or push-triggered promotions).
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
	// Collect all commit actions for this promotion
	var allActions []*gitprovider.CommitAction
	for target, source := range promotion.ComputedSyncPaths {
		actions, err := generateSyncCommitActions(ctx, provider, owner, repo, source, target, defaultBranch, prLogger)
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

	// Get HEAD SHA of default branch
	ref, err := provider.GetRef(ctx, owner, repo, "refs/heads/"+defaultBranch)
	if err != nil {
		prLogger.Errorf("Failed to get default branch ref: %v", err)
		return err
	}

	// Create promotion branch (delete stale branch first if it exists)
	newBranchName := generateSafePromotionBranchName(prNumber, prBranch, promotion.Metadata.TargetPaths)
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

	// Create commit on the new branch
	commitMsg := fmt.Sprintf("Syncing from %s", promotion.Metadata.SourcePath)
	_, err = provider.CreateCommit(ctx, owner, repo, &gitprovider.CommitOptions{
		Message:       commitMsg,
		Branch:        newBranchName,
		CommitActions: allActions,
	})
	if err != nil {
		prLogger.Errorf("Failed to create commit: %v", err)
		// Clean up branch on failure
		_ = provider.DeleteBranch(ctx, owner, repo, newBranchName)
		return err
	}

	// Determine the original author for chained promotions
	originalPrAuthor := prAuthor
	if existingMetadata != nil && existingMetadata.OriginalPrAuthor != "" {
		originalPrAuthor = existingMetadata.OriginalPrAuthor
	}

	// Create MR
	components := strings.Join(promotion.Metadata.ComponentNames, ",")
	mrTitle := fmt.Sprintf("🚀 Promotion: %s ➡️  %s", components, promotion.Metadata.TargetDescription)
	mrBody := generatePromotionPrBody(prNumber, components, promotion, originalPrAuthor, repoURL, existingMetadata)

	newPR := &gitprovider.NewPullRequest{
		Title:     mrTitle,
		Body:      mrBody,
		Head:      newBranchName,
		Base:      defaultBranch,
		Labels:    config.PromtionPrLables,
		Assignees: []string{originalPrAuthor},
	}

	createdMR, err := provider.CreatePullRequest(ctx, owner, repo, newPR)
	if err != nil {
		prLogger.Errorf("Failed to create promotion MR: %v", err)
		return err
	}

	prLogger.Infof("Created promotion MR #%d: %s", createdMR.Number, createdMR.HTMLURL)

	// Auto-approve if configured
	if config.AutoApprovePromotionPrs && approverProvider != nil {
		_, err = approverProvider.ApprovePullRequest(ctx, owner, repo, createdMR.Number)
		if err != nil {
			prLogger.Errorf("Failed to auto-approve promotion MR #%d: %v", createdMR.Number, err)
		} else {
			prLogger.Infof("Auto-approved promotion MR #%d", createdMR.Number)
		}
	}

	// Auto-merge if configured
	if promotion.Metadata.AutoMerge {
		// Comment before merging for visibility
		_, _ = provider.CommentOnPullRequest(ctx, owner, repo, createdMR.Number,
			fmt.Sprintf("Auto-merging promotion MR !%d as configured.", createdMR.Number))

		prLogger.Infof("Auto-merging promotion MR #%d", createdMR.Number)
		err = mergePrWithRetry(ctx, provider, owner, repo, createdMR.Number, components, promotion.Metadata.TargetDescription, prLogger)
		if err != nil {
			prLogger.Errorf("Failed to auto-merge promotion MR #%d: %v", createdMR.Number, err)
			_, _ = provider.CommentOnPullRequest(ctx, owner, repo, createdMR.Number,
				fmt.Sprintf("Auto-merge failed: %v\nPlease merge manually.", err))
		}
	}

	return nil
}

// mergePrWithRetry merges a PR with exponential backoff retry on transient errors
func mergePrWithRetry(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo string,
	mrNumber int,
	components, targetDescription string,
	prLogger *log.Entry,
) error {
	operation := func() error {
		err := provider.MergePullRequest(ctx, owner, repo, mrNumber, &gitprovider.MergeOptions{
			CommitMessage: fmt.Sprintf("Auto-merge promotion: %s -> %s", components, targetDescription),
			MergeMethod:   gitprovider.MergeMethodMerge,
		})
		if err != nil {
			errMsg := err.Error()
			// Retry on transient merge errors (not yet mergeable, pipeline pending, etc.)
			if isMergeErrorRetryable(errMsg) {
				prLogger.Warnf("Transient merge error for MR #%d, will retry: %v", mrNumber, err)
				return err
			}
			// Permanent error — stop retrying
			prLogger.Errorf("Permanent merge error for MR #%d: %v", mrNumber, err)
			return backoff.Permanent(err)
		}
		return nil
	}

	return backoff.Retry(operation, backoff.NewExponentialBackOff())
}

// isMergeErrorRetryable checks if a merge error is transient and worth retrying
func isMergeErrorRetryable(errMessage string) bool {
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

// generateSafePromotionBranchName creates a unique branch name based on PR number, branch and targets.
// Max length of branch name is 250 characters.
func generateSafePromotionBranchName(prNumber int, originalBranchName string, targetPaths []string) string {
	targetPathsBa := []byte(strings.Join(targetPaths, "_"))
	hasher := sha1.New() //nolint:gosec // G505: not a cryptographic use case
	hasher.Write(targetPathsBa)
	uniqBranchNameSuffix := firstN(hex.EncodeToString(hasher.Sum(nil)), 12)
	safeOriginalBranchName := firstN(strings.ReplaceAll(originalBranchName, "/", "-"), 200)
	return fmt.Sprintf("promotions/%v-%v-%v", prNumber, safeOriginalBranchName, uniqBranchNameSuffix)
}

func firstN(str string, n int) string {
	v := []rune(str)
	if n >= len(v) {
		return str
	}
	return string(v[:n])
}

// generatePromotionPrBody creates the body text for a promotion MR, including serialized metadata
// for chained promotions. existingMetadata is the metadata from the triggering MR (nil for first hop).
func generatePromotionPrBody(prNumber int, components string, promotion PromotionInstance, originalAuthor, repoURL string, existingMetadata *prMetadata) string {
	// Build the new metadata chain
	newMetadata := prMetadata{
		OriginalPrAuthor: originalAuthor,
	}

	// Carry forward previous promotion metadata chain
	if existingMetadata != nil && existingMetadata.PreviousPromotionMetadata != nil {
		newMetadata.PreviousPromotionMetadata = existingMetadata.PreviousPromotionMetadata
	} else {
		newMetadata.PreviousPromotionMetadata = make(map[int]promotionInstanceMetaDataSe)
	}

	// Add current promotion to the chain
	newMetadata.PreviousPromotionMetadata[prNumber] = promotionInstanceMetaDataSe{
		SourcePath:  promotion.Metadata.SourcePath,
		TargetPaths: promotion.Metadata.TargetPaths,
	}

	// Store promoted paths for potential re-triggering
	promotedPaths := make([]string, 0, len(promotion.ComputedSyncPaths))
	for k := range promotion.ComputedSyncPaths {
		promotedPaths = append(promotedPaths, k)
	}
	newMetadata.PromotedPaths = promotedPaths

	// Build the visible body
	var body strings.Builder

	body.WriteString(fmt.Sprintf("Promotion path(%s):\n\n", components))

	// Render promotion chain (sorted by PR number)
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
			prRef = fmt.Sprintf("[!%d](%s/-/merge_requests/%d)", k, repoURL, k)
		} else {
			prRef = fmt.Sprintf("!%d", k)
		}
		body.WriteString(fmt.Sprintf("%s↘️  %s  `%s` ➡️ %s`%s`  \n",
			strings.Repeat(indent, i), prRef, meta.SourcePath, strings.Repeat(indent, i), tp))
	}

	if len(promotion.Metadata.PerComponentSkippedTargetPaths) > 0 {
		body.WriteString("\n**Skipped target paths:**\n")
		for comp, paths := range promotion.Metadata.PerComponentSkippedTargetPaths {
			body.WriteString(fmt.Sprintf("- **%s**: %s\n", comp, strings.Join(paths, ", ")))
		}
	}

	// Serialize metadata and embed in hidden HTML comment
	metadataString, _ := newMetadata.serialize()
	body.WriteString("\n<!--|Telefonistka data, do not delete|" + metadataString + "|-->")

	return body.String()
}

// commentPromotionPlan comments the dry-run promotion plan on the MR
func commentPromotionPlan(
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

// Helper functions

func containMatchingRegex(patterns []string, str string) bool {
	for _, pattern := range patterns {
		doesMatch, err := regexp.MatchString(pattern, str)
		if err != nil {
			log.Errorf("failed to match regex %s vs %s: %s", pattern, str, err)
			return false
		}
		if doesMatch {
			return true
		}
	}
	return false
}

// setCommitStatus sets the commit status on the given SHA
func setCommitStatus(ctx context.Context, provider gitprovider.GitProvider, owner, repo, sha, state string, prLogger *log.Entry) {
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

// DetectDrift compares source and target directories for each promotion path
// and returns a comment body describing any drift found.
// This is called when an MR is opened or updated to warn about environment drift.
func DetectDrift(ctx context.Context, details ProviderClientDetails) error {
	details.PrLogger.Debug("Checking for drift")
	if ctx.Err() != nil {
		return ctx.Err()
	}

	defaultBranch, err := details.Provider.GetDefaultBranch(ctx, details.Owner, details.Repo)
	if err != nil {
		return fmt.Errorf("failed to get default branch: %w", err)
	}

	config, err := getRepoConfig(ctx, details.Provider, details.Owner, details.Repo, defaultBranch, details.PrLogger)
	if err != nil {
		_, _ = details.Provider.CommentOnPullRequest(ctx, details.Owner, details.Repo, details.PrNumber,
			fmt.Sprintf("Failed to get configuration\n```\n%s\n```\n", err))
		return err
	}

	// List changed files in the MR to determine which promotions are relevant
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
			hasDiff, diffOutput, err := compareRepoDirectories(ctx, details.Provider, details.Owner, details.Repo, source, target, defaultBranch, details.RepoURL, details.PrLogger)
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
		comment := generateDriftComment(diffOutputMap)
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

// compareRepoDirectories compares two directories on the default branch by
// building flat maps of relative-path→SHA, then fetching content for any differences.
func compareRepoDirectories(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo, sourcePath, targetPath, ref, repoURL string,
	prLogger *log.Entry,
) (bool, string, error) {
	sourceFiles, err := listFilesRecursive(ctx, provider, owner, repo, sourcePath, ref)
	if err != nil {
		// Source dir missing means drift (target has files that source doesn't)
		prLogger.Debugf("Source directory %s not accessible: %v", sourcePath, err)
		return true, fmt.Sprintf("Source directory `%s` not found\n", sourcePath), nil
	}

	targetFiles, err := listFilesRecursive(ctx, provider, owner, repo, targetPath, ref)
	if err != nil {
		// Target dir missing is drift
		prLogger.Debugf("Target directory %s not accessible: %v", targetPath, err)
		return true, fmt.Sprintf("Target directory `%s` not found\n", targetPath), nil
	}

	// Build SHA maps keyed by relative path
	sourceSHAs := make(map[string]string)
	for _, f := range sourceFiles {
		rel := strings.TrimPrefix(f.Path, sourcePath)
		rel = strings.TrimPrefix(rel, "/")
		sourceSHAs[rel] = f.SHA
	}

	targetSHAs := make(map[string]string)
	for _, f := range targetFiles {
		rel := strings.TrimPrefix(f.Path, targetPath)
		rel = strings.TrimPrefix(rel, "/")
		targetSHAs[rel] = f.SHA
	}

	// Quick check: if all SHAs match perfectly, no drift
	if len(sourceSHAs) == len(targetSHAs) {
		allMatch := true
		for k, v := range sourceSHAs {
			if tv, ok := targetSHAs[k]; !ok || tv != v {
				allMatch = false
				break
			}
		}
		if allMatch {
			return false, "", nil
		}
	}

	// Generate diff output by fetching content for files with different SHAs
	var hasDiff bool
	var diffOutput strings.Builder
	var filesWithDiff []string
	diffOutput.WriteString("\n```diff\n")

	// Files in source with different SHA or missing in target
	for filename, sha := range sourceSHAs {
		if targetSHA, found := targetSHAs[filename]; found {
			if sha != targetSHA {
				hasDiff = true
				// Fetch actual content to produce unified diff
				sourceContent, err := provider.GetFileContent(ctx, owner, repo, sourcePath+"/"+filename, ref)
				if err != nil {
					prLogger.Warnf("Failed to get source content for %s: %v", filename, err)
					diffOutput.WriteString(fmt.Sprintf("--- %s/%s\n+++ %s/%s\n(content differs, could not fetch)\n", sourcePath, filename, targetPath, filename))
					continue
				}
				targetContent, err := provider.GetFileContent(ctx, owner, repo, targetPath+"/"+filename, ref)
				if err != nil {
					prLogger.Warnf("Failed to get target content for %s: %v", filename, err)
					diffOutput.WriteString(fmt.Sprintf("--- %s/%s\n+++ %s/%s\n(content differs, could not fetch)\n", sourcePath, filename, targetPath, filename))
					continue
				}
				edits := myers.ComputeEdits(span.URIFromPath(filename), string(sourceContent), string(targetContent))
				diffOutput.WriteString(fmt.Sprint(gotextdiff.ToUnified(sourcePath+"/"+filename, targetPath+"/"+filename, string(sourceContent), edits)))
				filesWithDiff = append(filesWithDiff, sourcePath+"/"+filename)
			}
		} else {
			hasDiff = true
			diffOutput.WriteString(fmt.Sprintf("--- %s/%s (missing from target dir %s)\n", sourcePath, filename, targetPath))
		}
	}

	// Files in target but missing in source
	for filename := range targetSHAs {
		if _, found := sourceSHAs[filename]; !found {
			hasDiff = true
			diffOutput.WriteString(fmt.Sprintf("+++ %s/%s (missing from source dir %s)\n", targetPath, filename, sourcePath))
		}
	}

	diffOutput.WriteString("\n```\n")

	// Add blame links for files with content differences
	if len(filesWithDiff) > 0 && repoURL != "" {
		diffOutput.WriteString("\n### Blame Links:\n")
		for _, f := range filesWithDiff {
			blameURL := fmt.Sprintf("%s/-/blame/HEAD/%s", repoURL, f)
			diffOutput.WriteString(fmt.Sprintf("- [%s](%s)\n", f, blameURL))
		}
	}

	return hasDiff, diffOutput.String(), nil
}

// generateDriftComment builds the drift warning comment body
func generateDriftComment(diffOutputMap map[string]string) string {
	var body strings.Builder
	body.WriteString("# ⚠️  Found drift between environments ⚠️\n\n")
	body.WriteString("## Intro\n")
	body.WriteString("Drift detection runs on the files in the main branch irrespective of the changes of the MR.\n\n")
	body.WriteString("This could happen in two scenarios:\n")
	body.WriteString("1. A promotion that affects these components is still in progress or was cancelled before completion. ")
	body.WriteString("This means that your automated promotion MR will **include these changes** in addition to your changes!\n\n")
	body.WriteString("2. Someone made a change directly to one of the directories representing promotion targets. ")
	body.WriteString("These changes will be **overridden** by the automated promotion MRs unless changes are made to their respective branches.\n\n")
	body.WriteString("## Diffs\n\n")

	for title, diffOutput := range diffOutputMap {
		body.WriteString(title + "\n\n")
		body.WriteString("<details><summary>Diff (Click to expand)</summary>\n\n")
		body.WriteString(diffOutput)
		body.WriteString("\n</details>\n\n")
	}

	return body.String()
}

func containsString(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// HandlePushPromotion handles the promotion workflow triggered by a push to the default branch.
// It uses CompareCommits to determine changed files, then runs the same promotion logic as MR merge.
func HandlePushPromotion(ctx context.Context, details ProviderClientDetails, beforeSHA, afterSHA, defaultBranch string) error {
	prLogger := details.PrLogger

	// Load telefonistka.yaml config
	config, err := getRepoConfig(ctx, details.Provider, details.Owner, details.Repo, defaultBranch, prLogger)
	if err != nil {
		prLogger.Errorf("Failed to get repo config: %v", err)
		return err
	}

	// Compare commits to get changed files
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

	// Generate promotion plan (no labels for push events, no PR number)
	promotions, err := generatePromotionPlan(ctx, details.Provider, details.Owner, details.Repo, changedFiles, nil, config, defaultBranch, prLogger)
	if err != nil {
		prLogger.Errorf("Failed to generate promotion plan: %v", err)
		return err
	}

	if len(promotions) == 0 {
		prLogger.Info("No promotions needed for this push")
		return nil
	}

	// For push events we use a pseudo PR number of 0 and the ref name as branch
	// No existing metadata for push-triggered promotions
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
