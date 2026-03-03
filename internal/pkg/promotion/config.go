package promotion

import (
	"context"

	cfg "github.com/commercetools/telefonistka/internal/pkg/configuration"
	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// GetRepoConfig loads the telefonistka.yaml configuration from the repo root.
func GetRepoConfig(ctx context.Context, provider gitprovider.GitProvider, owner, repo, ref string, prLogger *log.Entry) (*cfg.Config, error) {
	content, err := provider.GetFileContent(ctx, owner, repo, "telefonistka.yaml", ref)
	if err != nil {
		prLogger.Errorf("Could not get in-repo configuration: %v", err)
		return nil, err
	}

	config, err := cfg.ParseConfigFromYaml(string(content))
	if err != nil {
		prLogger.Errorf("Failed to parse configuration: %v", err)
		return nil, err
	}
	return config, nil
}

// GetComponentConfig loads an optional per-component telefonistka.yaml.
func GetComponentConfig(ctx context.Context, provider gitprovider.GitProvider, owner, repo, componentPath, branch string, prLogger *log.Entry) (*cfg.ComponentConfig, error) {
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
