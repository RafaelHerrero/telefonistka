package gitlab

import (
	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
)

// Register the GitLab provider with the factory
func init() {
	gitprovider.RegisterProvider(gitprovider.ProviderTypeGitLab, func(config *gitprovider.ProviderConfig) (gitprovider.GitProvider, error) {
		return NewGitLabProvider(config)
	})
}
