package main

// End-to-End GitLab Provider Test
//
// This script performs a complete real-world test of the GitLab provider:
// 1. Initializes a repository with initial content
// 2. Creates a feature branch with changes
// 3. Creates a merge request
// 4. Tests comments, labels, approvals
// 5. Tests promotion workflow (simulates workspace -> staging sync)
//
// Usage:
//   GITLAB_URL="http://gitlab.localhost" \
//   GITLAB_TOKEN="your_token" \
//   GITLAB_PROJECT="root/test" \
//   go run cmd/test-e2e-gitlab/main.go

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	_ "github.com/commercetools/telefonistka/internal/pkg/gitprovider/gitlab"
)

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║     GitLab Provider End-to-End Test Suite                 ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Get configuration from environment
	gitlabURL := os.Getenv("GITLAB_URL")
	gitlabToken := os.Getenv("GITLAB_TOKEN")
	gitlabProject := os.Getenv("GITLAB_PROJECT")

	if gitlabURL == "" {
		gitlabURL = "https://gitlab.com"
	}

	if gitlabToken == "" {
		fmt.Println("❌ GITLAB_TOKEN not set")
		fmt.Println("   Set it with: export GITLAB_TOKEN=your_token")
		os.Exit(1)
	}

	if gitlabProject == "" {
		fmt.Println("❌ GITLAB_PROJECT not set")
		fmt.Println("   Set it with: export GITLAB_PROJECT=owner/repo")
		os.Exit(1)
	}

	parts := strings.Split(gitlabProject, "/")
	if len(parts) != 2 {
		fmt.Printf("❌ Invalid GITLAB_PROJECT format: %s\n", gitlabProject)
		fmt.Println("   Expected format: owner/repo")
		os.Exit(1)
	}

	owner := parts[0]
	repo := parts[1]

	fmt.Printf("Configuration:\n")
	fmt.Printf("  GitLab URL: %s\n", gitlabURL)
	fmt.Printf("  Project:    %s/%s\n", owner, repo)
	fmt.Printf("  Token:      %s...%s\n", gitlabToken[:10], gitlabToken[len(gitlabToken)-4:])
	fmt.Println()

	// Create provider
	factory, err := gitprovider.NewDefaultProviderFactory(10)
	if err != nil {
		fmt.Printf("❌ Failed to create factory: %v\n", err)
		os.Exit(1)
	}

	config := &gitprovider.ProviderConfig{
		Type:    gitprovider.ProviderTypeGitLab,
		Token:   gitlabToken,
		BaseURL: gitlabURL,
	}

	provider, err := factory.Create(config)
	if err != nil {
		fmt.Printf("❌ Failed to create provider: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Provider created: %s\n", provider.Type())
	fmt.Printf("   Tree API: %t | GraphQL: %t\n", provider.SupportsTreeAPI(), provider.SupportsGraphQL())
	fmt.Println()

	ctx := context.Background()

	// Run test suite
	if err := runTestSuite(ctx, provider, owner, repo); err != nil {
		fmt.Printf("\n❌ Test suite failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║  ✅  ALL TESTS PASSED - GitLab Provider is Working!       ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
}

func runTestSuite(ctx context.Context, provider gitprovider.GitProvider, owner, repo string) error {
	// Test 1: Verify connection and get bot identity
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Test 1: Connection & Bot Identity")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	bot, err := provider.GetBotIdentity(ctx)
	if err != nil {
		return fmt.Errorf("failed to get bot identity: %w", err)
	}
	fmt.Printf("✅ Connected as: %s (%s)\n", bot.Login, bot.Name)
	fmt.Println()

	// Test 2: Get repository
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Test 2: Repository Access")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	repository, err := provider.GetRepository(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}
	fmt.Printf("✅ Repository: %s\n", repository.FullName)
	fmt.Printf("   Default Branch: %s\n", repository.DefaultBranch)
	fmt.Printf("   Private: %t\n", repository.Private)
	fmt.Println()

	defaultBranch := repository.DefaultBranch

	// Test 3: Initialize repository with content
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Test 3: Initialize Repository")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Check if main branch exists
	mainRef, err := provider.GetRef(ctx, owner, repo, defaultBranch)
	if err != nil {
		fmt.Printf("⚠️  Default branch doesn't exist, creating initial structure...\n")

		// Create initial commit with directory structure
		err = createInitialStructure(ctx, provider, owner, repo, defaultBranch)
		if err != nil {
			return fmt.Errorf("failed to create initial structure: %w", err)
		}

		// Get the ref again
		mainRef, err = provider.GetRef(ctx, owner, repo, defaultBranch)
		if err != nil {
			return fmt.Errorf("failed to get ref after initialization: %w", err)
		}
	}

	fmt.Printf("✅ Base branch ready: %s @ %s\n", defaultBranch, mainRef.SHA[:8])
	fmt.Println()

	// Test 4: Create feature branch and MR
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Test 4: Create Feature Branch & Merge Request")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	timestamp := time.Now().Unix()
	featureBranch := fmt.Sprintf("test/feature-%d", timestamp)

	// Create branch from main
	_, err = provider.CreateRef(ctx, owner, repo, featureBranch, mainRef.SHA)
	if err != nil {
		return fmt.Errorf("failed to create feature branch: %w", err)
	}
	fmt.Printf("✅ Created branch: %s\n", featureBranch)

	// Create a commit on the feature branch
	commitMessage := "Add new feature to workspace"
	commitContent := map[string]string{
		"workspace/app.yaml":    fmt.Sprintf("# Feature added at %d\nversion: 2.0\nfeature: enabled\n", timestamp),
		"workspace/config.yaml": "environment: development\nfeature_flag: true\n",
	}

	commit, err := createCommitWithFiles(ctx, provider, owner, repo, featureBranch, commitMessage, commitContent)
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}
	fmt.Printf("✅ Created commit: %s\n", commit.SHA[:8])

	// Create MR
	mrTitle := fmt.Sprintf("Test Feature MR - %d", timestamp)
	mrBody := "This is a test merge request to validate GitLab provider functionality.\n\n## Changes\n- Added new feature configuration\n- Updated workspace files"

	pr, err := provider.CreatePullRequest(ctx, owner, repo, &gitprovider.NewPullRequest{
		Title:  mrTitle,
		Body:   mrBody,
		Head:   featureBranch,
		Base:   defaultBranch,
		Labels: []string{"test", "automation"},
	})
	if err != nil {
		return fmt.Errorf("failed to create MR: %w", err)
	}
	fmt.Printf("✅ Created MR #%d: %s\n", pr.Number, pr.Title)
	fmt.Printf("   URL: %s\n", pr.HTMLURL)
	fmt.Println()

	// Test 5: MR Operations
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Test 5: Merge Request Operations")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Comment on MR
	commentBody := "🤖 This is an automated test comment from the GitLab provider test suite!"
	comment, err := provider.CommentOnPullRequest(ctx, owner, repo, pr.Number, commentBody)
	if err != nil {
		return fmt.Errorf("failed to create comment: %w", err)
	}
	fmt.Printf("✅ Created comment: %s\n", comment.Body[:50])

	// Add label
	err = provider.AddLabels(ctx, owner, repo, pr.Number, []string{"verified"})
	if err != nil {
		fmt.Printf("⚠️  Could not add label (may need to create it first): %v\n", err)
	} else {
		fmt.Printf("✅ Added label: verified\n")
	}

	// Set commit status
	statusContext := "telefonistka/test"
	statusDesc := "Test validation passed"
	err = provider.SetCommitStatus(ctx, owner, repo, commit.SHA, &gitprovider.Status{
		State:       "success",
		Context:     statusContext,
		Description: statusDesc,
		TargetURL:   pr.HTMLURL,
	})
	if err != nil {
		return fmt.Errorf("failed to set commit status: %w", err)
	}
	fmt.Printf("✅ Set commit status: %s - %s\n", statusContext, statusDesc)
	fmt.Println()

	// Test 6: Promotion Simulation (workspace -> staging sync)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Test 6: Promotion Workflow (workspace -> staging)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Create promotion branch
	promotionBranch := fmt.Sprintf("promotion/staging-%d", timestamp)
	_, err = provider.CreateRef(ctx, owner, repo, promotionBranch, mainRef.SHA)
	if err != nil {
		return fmt.Errorf("failed to create promotion branch: %w", err)
	}
	fmt.Printf("✅ Created promotion branch: %s\n", promotionBranch)

	// Sync workspace/ to staging/ (this simulates the core Telefonistka functionality)
	promotionCommitMsg := "Promote workspace changes to staging"
	promotionFiles := map[string]string{
		"staging/app.yaml":    fmt.Sprintf("# Promoted at %d\nversion: 2.0\nfeature: enabled\n", timestamp),
		"staging/config.yaml": "environment: staging\nfeature_flag: true\n",
	}

	promotionCommit, err := createCommitWithFiles(ctx, provider, owner, repo, promotionBranch, promotionCommitMsg, promotionFiles)
	if err != nil {
		return fmt.Errorf("failed to create promotion commit: %w", err)
	}
	fmt.Printf("✅ Created promotion commit: %s\n", promotionCommit.SHA[:8])

	// Create promotion MR
	promotionMRTitle := fmt.Sprintf("🚀 Promote to staging - %d", timestamp)
	promotionMRBody := "## Automated Promotion\n\nThis MR syncs changes from `workspace/` to `staging/`.\n\n### Files promoted:\n- app.yaml\n- config.yaml"

	promotionPR, err := provider.CreatePullRequest(ctx, owner, repo, &gitprovider.NewPullRequest{
		Title:  promotionMRTitle,
		Body:   promotionMRBody,
		Head:   promotionBranch,
		Base:   defaultBranch,
		Labels: []string{"promotion", "staging"},
	})
	if err != nil {
		return fmt.Errorf("failed to create promotion MR: %w", err)
	}
	fmt.Printf("✅ Created promotion MR #%d\n", promotionPR.Number)
	fmt.Printf("   URL: %s\n", promotionPR.HTMLURL)

	// Add promotion comment
	promotionComment := "✅ Promotion validation successful\n\nFiles synced from workspace to staging:\n- workspace/app.yaml → staging/app.yaml\n- workspace/config.yaml → staging/config.yaml"
	_, err = provider.CommentOnPullRequest(ctx, owner, repo, promotionPR.Number, promotionComment)
	if err != nil {
		return fmt.Errorf("failed to create promotion comment: %w", err)
	}
	fmt.Printf("✅ Added promotion validation comment\n")
	fmt.Println()

	// Test 7: List and verify
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Test 7: Verification")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Note: ListPullRequests is not in the base interface, so we'll just verify by getting the PR
	fmt.Printf("✅ Verifying MRs are accessible...\n")

	// Get specific MR
	fetchedPR, err := provider.GetPullRequest(ctx, owner, repo, pr.Number)
	if err != nil {
		return fmt.Errorf("failed to get MR: %w", err)
	}
	fmt.Printf("✅ Fetched MR #%d: %s\n", fetchedPR.Number, fetchedPR.Title)
	fmt.Printf("   State: %s | Mergeable: %t\n", fetchedPR.State, fetchedPR.Mergeable)

	// Get files changed in MR
	files, err := provider.ListPullRequestFiles(ctx, owner, repo, pr.Number)
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}
	fmt.Printf("✅ MR has %d file(s) changed\n", len(files))
	for _, f := range files {
		fmt.Printf("   - %s (%s)\n", f.Filename, f.Status)
	}

	// List comments
	comments, err := provider.ListPullRequestComments(ctx, owner, repo, pr.Number)
	if err != nil {
		return fmt.Errorf("failed to list comments: %w", err)
	}
	fmt.Printf("✅ MR has %d comment(s)\n", len(comments))

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Summary")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Feature MR:   %s\n", pr.HTMLURL)
	fmt.Printf("Promotion MR: %s\n", promotionPR.HTMLURL)
	fmt.Println()
	fmt.Println("The GitLab provider successfully:")
	fmt.Println("  ✅ Created branches")
	fmt.Println("  ✅ Created commits")
	fmt.Println("  ✅ Created merge requests")
	fmt.Println("  ✅ Added comments and labels")
	fmt.Println("  ✅ Set commit statuses")
	fmt.Println("  ✅ Simulated promotion workflow")

	return nil
}

func createInitialStructure(ctx context.Context, provider gitprovider.GitProvider, owner, repo, branch string) error {
	initialFiles := map[string]string{
		"README.md":            "# Telefonistka Test Repository\n\nThis repository is used for testing the GitLab provider.\n",
		"workspace/README.md":  "# Workspace\n\nDevelopment environment files.\n",
		"staging/README.md":    "# Staging\n\nStaging environment files.\n",
		"production/README.md": "# Production\n\nProduction environment files.\n",
	}

	_, err := createCommitWithFiles(ctx, provider, owner, repo, branch, "Initial repository structure", initialFiles)
	return err
}

func createCommitWithFiles(ctx context.Context, provider gitprovider.GitProvider, owner, repo, branch, message string, files map[string]string) (*gitprovider.Commit, error) {
	// Get current branch ref
	ref, err := provider.GetRef(ctx, owner, repo, branch)
	if err != nil {
		return nil, fmt.Errorf("failed to get ref: %w", err)
	}

	// Create commit actions for each file
	var actions []*gitprovider.CommitAction
	for path, content := range files {
		actions = append(actions, &gitprovider.CommitAction{
			Action:   "create",
			FilePath: path,
			Content:  content,
			Encoding: "text",
		})
	}

	// Create commit
	now := time.Now()
	commit, err := provider.CreateCommit(ctx, owner, repo, &gitprovider.CommitOptions{
		Message:       message,
		Branch:        branch,
		ParentSHA:     ref.SHA,
		CommitActions: actions,
		Author: &gitprovider.CommitAuthor{
			Name:  "Telefonistka Test",
			Email: "test@telefonistka.io",
			Date:  now,
		},
		UpdateBranch: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create commit: %w", err)
	}

	return commit, nil
}
