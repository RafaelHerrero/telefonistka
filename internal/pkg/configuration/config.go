package configuration

import (
	yaml "gopkg.in/yaml.v2"
)

type WebhookEndpointRegex struct {
	Expression   string   `yaml:"expression"`
	Replacements []string `yaml:"replacements"`
}

type ComponentConfig struct {
	PromotionTargetAllowList []string `yaml:"promotionTargetAllowList"`
	PromotionTargetBlockList []string `yaml:"promotionTargetBlockList"`
	DisableArgoCDDiff        bool     `yaml:"disableArgoCDDiff"`
}

type Condition struct {
	PrHasLabels []string `yaml:"prHasLabels"`
	AutoMerge   bool     `yaml:"autoMerge"`
}

type PromotionPr struct {
	TargetDescription string   `yaml:"targetDescription"`
	TargetPaths       []string `yaml:"targetPaths"`
	BlockList         []string `yaml:"blockList"`
}

type PromotionPath struct {
	Conditions              Condition     `yaml:"conditions"`
	ComponentPathExtraDepth int           `yaml:"componentPathExtraDepth"`
	SourcePath              string        `yaml:"sourcePath"`
	PromotionPrs            []PromotionPr `yaml:"promotionPrs"`
}

type Config struct {
	// What paths trigger promotion to which paths
	PromotionPaths []PromotionPath `yaml:"promotionPaths"`

	// Generic configuration
	PromtionPrLables             []string               `yaml:"promtionPRlables"`
	DryRunMode                   bool                   `yaml:"dryRunMode"`
	AutoApprovePromotionPrs      bool                   `yaml:"autoApprovePromotionPrs"`
	ToggleCommitStatus           map[string]string      `yaml:"toggleCommitStatus"`
	WebhookEndpointRegexs        []WebhookEndpointRegex `yaml:"webhookEndpointRegexs"`
	WhProxtSkipTLSVerifyUpstream bool                   `yaml:"whProxtSkipTLSVerifyUpstream"`
	Argocd                       ArgocdConfig           `yaml:"argocd"`

	// Git provider configuration
	GitProvider ProviderConfig `yaml:"gitProvider"`
}

type ArgocdConfig struct {
	CommentDiffonPR               bool   `yaml:"commentDiffonPR"`
	AutoMergeNoDiffPRs            bool   `yaml:"autoMergeNoDiffPRs"`
	AllowSyncfromBranchPathRegex  string `yaml:"allowSyncfromBranchPathRegex"`
	UseSHALabelForAppDiscovery    bool   `yaml:"useSHALabelForAppDiscovery"`
	CreateTempAppObjectFroNewApps bool   `yaml:"createTempAppObjectFromNewApps"`
}

// ProviderConfig contains Git provider configuration
type ProviderConfig struct {
	Type   string            `yaml:"type"`   // "github" or "gitlab"
	URL    string            `yaml:"url"`    // For self-hosted instances
	Config map[string]string `yaml:"config"` // Provider-specific config
}

func ParseConfigFromYaml(y string) (*Config, error) {
	config := &Config{}

	err := yaml.Unmarshal([]byte(y), config)

	return config, err
}
