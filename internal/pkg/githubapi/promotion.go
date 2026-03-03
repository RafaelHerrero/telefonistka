package githubapi

import (
	"fmt"

	cfg "github.com/commercetools/telefonistka/internal/pkg/configuration"
	promlib "github.com/commercetools/telefonistka/internal/pkg/promotion"
)

type PromotionInstance = promlib.PromotionInstance
type PromotionInstanceMetaData = promlib.PromotionInstanceMetaData


func DetectDrift(ghPrClientDetails GhPrClientDetails) error {
	ghPrClientDetails.PrLogger.Debugln("Checking for Drift")
	if ghPrClientDetails.Ctx.Err() != nil {
		return ghPrClientDetails.Ctx.Err()
	}
	diffOutputMap := make(map[string]string)
	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()
	config, err := GetInRepoConfig(ghPrClientDetails, defaultBranch)
	if err != nil {
		_ = ghPrClientDetails.CommentOnPr(fmt.Sprintf("Failed to get configuration\n```\n%s\n```\n", err))
		return err
	}

	promotions, _ := GeneratePromotionPlan(ghPrClientDetails, config, ghPrClientDetails.Ref)

	for _, promotion := range promotions {
		ghPrClientDetails.PrLogger.Debugf("Checking drift for %s", promotion.Metadata.SourcePath)
		for trgt, src := range promotion.ComputedSyncPaths {
			hasDiff, diffOutput, _ := CompareRepoDirectories(ghPrClientDetails, src, trgt, defaultBranch)
			if hasDiff {
				mapKey := fmt.Sprintf("`%s` ↔️  `%s`", src, trgt)
				diffOutputMap[mapKey] = diffOutput
				ghPrClientDetails.PrLogger.Debugf("Found diff @ %s", mapKey)
			}
		}
	}
	if len(diffOutputMap) != 0 {
		templateOutput, err := executeTemplate("driftMsg", defaultTemplatesFullPath("drift-pr-comment.gotmpl"), diffOutputMap)
		if err != nil {
			return err
		}

		err = commentPR(ghPrClientDetails, templateOutput)
		if err != nil {
			return err
		}
	} else {
		ghPrClientDetails.PrLogger.Infof("No drift found")
	}

	return nil
}

func getComponentConfig(ghPrClientDetails GhPrClientDetails, componentPath string, branch string) (*cfg.ComponentConfig, error) {
	return promlib.GetComponentConfig(ghPrClientDetails.Ctx, ghPrClientDetails.toProvider(),
		ghPrClientDetails.Owner, ghPrClientDetails.Repo, componentPath, branch, ghPrClientDetails.PrLogger)
}

// generateListOfRelevantComponents lists PR files via the provider and delegates
// component identification to the shared promotion package.
func generateListOfRelevantComponents(ghPrClientDetails GhPrClientDetails, config *cfg.Config) (relevantComponents map[relevantComponent]struct{}, err error) {
	provider := ghPrClientDetails.toProvider()
	prFiles, err := provider.ListPullRequestFiles(ghPrClientDetails.Ctx,
		ghPrClientDetails.Owner, ghPrClientDetails.Repo, ghPrClientDetails.PrNumber)
	if err != nil {
		return nil, err
	}
	changedFiles := make([]string, 0, len(prFiles))
	for _, f := range prFiles {
		changedFiles = append(changedFiles, f.Filename)
	}
	return promlib.IdentifyRelevantComponents(changedFiles, config, ghPrClientDetails.PrLogger), nil
}

type relevantComponent = promlib.RelevantComponent

func generateListOfChangedComponentPaths(ghPrClientDetails GhPrClientDetails, config *cfg.Config) (changedComponentPaths []string, err error) {
	// If the PR has a list of promoted paths in the PR Telefonistika metadata(=is a promotion PR), we use that
	if len(ghPrClientDetails.PrMetadata.PromotedPaths) > 0 {
		changedComponentPaths = ghPrClientDetails.PrMetadata.PromotedPaths
		return changedComponentPaths, nil
	}

	// If not we will use in-repo config to generate it, and turns the map with struct keys into a list of strings
	relevantComponents, err := generateListOfRelevantComponents(ghPrClientDetails, config)
	if err != nil {
		return nil, err
	}
	for component := range relevantComponents {
		changedComponentPaths = append(changedComponentPaths, component.SourcePath+component.ComponentName)
	}
	return changedComponentPaths, nil
}

func GeneratePromotionPlan(ghPrClientDetails GhPrClientDetails, config *cfg.Config, configBranch string) (map[string]PromotionInstance, error) {
	provider := ghPrClientDetails.toProvider()

	prFiles, err := provider.ListPullRequestFiles(ghPrClientDetails.Ctx,
		ghPrClientDetails.Owner, ghPrClientDetails.Repo, ghPrClientDetails.PrNumber)
	if err != nil {
		return nil, err
	}
	changedFiles := make([]string, 0, len(prFiles))
	for _, f := range prFiles {
		changedFiles = append(changedFiles, f.Filename)
	}

	labels := make([]string, 0, len(ghPrClientDetails.Labels))
	for _, l := range ghPrClientDetails.Labels {
		labels = append(labels, *l.Name)
	}

	return promlib.GeneratePromotionPlan(ghPrClientDetails.Ctx, provider,
		ghPrClientDetails.Owner, ghPrClientDetails.Repo, changedFiles, labels,
		config, configBranch, ghPrClientDetails.PrLogger)
}
