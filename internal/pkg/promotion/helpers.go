package promotion

import (
	"crypto/sha1" //nolint:gosec // G505: not a cryptographic use case
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	log "github.com/sirupsen/logrus"
)

// ContainsString checks if a string slice contains a given string.
func ContainsString(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// ContainMatchingRegex checks if any regex pattern in the slice matches the given string.
func ContainMatchingRegex(patterns []string, str string) bool {
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

// IsFileBlocked checks if a relative file path matches any of the blockList glob patterns.
// Patterns use doublestar syntax (e.g. "**/application.yaml", "manifests/*.yaml").
func IsFileBlocked(relativePath string, blockList []string) bool {
	for _, pattern := range blockList {
		matched, err := doublestar.PathMatch(pattern, relativePath)
		if err != nil {
			log.Errorf("Invalid blockList glob pattern %q: %v", pattern, err)
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// FirstN returns the first n runes of a string.
func FirstN(str string, n int) string {
	v := []rune(str)
	if n >= len(v) {
		return str
	}
	return string(v[:n])
}

// GenerateSafePromotionBranchName creates a unique branch name based on PR number, branch and targets.
// Max length of branch name is 250 characters.
func GenerateSafePromotionBranchName(prNumber int, originalBranchName string, targetPaths []string) string {
	targetPathsBa := []byte(strings.Join(targetPaths, "_"))
	hasher := sha1.New() //nolint:gosec // G505: not a cryptographic use case
	hasher.Write(targetPathsBa)
	uniqBranchNameSuffix := FirstN(hex.EncodeToString(hasher.Sum(nil)), 12)
	safeOriginalBranchName := FirstN(strings.ReplaceAll(originalBranchName, "/", "-"), 200)
	return fmt.Sprintf("promotions/%v-%v-%v", prNumber, safeOriginalBranchName, uniqBranchNameSuffix)
}
