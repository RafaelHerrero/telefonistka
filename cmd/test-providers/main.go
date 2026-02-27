package main

// Test Helper Script for GitProvider Implementation
//
// This script helps you manually test both GitHub and GitLab providers.
//
// Usage:
//   # Test GitHub
//   GITHUB_OAUTH_TOKEN=your_token go run test_providers.go github
//
//   # Test GitLab
//   GITLAB_TOKEN=your_token GITLAB_URL=https://gitlab.com go run test_providers.go gitlab
//
//   # Test both
//   GITHUB_OAUTH_TOKEN=gh_token GITLAB_TOKEN=gl_token go run test_providers.go both

import (
	"context"
	"fmt"
	"os"

	"github.com/commercetools/telefonistka/internal/pkg/gitprovider"
	_ "github.com/commercetools/telefonistka/internal/pkg/gitprovider/github"
	_ "github.com/commercetools/telefonistka/internal/pkg/gitprovider/gitlab"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_providers.go [github|gitlab|both]")
		os.Exit(1)
	}

	mode := os.Args[1]

	switch mode {
	case "github":
		testGitHub()
	case "gitlab":
		testGitLab()
	case "both":
		testGitHub()
		fmt.Println()
		testGitLab()
	default:
		fmt.Printf("Unknown mode: %s\n", mode)
		fmt.Println("Use: github, gitlab, or both")
		os.Exit(1)
	}
}

func testGitHub() {
	fmt.Println("====================================")
	fmt.Println("Testing GitHub Provider")
	fmt.Println("====================================")

	token := os.Getenv("GITHUB_OAUTH_TOKEN")
	if token == "" {
		fmt.Println("❌ GITHUB_OAUTH_TOKEN not set")
		fmt.Println("   Set it with: export GITHUB_OAUTH_TOKEN=your_token")
		return
	}

	factory, err := gitprovider.NewDefaultProviderFactory(10)
	if err != nil {
		fmt.Printf("❌ Failed to create factory: %v\n", err)
		return
	}

	config := &gitprovider.ProviderConfig{
		Type:  gitprovider.ProviderTypeGitHub,
		Token: token,
	}

	provider, err := factory.Create(config)
	if err != nil {
		fmt.Printf("❌ Failed to create provider: %v\n", err)
		return
	}

	fmt.Printf("✅ Provider created: %s\n", provider.Type())
	fmt.Printf("   Supports Tree API: %t\n", provider.SupportsTreeAPI())
	fmt.Printf("   Supports GraphQL: %t\n", provider.SupportsGraphQL())

	ctx := context.Background()

	// Test 1: Get bot identity
	fmt.Println("\n🧪 Test 1: Get Bot Identity")
	bot, err := provider.GetBotIdentity(ctx)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Printf("✅ Bot Identity: %s (Type: %s)\n", bot.Login, bot.Type)
	}

	// Test 2: Get repository
	fmt.Println("\n🧪 Test 2: Get Repository (commercetools/telefonistka)")
	repo, err := provider.GetRepository(ctx, "commercetools", "telefonistka")
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Printf("✅ Repository: %s\n", repo.FullName)
		fmt.Printf("   Default Branch: %s\n", repo.DefaultBranch)
		fmt.Printf("   Private: %t\n", repo.Private)
		fmt.Printf("   URL: %s\n", repo.HTMLURL)
	}

	// Test 3: Get branch
	if repo != nil {
		fmt.Println("\n🧪 Test 3: Get Branch")
		ref, err := provider.GetRef(ctx, "commercetools", "telefonistka", repo.DefaultBranch)
		if err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
		} else {
			fmt.Printf("✅ Branch: %s\n", ref.Ref)
			fmt.Printf("   SHA: %s\n", ref.SHA[:8])
		}
	}

	fmt.Println("\n✅ GitHub provider tests completed!")
}

func testGitLab() {
	fmt.Println("====================================")
	fmt.Println("Testing GitLab Provider")
	fmt.Println("====================================")

	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		fmt.Println("❌ GITLAB_TOKEN not set")
		fmt.Println("   Set it with: export GITLAB_TOKEN=your_token")
		return
	}

	baseURL := os.Getenv("GITLAB_URL")
	if baseURL == "" {
		baseURL = "https://gitlab.com"
		fmt.Printf("ℹ️  Using default GitLab URL: %s\n", baseURL)
	}

	factory, err := gitprovider.NewDefaultProviderFactory(10)
	if err != nil {
		fmt.Printf("❌ Failed to create factory: %v\n", err)
		return
	}

	config := &gitprovider.ProviderConfig{
		Type:    gitprovider.ProviderTypeGitLab,
		Token:   token,
		BaseURL: baseURL,
	}

	provider, err := factory.Create(config)
	if err != nil {
		fmt.Printf("❌ Failed to create provider: %v\n", err)
		return
	}

	fmt.Printf("✅ Provider created: %s\n", provider.Type())
	fmt.Printf("   Supports Tree API: %t\n", provider.SupportsTreeAPI())
	fmt.Printf("   Supports GraphQL: %t\n", provider.SupportsGraphQL())

	ctx := context.Background()

	// Test 1: Get bot identity
	fmt.Println("\n🧪 Test 1: Get Bot Identity")
	bot, err := provider.GetBotIdentity(ctx)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Printf("✅ Bot Identity: %s\n", bot.Login)
		fmt.Printf("   Name: %s\n", bot.Name)
		fmt.Printf("   Is Bot: %t\n", bot.IsBot)
	}

	// Test 2: Get repository (gitlab-org/gitlab is public)
	fmt.Println("\n🧪 Test 2: Get Repository (gitlab-org/gitlab)")
	repo, err := provider.GetRepository(ctx, "root", "test")
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		fmt.Println("   Note: This tests public repo access. For private repos, ensure your token has permissions.")
	} else {
		fmt.Printf("✅ Repository: %s\n", repo.FullName)
		fmt.Printf("   Default Branch: %s\n", repo.DefaultBranch)
		fmt.Printf("   Private: %t\n", repo.Private)
		fmt.Printf("   URL: %s\n", repo.HTMLURL)
	}

	// Test 3: Get branch
	if repo != nil {
		fmt.Println("\n🧪 Test 3: Get Branch")
		ref, err := provider.GetRef(ctx, "gitlab-org", "gitlab", repo.DefaultBranch)
		if err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
		} else {
			fmt.Printf("✅ Branch: %s\n", ref.Ref)
			fmt.Printf("   SHA: %s\n", ref.SHA[:8])
		}
	}

	fmt.Println("\n✅ GitLab provider tests completed!")
}
