package promotion

import (
	"context"
	"regexp"
	"sort"
	"strings"

	cfg "github.com/commercetools/telefonistka/internal/pkg/configuration"
	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	log "github.com/sirupsen/logrus"
)

// GeneratePromotionPlan generates a map of promotions based on changed files and config.
// changedFiles is a list of file paths that changed (from MR files or commit comparison).
func GeneratePromotionPlan(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo string,
	changedFiles []string,
	labels []string,
	config *cfg.Config,
	defaultBranch string,
	prLogger *log.Entry,
) (map[string]PromotionInstance, error) {
	relevantComponents := IdentifyRelevantComponents(changedFiles, config, prLogger)
	return GeneratePlanFromComponents(ctx, provider, owner, repo, config, relevantComponents, labels, defaultBranch, prLogger)
}

// IdentifyRelevantComponents extracts component names from changed files based on config PromotionPaths.
func IdentifyRelevantComponents(changedFiles []string, config *cfg.Config, prLogger *log.Entry) map[RelevantComponent]struct{} {
	relevantComponents := make(map[RelevantComponent]struct{})

	for _, filename := range changedFiles {
		for _, promotionPathConfig := range config.PromotionPaths {
			if match, _ := regexp.MatchString("^"+promotionPathConfig.SourcePath+".*", filename); match {
				componentPathRegexSubStrings := []string{}
				for i := 0; i <= promotionPathConfig.ComponentPathExtraDepth; i++ {
					componentPathRegexSubStrings = append(componentPathRegexSubStrings, "[^/]*")
				}
				componentPathRegexSubString := strings.Join(componentPathRegexSubStrings, "/")
				getComponentRegexString := regexp.MustCompile("^" + promotionPathConfig.SourcePath + "(" + componentPathRegexSubString + ")/.*")
				componentName := getComponentRegexString.ReplaceAllString(filename, "${1}")

				getSourcePathRegexString := regexp.MustCompile("^(" + promotionPathConfig.SourcePath + ")" + componentName + "/.*")
				compiledSourcePath := getSourcePathRegexString.ReplaceAllString(filename, "${1}")

				rc := RelevantComponent{
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

// GeneratePlanFromComponents creates PromotionInstances by matching components against config.
func GeneratePlanFromComponents(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo string,
	config *cfg.Config,
	relevantComponents map[RelevantComponent]struct{},
	labels []string,
	configBranch string,
	prLogger *log.Entry,
) (map[string]PromotionInstance, error) {
	promotions := make(map[string]PromotionInstance)

	for componentToPromote := range relevantComponents {
		componentConfig, err := GetComponentConfig(ctx, provider, owner, repo, componentToPromote.SourcePath+componentToPromote.ComponentName, configBranch, prLogger)
		if err != nil {
			prLogger.Errorf("Failed to get in component configuration, err=%s, skipping %s", err, componentToPromote.SourcePath+componentToPromote.ComponentName)
		}

		for _, configPromotionPath := range config.PromotionPaths {
			if match, _ := regexp.MatchString(configPromotionPath.SourcePath, componentToPromote.SourcePath); match {
				if configPromotionPath.Conditions.PrHasLabels != nil {
					hasRightLabel := false
					for _, l := range labels {
						if ContainsString(configPromotionPath.Conditions.PrHasLabels, l) {
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
								BlockList:                      ppr.BlockList,
							},
							ComputedSyncPaths: map[string]string{},
						}
					} else if !ContainsString(entry.Metadata.ComponentNames, componentToPromote.ComponentName) {
						entry.Metadata.ComponentNames = append(entry.Metadata.ComponentNames, componentToPromote.ComponentName)
						promotions[mapKey] = entry
					}

					for _, individualPath := range ppr.TargetPaths {
						if componentConfig != nil {
							if componentConfig.PromotionTargetBlockList != nil {
								if ContainMatchingRegex(componentConfig.PromotionTargetBlockList, individualPath) {
									promotions[mapKey].Metadata.PerComponentSkippedTargetPaths[componentToPromote.ComponentName] = append(
										promotions[mapKey].Metadata.PerComponentSkippedTargetPaths[componentToPromote.ComponentName], individualPath)
									continue
								}
							}
							if componentConfig.PromotionTargetAllowList != nil {
								if !ContainMatchingRegex(componentConfig.PromotionTargetAllowList, individualPath) {
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
