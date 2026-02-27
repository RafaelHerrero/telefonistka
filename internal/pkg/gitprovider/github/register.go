package github

import (
	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
)

// Register the GitHub provider with the factory
func init() {
	gitprovider.RegisterProvider(gitprovider.ProviderTypeGitHub, func(config *gitprovider.ProviderConfig) (gitprovider.GitProvider, error) {
		return NewGitHubProvider(config)
	})
}
