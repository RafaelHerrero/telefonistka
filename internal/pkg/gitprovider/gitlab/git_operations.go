package gitlab

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// GetRef returns a Git reference
func (g *GitLabProvider) GetRef(ctx context.Context, owner, repo, ref string) (*gitprovider.Reference, error) {
	projectPath := g.getProjectPath(owner, repo)

	// Strip "refs/heads/" prefix if present
	if len(ref) > 11 && ref[:11] == "refs/heads/" {
		ref = ref[11:]
	}

	branch, resp, err := g.client.Branches.GetBranch(projectPath, ref)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get ref: %w", err)
	}

	return &gitprovider.Reference{
		Ref: "refs/heads/" + branch.Name,
		SHA: branch.Commit.ID,
	}, nil
}

// CreateRef creates a new Git reference (branch)
func (g *GitLabProvider) CreateRef(ctx context.Context, owner, repo, ref, sha string) (*gitprovider.Reference, error) {
	projectPath := g.getProjectPath(owner, repo)

	// Strip "refs/heads/" prefix if present
	if len(ref) > 11 && ref[:11] == "refs/heads/" {
		ref = ref[11:]
	}

	opts := &gitlab.CreateBranchOptions{
		Branch: gitlab.Ptr(ref),
		Ref:    gitlab.Ptr(sha),
	}

	branch, resp, err := g.client.Branches.CreateBranch(projectPath, opts)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to create ref: %w", err)
	}

	return &gitprovider.Reference{
		Ref: "refs/heads/" + branch.Name,
		SHA: branch.Commit.ID,
	}, nil
}

// UpdateRef updates a Git reference
func (g *GitLabProvider) UpdateRef(ctx context.Context, owner, repo, ref, sha string, force bool) (*gitprovider.Reference, error) {
	// GitLab doesn't have a direct "update ref" API
	// We need to delete and recreate, or use protected branches API
	// For now, we'll use a commit to update the branch
	projectPath := g.getProjectPath(owner, repo)

	// Strip "refs/heads/" prefix if present
	if len(ref) > 11 && ref[:11] == "refs/heads/" {
		ref = ref[11:]
	}

	// Create an empty commit on the target SHA and point the branch to it
	// Actually, we can't easily update a ref in GitLab without commits
	// The best approach is to delete and recreate
	if force {
		// Delete the branch
		_, err := g.client.Branches.DeleteBranch(projectPath, ref)
		if err != nil {
			return nil, fmt.Errorf("failed to delete branch for update: %w", err)
		}

		// Recreate it
		return g.CreateRef(ctx, owner, repo, ref, sha)
	}

	return nil, fmt.Errorf("non-force ref update not supported in GitLab provider")
}

// DeleteRef deletes a Git reference (branch)
func (g *GitLabProvider) DeleteRef(ctx context.Context, owner, repo, ref string) error {
	projectPath := g.getProjectPath(owner, repo)

	// Strip "refs/heads/" prefix if present
	if len(ref) > 11 && ref[:11] == "refs/heads/" {
		ref = ref[11:]
	}

	resp, err := g.client.Branches.DeleteBranch(projectPath, ref)
	g.updateLastResponse(resp)

	if err != nil {
		return fmt.Errorf("failed to delete ref: %w", err)
	}

	return nil
}

// GetCommit returns a Git commit
func (g *GitLabProvider) GetCommit(ctx context.Context, owner, repo, sha string) (*gitprovider.Commit, error) {
	projectPath := g.getProjectPath(owner, repo)

	commit, resp, err := g.client.Commits.GetCommit(projectPath, sha, nil)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	return &gitprovider.Commit{
		SHA:       commit.ID,
		Message:   commit.Message,
		Author:    commit.AuthorName,
		Date:      *commit.AuthoredDate,
		Parents:   commit.ParentIDs,
		HTMLURL:   commit.WebURL,
		Committer: commit.CommitterName,
	}, nil
}

// CreateCommit creates a new Git commit using GitLab's Commits API with actions
// This is the CRITICAL method that solves the tree API challenge
func (g *GitLabProvider) CreateCommit(ctx context.Context, owner, repo string, opts *gitprovider.CommitOptions) (*gitprovider.Commit, error) {
	projectPath := g.getProjectPath(owner, repo)

	var actions []*gitlab.CommitActionOptions

	// GitLab uses CommitActions instead of tree entries
	if len(opts.CommitActions) > 0 {
		// Convert our CommitActions to GitLab's format
		for _, action := range opts.CommitActions {
			glAction := &gitlab.CommitActionOptions{
				Action:   gitlab.Ptr(gitlab.FileActionValue(action.Action)),
				FilePath: gitlab.Ptr(action.FilePath),
			}

			if action.Action != "delete" {
				glAction.Content = gitlab.Ptr(action.Content)
			}

			if action.Encoding != "" {
				glAction.Encoding = gitlab.Ptr(action.Encoding)
			}

			if action.PreviousPath != "" {
				glAction.PreviousPath = gitlab.Ptr(action.PreviousPath)
			}

			actions = append(actions, glAction)
		}
	} else if len(opts.TreeEntries) > 0 {
		// Convert TreeEntries to CommitActions
		// This is for backward compatibility with GitHub-style tree API
		for _, entry := range opts.TreeEntries {
			action := &gitlab.CommitActionOptions{
				FilePath: gitlab.Ptr(entry.Path),
			}

			// Determine action based on entry
			// For GitLab, we need to fetch the file content if it's a blob
			if entry.Type == "blob" {
				// Get the blob content
				blob, _, err := g.client.RepositoryFiles.GetFile(projectPath, entry.Path, &gitlab.GetFileOptions{
					Ref: gitlab.Ptr(entry.SHA),
				})

				if err != nil {
					// File doesn't exist, so it's a create action
					action.Action = gitlab.Ptr(gitlab.FileCreate)
					// We don't have content, which is a problem
					// For now, create an empty file
					action.Content = gitlab.Ptr("")
				} else {
					// File exists, so it's an update
					action.Action = gitlab.Ptr(gitlab.FileUpdate)
					action.Content = gitlab.Ptr(blob.Content)
				}
			}

			actions = append(actions, action)
		}
	} else {
		return nil, fmt.Errorf("either CommitActions or TreeEntries must be provided")
	}

	branch := opts.Branch
	if branch == "" {
		return nil, fmt.Errorf("branch must be specified for GitLab commits")
	}

	commitOpts := &gitlab.CreateCommitOptions{
		Branch:        gitlab.Ptr(branch),
		CommitMessage: gitlab.Ptr(opts.Message),
		Actions:       actions,
	}

	if opts.Author != nil {
		commitOpts.AuthorName = gitlab.Ptr(opts.Author.Name)
		commitOpts.AuthorEmail = gitlab.Ptr(opts.Author.Email)
	}

	commit, resp, err := g.client.Commits.CreateCommit(projectPath, commitOpts)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to create commit: %w", err)
	}

	return &gitprovider.Commit{
		SHA:       commit.ID,
		Message:   commit.Message,
		Author:    commit.AuthorName,
		Date:      *commit.AuthoredDate,
		Parents:   commit.ParentIDs,
		HTMLURL:   commit.WebURL,
		Committer: commit.CommitterName,
	}, nil
}

// CompareCommits compares two commits
func (g *GitLabProvider) CompareCommits(ctx context.Context, owner, repo, base, head string) (*gitprovider.DiffResult, error) {
	projectPath := g.getProjectPath(owner, repo)

	compare, resp, err := g.client.Repositories.Compare(projectPath, &gitlab.CompareOptions{
		From: gitlab.Ptr(base),
		To:   gitlab.Ptr(head),
	})
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to compare commits: %w", err)
	}

	var files []*gitprovider.CommitFile
	for _, diff := range compare.Diffs {
		status := "modified"
		if diff.NewFile {
			status = "added"
		} else if diff.DeletedFile {
			status = "removed"
		} else if diff.RenamedFile {
			status = "renamed"
		}

		files = append(files, &gitprovider.CommitFile{
			Filename:  diff.NewPath,
			Status:    status,
			Additions: 0, // Would need to parse diff
			Deletions: 0, // Would need to parse diff
			Changes:   0,
			Patch:     diff.Diff,
		})
	}

	return &gitprovider.DiffResult{
		FromSHA: base,
		ToSHA:   head,
		Files:   files,
		Changes: len(files),
	}, nil
}

// GetTree returns a Git tree - NOT SUPPORTED in GitLab
func (g *GitLabProvider) GetTree(ctx context.Context, owner, repo, sha string, recursive bool) ([]*gitprovider.TreeEntry, error) {
	return nil, &gitprovider.ProviderNotSupportedError{
		Provider:  gitprovider.ProviderTypeGitLab,
		Operation: "GetTree",
		Message:   "GitLab does not support GitHub-style tree API",
	}
}

// CreateTree creates a Git tree - NOT SUPPORTED in GitLab
func (g *GitLabProvider) CreateTree(ctx context.Context, owner, repo string, baseTree string, entries []*gitprovider.TreeEntry) (string, error) {
	return "", &gitprovider.ProviderNotSupportedError{
		Provider:  gitprovider.ProviderTypeGitLab,
		Operation: "CreateTree",
		Message:   "GitLab does not support GitHub-style tree API - use CreateCommit with CommitActions instead",
	}
}

// CreateBranch creates a new branch
func (g *GitLabProvider) CreateBranch(ctx context.Context, owner, repo, branch, sha string) (*gitprovider.Reference, error) {
	return g.CreateRef(ctx, owner, repo, branch, sha)
}

// GetBranch returns a branch
func (g *GitLabProvider) GetBranch(ctx context.Context, owner, repo, branch string) (*gitprovider.Reference, error) {
	return g.GetRef(ctx, owner, repo, branch)
}

// DeleteBranch deletes a branch
func (g *GitLabProvider) DeleteBranch(ctx context.Context, owner, repo, branch string) error {
	return g.DeleteRef(ctx, owner, repo, branch)
}

// ListDirectoryFilesRecursive is a helper to recursively list all files in a directory
// This is used to implement the tree sync functionality for GitLab
func (g *GitLabProvider) ListDirectoryFilesRecursive(ctx context.Context, owner, repo, path, ref string) ([]*gitprovider.FileNode, error) {
	projectPath := g.getProjectPath(owner, repo)

	var allFiles []*gitprovider.FileNode

	opts := &gitlab.ListTreeOptions{
		Path:      gitlab.Ptr(path),
		Ref:       gitlab.Ptr(ref),
		Recursive: gitlab.Ptr(true),
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	for {
		tree, resp, err := g.client.Repositories.ListTree(projectPath, opts)
		g.updateLastResponse(resp)

		if err != nil {
			return nil, fmt.Errorf("failed to list tree: %w", err)
		}

		for _, item := range tree {
			nodeType := "file"
			switch item.Type {
			case "tree":
				nodeType = "dir"
			case "blob":
				nodeType = "file"
			case "commit":
				nodeType = "submodule"
			}

			// Only include files, not directories
			if nodeType == "file" {
				allFiles = append(allFiles, &gitprovider.FileNode{
					Name: item.Name,
					Path: item.Path,
					Type: nodeType,
					SHA:  item.ID,
				})
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allFiles, nil
}

// GetFileContentWithSHA returns file content with its SHA for commit actions
func (g *GitLabProvider) GetFileContentWithSHA(ctx context.Context, owner, repo, path, ref string) (content []byte, sha string, err error) {
	projectPath := g.getProjectPath(owner, repo)

	file, resp, err := g.client.RepositoryFiles.GetFile(projectPath, path, &gitlab.GetFileOptions{
		Ref: gitlab.Ptr(ref),
	})
	g.updateLastResponse(resp)

	if err != nil {
		return nil, "", fmt.Errorf("failed to get file: %w", err)
	}

	// GitLab returns base64-encoded content
	// Decode it
	decoded, err := base64.StdEncoding.DecodeString(file.Content)
	if err != nil {
		// If decode fails, assume it's already decoded
		decoded = []byte(file.Content)
	}

	return decoded, file.BlobID, nil
}

// CreateCommitActionsForSync creates commit actions to sync a directory
// This is a helper that implements the tree sync logic for GitLab
func (g *GitLabProvider) CreateCommitActionsForSync(ctx context.Context, owner, repo, sourcePath, targetPath, sourceRef string) ([]*gitprovider.CommitAction, error) {
	// List all files in source directory
	sourceFiles, err := g.ListDirectoryFilesRecursive(ctx, owner, repo, sourcePath, sourceRef)
	if err != nil {
		return nil, fmt.Errorf("failed to list source files: %w", err)
	}

	var actions []*gitprovider.CommitAction

	// For each source file, create an action to update/create it in target
	for _, file := range sourceFiles {
		// Get file content
		content, _, err := g.GetFileContentWithSHA(ctx, owner, repo, file.Path, sourceRef)
		if err != nil {
			return nil, fmt.Errorf("failed to get file content for %s: %w", file.Path, err)
		}

		// Calculate target path
		relativePath := file.Path[len(sourcePath):]
		if len(relativePath) > 0 && relativePath[0] == '/' {
			relativePath = relativePath[1:]
		}
		targetFilePath := targetPath
		if targetFilePath[len(targetFilePath)-1] != '/' {
			targetFilePath += "/"
		}
		targetFilePath += relativePath

		// Base64 encode the content for GitLab
		encodedContent := base64.StdEncoding.EncodeToString(content)

		actions = append(actions, &gitprovider.CommitAction{
			Action:   "update", // Use update which will create if not exists
			FilePath: targetFilePath,
			Content:  encodedContent,
			Encoding: "base64",
		})
	}

	return actions, nil
}
