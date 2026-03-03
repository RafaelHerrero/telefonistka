package promotion

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	log "github.com/sirupsen/logrus"
)

// ListFilesRecursive lists all files (not dirs) recursively under a path using GetDirectoryContent.
func ListFilesRecursive(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo, path, ref string,
) ([]*gitprovider.FileNode, error) {
	entries, err := provider.GetDirectoryContent(ctx, owner, repo, path, ref)
	if err != nil {
		return nil, err
	}

	var files []*gitprovider.FileNode
	for _, entry := range entries {
		if entry.Type == "file" {
			files = append(files, entry)
		} else if entry.Type == "dir" {
			subFiles, err := ListFilesRecursive(ctx, provider, owner, repo, entry.Path, ref)
			if err != nil {
				return nil, err
			}
			files = append(files, subFiles...)
		}
	}
	return files, nil
}

// GenerateSyncCommitActions creates CommitActions to sync files from source to target directory.
// It handles create/update and delete operations.
// blockList contains glob patterns (doublestar syntax) for files that should be skipped during sync.
func GenerateSyncCommitActions(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo, sourcePath, targetPath, ref string,
	blockList []string,
	prLogger *log.Entry,
) ([]*gitprovider.CommitAction, error) {
	sourceFiles, err := ListFilesRecursive(ctx, provider, owner, repo, sourcePath, ref)
	if err != nil {
		prLogger.Infof("Source directory %s not found, assuming deletion PR", sourcePath)
		return GenerateDeleteActions(ctx, provider, owner, repo, targetPath, ref, prLogger)
	}

	targetFiles, _ := ListFilesRecursive(ctx, provider, owner, repo, targetPath, ref)

	targetRelativePaths := make(map[string]struct{})
	for _, file := range targetFiles {
		relativePath := strings.TrimPrefix(file.Path, targetPath)
		relativePath = strings.TrimPrefix(relativePath, "/")
		targetRelativePaths[relativePath] = struct{}{}
	}

	sourceRelativePaths := make(map[string]struct{})
	var actions []*gitprovider.CommitAction

	for _, file := range sourceFiles {
		relativePath := strings.TrimPrefix(file.Path, sourcePath)
		relativePath = strings.TrimPrefix(relativePath, "/")
		sourceRelativePaths[relativePath] = struct{}{}

		if IsFileBlocked(relativePath, blockList) {
			prLogger.Debugf("Skipping blocked file %s (matched blockList pattern)", relativePath)
			continue
		}

		content, err := provider.GetFileContent(ctx, owner, repo, file.Path, ref)
		if err != nil {
			prLogger.Errorf("Failed to get file content for %s: %v", file.Path, err)
			return nil, err
		}

		targetFilePath := strings.TrimSuffix(targetPath, "/") + "/" + relativePath
		encodedContent := base64.StdEncoding.EncodeToString(content)

		action := "create"
		if _, exists := targetRelativePaths[relativePath]; exists {
			action = "update"
		}

		actions = append(actions, &gitprovider.CommitAction{
			Action:   action,
			FilePath: targetFilePath,
			Content:  encodedContent,
			Encoding: "base64",
		})
	}

	for _, file := range targetFiles {
		relativePath := strings.TrimPrefix(file.Path, targetPath)
		relativePath = strings.TrimPrefix(relativePath, "/")
		if _, exists := sourceRelativePaths[relativePath]; !exists {
			if IsFileBlocked(relativePath, blockList) {
				prLogger.Debugf("Skipping deletion of blocked file %s (matched blockList pattern)", relativePath)
				continue
			}
			prLogger.Debugf("%s not found in source %s, marking for deletion", relativePath, sourcePath)
			actions = append(actions, &gitprovider.CommitAction{
				Action:   "delete",
				FilePath: file.Path,
			})
		}
	}

	return actions, nil
}

// GenerateDeleteActions creates delete CommitActions for all files under a path.
func GenerateDeleteActions(
	ctx context.Context,
	provider gitprovider.GitProvider,
	owner, repo, path, ref string,
	prLogger *log.Entry,
) ([]*gitprovider.CommitAction, error) {
	files, err := ListFilesRecursive(ctx, provider, owner, repo, path, ref)
	if err != nil {
		prLogger.Infof("Target directory %s also not found, nothing to delete", path)
		return nil, nil
	}

	var actions []*gitprovider.CommitAction
	for _, file := range files {
		actions = append(actions, &gitprovider.CommitAction{
			Action:   "delete",
			FilePath: file.Path,
		})
	}
	return actions, nil
}
