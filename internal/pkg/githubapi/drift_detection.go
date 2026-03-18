package githubapi

import (
	promlib "github.com/commercetools/telefonistka/internal/pkg/promotion"
)

func CompareRepoDirectories(ghPrClientDetails GhPrClientDetails, sourcePath string, targetPath string, defaultBranch string, blockList []string) (bool, string, error) {
	provider := ghPrClientDetails.toProvider()
	blamePrefix := ghPrClientDetails.getBlameURLPrefix() + "/HEAD"
	return promlib.CompareRepoDirectories(ghPrClientDetails.Ctx, provider,
		ghPrClientDetails.Owner, ghPrClientDetails.Repo, sourcePath, targetPath,
		defaultBranch, blamePrefix, blockList, ghPrClientDetails.PrLogger)
}
