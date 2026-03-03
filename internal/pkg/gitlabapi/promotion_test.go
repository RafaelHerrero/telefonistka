package gitlabapi

import (
	"testing"

	promlib "github.com/commercetools/telefonistka/internal/pkg/promotion"
)

func TestIsFileBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		relativePath string
		blockList    []string
		expected     bool
	}{
		{
			name:         "matches doublestar pattern for application.yaml",
			relativePath: "ingress/application.yaml",
			blockList:    []string{"**/application.yaml"},
			expected:     true,
		},
		{
			name:         "matches doublestar pattern for nested path",
			relativePath: "deep/nested/dir/application.yaml",
			blockList:    []string{"**/application.yaml"},
			expected:     true,
		},
		{
			name:         "matches top-level file",
			relativePath: "application.yaml",
			blockList:    []string{"**/application.yaml"},
			expected:     true,
		},
		{
			name:         "matches values-env.yaml pattern",
			relativePath: "manifests/values-env.yaml",
			blockList:    []string{"**/values-env.yaml"},
			expected:     true,
		},
		{
			name:         "does not match unrelated file",
			relativePath: "manifests/values.yaml",
			blockList:    []string{"**/application.yaml", "**/values-env.yaml"},
			expected:     false,
		},
		{
			name:         "empty blockList matches nothing",
			relativePath: "application.yaml",
			blockList:    []string{},
			expected:     false,
		},
		{
			name:         "nil blockList matches nothing",
			relativePath: "application.yaml",
			blockList:    nil,
			expected:     false,
		},
		{
			name:         "matches simple glob pattern",
			relativePath: "manifests/secret.yaml",
			blockList:    []string{"manifests/secret.yaml"},
			expected:     true,
		},
		{
			name:         "matches wildcard in directory",
			relativePath: "manifests/values-env.yaml",
			blockList:    []string{"manifests/*-env.yaml"},
			expected:     true,
		},
		{
			name:         "multiple patterns - second matches",
			relativePath: "manifests/values-env.yaml",
			blockList:    []string{"**/application.yaml", "**/values-env.yaml"},
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := promlib.IsFileBlocked(tt.relativePath, tt.blockList)
			if result != tt.expected {
				t.Errorf("promlib.IsFileBlocked(%q, %v) = %v, want %v", tt.relativePath, tt.blockList, result, tt.expected)
			}
		})
	}
}
