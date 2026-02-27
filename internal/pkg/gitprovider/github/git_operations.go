package github

import (
	"context"
	"fmt"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	"github.com/google/go-github/v62/github"
)

// GetRef returns a Git reference
func (g *GitHubProvider) GetRef(ctx context.Context, owner, repo, ref string) (*gitprovider.Reference, error) {
	// GitHub requires refs to start with "refs/"
	if ref[:5] != "refs/" {
		ref = "refs/heads/" + ref
	}

	ghRef, resp, err := g.v3Client.Git.GetRef(ctx, owner, repo, ref)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get ref: %w", err)
	}

	return &gitprovider.Reference{
		Ref:    ghRef.GetRef(),
		SHA:    ghRef.GetObject().GetSHA(),
		NodeID: ghRef.GetNodeID(),
	}, nil
}

// CreateRef creates a new Git reference
func (g *GitHubProvider) CreateRef(ctx context.Context, owner, repo, ref, sha string) (*gitprovider.Reference, error) {
	// GitHub requires refs to start with "refs/"
	if ref[:5] != "refs/" {
		ref = "refs/heads/" + ref
	}

	ghRef := &github.Reference{
		Ref: github.String(ref),
		Object: &github.GitObject{
			SHA: github.String(sha),
		},
	}

	createdRef, resp, err := g.v3Client.Git.CreateRef(ctx, owner, repo, ghRef)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to create ref: %w", err)
	}

	return &gitprovider.Reference{
		Ref:    createdRef.GetRef(),
		SHA:    createdRef.GetObject().GetSHA(),
		NodeID: createdRef.GetNodeID(),
	}, nil
}

// UpdateRef updates a Git reference
func (g *GitHubProvider) UpdateRef(ctx context.Context, owner, repo, ref, sha string, force bool) (*gitprovider.Reference, error) {
	// GitHub requires refs to start with "refs/"
	if ref[:5] != "refs/" {
		ref = "refs/heads/" + ref
	}

	ghRef := &github.Reference{
		Ref: github.String(ref),
		Object: &github.GitObject{
			SHA: github.String(sha),
		},
	}

	updatedRef, resp, err := g.v3Client.Git.UpdateRef(ctx, owner, repo, ghRef, force)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to update ref: %w", err)
	}

	return &gitprovider.Reference{
		Ref:    updatedRef.GetRef(),
		SHA:    updatedRef.GetObject().GetSHA(),
		NodeID: updatedRef.GetNodeID(),
	}, nil
}

// DeleteRef deletes a Git reference
func (g *GitHubProvider) DeleteRef(ctx context.Context, owner, repo, ref string) error {
	// GitHub requires refs to start with "refs/"
	if ref[:5] != "refs/" {
		ref = "refs/heads/" + ref
	}

	resp, err := g.v3Client.Git.DeleteRef(ctx, owner, repo, ref)
	g.updateLastResponse(resp)

	if err != nil {
		return fmt.Errorf("failed to delete ref: %w", err)
	}

	return nil
}

// GetCommit returns a Git commit
func (g *GitHubProvider) GetCommit(ctx context.Context, owner, repo, sha string) (*gitprovider.Commit, error) {
	ghCommit, resp, err := g.v3Client.Git.GetCommit(ctx, owner, repo, sha)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	var parents []string
	for _, p := range ghCommit.Parents {
		parents = append(parents, p.GetSHA())
	}

	return &gitprovider.Commit{
		SHA:       ghCommit.GetSHA(),
		Message:   ghCommit.GetMessage(),
		Author:    ghCommit.GetAuthor().GetName(),
		Date:      ghCommit.GetAuthor().GetDate().Time,
		TreeSHA:   ghCommit.GetTree().GetSHA(),
		Parents:   parents,
		HTMLURL:   ghCommit.GetHTMLURL(),
		Committer: ghCommit.GetCommitter().GetName(),
	}, nil
}

// CreateCommit creates a new Git commit
func (g *GitHubProvider) CreateCommit(ctx context.Context, owner, repo string, opts *gitprovider.CommitOptions) (*gitprovider.Commit, error) {
	// For GitHub, we use the tree API
	// First, create the tree
	var treeSHA string
	var err error

	if len(opts.TreeEntries) > 0 {
		// Create tree from entries
		treeSHA, err = g.CreateTree(ctx, owner, repo, opts.ParentSHA, opts.TreeEntries)
		if err != nil {
			return nil, fmt.Errorf("failed to create tree: %w", err)
		}
	} else if len(opts.CommitActions) > 0 {
		// Convert CommitActions to TreeEntries
		// This is a fallback for GitLab-style commits
		// For now, return an error as this should use the actions-based approach
		return nil, fmt.Errorf("GitHub provider requires TreeEntries, not CommitActions")
	} else {
		return nil, fmt.Errorf("no tree entries or commit actions provided")
	}

	// Get parent commit
	parentCommit, err := g.GetCommit(ctx, owner, repo, opts.ParentSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent commit: %w", err)
	}

	// Create the commit
	ghCommit := &github.Commit{
		Message: github.String(opts.Message),
		Tree: &github.Tree{
			SHA: github.String(treeSHA),
		},
		Parents: []*github.Commit{
			{SHA: github.String(opts.ParentSHA)},
		},
	}

	if opts.Author != nil {
		ghCommit.Author = &github.CommitAuthor{
			Name:  github.String(opts.Author.Name),
			Email: github.String(opts.Author.Email),
			Date:  &github.Timestamp{Time: opts.Author.Date},
		}
	}

	if opts.Committer != nil {
		ghCommit.Committer = &github.CommitAuthor{
			Name:  github.String(opts.Committer.Name),
			Email: github.String(opts.Committer.Email),
			Date:  &github.Timestamp{Time: opts.Committer.Date},
		}
	}

	createdCommit, resp, err := g.v3Client.Git.CreateCommit(ctx, owner, repo, ghCommit, nil)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to create commit: %w", err)
	}

	result := &gitprovider.Commit{
		SHA:       createdCommit.GetSHA(),
		Message:   createdCommit.GetMessage(),
		Author:    createdCommit.GetAuthor().GetName(),
		Date:      createdCommit.GetAuthor().GetDate().Time,
		TreeSHA:   createdCommit.GetTree().GetSHA(),
		Parents:   []string{parentCommit.SHA},
		HTMLURL:   createdCommit.GetHTMLURL(),
		Committer: createdCommit.GetCommitter().GetName(),
	}

	// If UpdateBranch is true, update the branch to point to the new commit
	if opts.UpdateBranch && opts.Branch != "" {
		_, err = g.UpdateRef(ctx, owner, repo, opts.Branch, result.SHA, false)
		if err != nil {
			// If CreateBranch is true, try creating the branch instead
			if opts.CreateBranch {
				_, err = g.CreateRef(ctx, owner, repo, opts.Branch, result.SHA)
				if err != nil {
					return result, fmt.Errorf("commit created but failed to create branch: %w", err)
				}
			} else {
				return result, fmt.Errorf("commit created but failed to update branch: %w", err)
			}
		}
	}

	return result, nil
}

// CompareCommits compares two commits
func (g *GitHubProvider) CompareCommits(ctx context.Context, owner, repo, base, head string) (*gitprovider.DiffResult, error) {
	comparison, resp, err := g.v3Client.Repositories.CompareCommits(ctx, owner, repo, base, head, nil)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to compare commits: %w", err)
	}

	result := &gitprovider.DiffResult{
		FromSHA:   base,
		ToSHA:     head,
		Files:     convertGitHubCommitFiles(comparison.Files),
		Additions: comparison.GetTotalCommits(),
		Deletions: 0, // GitHub API doesn't provide this directly
		Changes:   len(comparison.Files),
	}

	return result, nil
}

// GetTree returns a Git tree
func (g *GitHubProvider) GetTree(ctx context.Context, owner, repo, sha string, recursive bool) ([]*gitprovider.TreeEntry, error) {
	ghTree, resp, err := g.v3Client.Git.GetTree(ctx, owner, repo, sha, recursive)
	g.updateLastResponse(resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get tree: %w", err)
	}

	var entries []*gitprovider.TreeEntry
	for _, entry := range ghTree.Entries {
		entries = append(entries, &gitprovider.TreeEntry{
			Path: entry.GetPath(),
			Mode: entry.GetMode(),
			Type: entry.GetType(),
			SHA:  entry.GetSHA(),
			Size: entry.GetSize(),
			URL:  entry.GetURL(),
		})
	}

	return entries, nil
}

// CreateTree creates a Git tree
func (g *GitHubProvider) CreateTree(ctx context.Context, owner, repo string, baseTree string, entries []*gitprovider.TreeEntry) (string, error) {
	// Get the base tree SHA if we're building on top of an existing tree
	var baseTreeSHA string
	if baseTree != "" {
		// Get the commit's tree SHA
		commit, err := g.GetCommit(ctx, owner, repo, baseTree)
		if err != nil {
			return "", fmt.Errorf("failed to get base tree: %w", err)
		}
		baseTreeSHA = commit.TreeSHA
	}

	// Convert to GitHub tree entries
	var ghEntries []*github.TreeEntry
	for _, entry := range entries {
		ghEntry := &github.TreeEntry{
			Path: github.String(entry.Path),
			Mode: github.String(entry.Mode),
			Type: github.String(entry.Type),
		}

		if entry.SHA != "" {
			ghEntry.SHA = github.String(entry.SHA)
		}

		ghEntries = append(ghEntries, ghEntry)
	}

	// Create the tree
	createdTree, resp, err := g.v3Client.Git.CreateTree(ctx, owner, repo, baseTreeSHA, ghEntries)
	g.updateLastResponse(resp)

	if err != nil {
		return "", fmt.Errorf("failed to create tree: %w", err)
	}

	return createdTree.GetSHA(), nil
}

// CreateBranch creates a new branch
func (g *GitHubProvider) CreateBranch(ctx context.Context, owner, repo, branch, sha string) (*gitprovider.Reference, error) {
	return g.CreateRef(ctx, owner, repo, branch, sha)
}

// GetBranch returns a branch
func (g *GitHubProvider) GetBranch(ctx context.Context, owner, repo, branch string) (*gitprovider.Reference, error) {
	return g.GetRef(ctx, owner, repo, branch)
}

// DeleteBranch deletes a branch
func (g *GitHubProvider) DeleteBranch(ctx context.Context, owner, repo, branch string) error {
	return g.DeleteRef(ctx, owner, repo, branch)
}
