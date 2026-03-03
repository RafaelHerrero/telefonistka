package promotion

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
)

// PrMetadata is serialized into the PR/MR body to enable chained promotions.
// When a promotion PR/MR is itself merged, this metadata is parsed to carry
// the original author and promotion history forward.
type PrMetadata struct {
	OriginalPrAuthor          string                        `json:"originalPrAuthor"`
	OriginalPrNumber          int                           `json:"originalPrNumber"`
	PromotedPaths             []string                      `json:"promotedPaths"`
	PreviousPromotionMetadata map[int]PromotionPathMetadata `json:"previousPromotionPaths"`
}

// PromotionPathMetadata is the serializable subset of PromotionInstanceMetaData
type PromotionPathMetadata struct {
	SourcePath  string   `json:"sourcePath"`
	TargetPaths []string `json:"targetPaths"`
}

// Serialize encodes the metadata to a base64 JSON string.
func (pm PrMetadata) Serialize() (string, error) {
	pmJSON, err := json.Marshal(pm)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pmJSON), nil
}

// DeSerialize decodes a base64 JSON string into the metadata.
func (pm *PrMetadata) DeSerialize(s string) error {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, pm)
}

// ParsePrMetadata extracts serialized metadata from a PR/MR body.
func ParsePrMetadata(body string) *PrMetadata {
	metadataRegex := regexp.MustCompile(`<!--\|.*\|(.*)\|-->`)
	matches := metadataRegex.FindStringSubmatch(body)
	if len(matches) == 2 && matches[1] != "" {
		pm := &PrMetadata{}
		if err := pm.DeSerialize(matches[1]); err != nil {
			return nil
		}
		return pm
	}
	return nil
}
