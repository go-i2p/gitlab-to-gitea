package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-i2p/gitlab-to-gitea/config"
	"github.com/go-i2p/gitlab-to-gitea/gitea"
	"github.com/go-i2p/gitlab-to-gitea/utils"
	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

const (
	scriptVersion = "1.0.0"
)

// Repository types for processing
type RepoInfo struct {
	Name        string
	FullName    string
	Description string
	CloneURL    string
	HTMLURL     string
	IsPrivate   bool
}

func main() {
	utils.PrintHeader("---=== GitHub to Gitea Repository Mirror ===---")
	fmt.Printf("Version: %s\n\n", scriptVersion)

	// Define command line flags
	githubAccount := flag.String("account", "", "GitHub username or organization name (required)")
	isOrg := flag.Bool("org", false, "Treat the account as an organization")
	githubToken := flag.String("github-token", "", "GitHub personal access token (optional but recommended to avoid rate limits)")
	targetOwner := flag.String("target-owner", "", "Gitea account where repositories will be created (defaults to current user)")
	includePrivate := flag.Bool("include-private", false, "Include private repositories (requires authentication)")
	help := flag.Bool("help", false, "Show usage information")

	flag.Parse()

	// Show help if requested or required args missing
	if *help || *githubAccount == "" {
		showUsage()
		return
	}

	// Load environment variables
	err := config.LoadEnv()
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to load environment variables: %v", err))
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to load configuration: %v", err))
		os.Exit(1)
	}

	// Initialize GitHub client
	githubClient := createGitHubClient(*githubToken)

	// Initialize Gitea client
	giteaClient, err := gitea.NewClient(cfg.GiteaURL, cfg.GiteaToken, cfg.InsecureSkipTLSVerify)
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to connect to Gitea: %v", err))
		os.Exit(1)
	}

	// Verify connections
	gtVersion, err := giteaClient.GetVersion()
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to get Gitea version: %v", err))
		os.Exit(1)
	}
	utils.PrintInfo(fmt.Sprintf("Connected to Gitea, version: %s", gtVersion))

	// Get Gitea current user if target owner not specified
	if *targetOwner == "" {
		var currentUser map[string]interface{}
		err = giteaClient.Get("user", &currentUser)
		if err != nil {
			utils.PrintError(fmt.Sprintf("Failed to get current user: %v", err))
			os.Exit(1)
		}
		*targetOwner = currentUser["username"].(string)
		utils.PrintInfo(fmt.Sprintf("Target owner set to current user: %s", *targetOwner))
	}

	// Get GitHub repositories
	repos, err := getGitHubRepositories(githubClient, *githubAccount, *isOrg, *includePrivate)
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to get GitHub repositories: %v", err))
		os.Exit(1)
	}

	if len(repos) == 0 {
		utils.PrintWarning(fmt.Sprintf("No repositories found for %s", *githubAccount))
		return
	}

	utils.PrintInfo(fmt.Sprintf("Found %d repositories for %s", len(repos), *githubAccount))

	// Mirror repositories to Gitea
	mirrorRepositories(giteaClient, repos, *targetOwner)
}

// showUsage displays the help information
func showUsage() {
	fmt.Println("GitHub to Gitea Repository Mirror")
	fmt.Println("\nThis tool mirrors GitHub repositories to a Gitea instance.")
	fmt.Println("\nUsage:")
	fmt.Println("  mirror -account <username> [-org] [-github-token <token>] [-target-owner <owner>] [-include-private]")
	fmt.Println("\nOptions:")
	flag.PrintDefaults()
	fmt.Println("\nNOTE: GitHub has API rate limits - 60 requests/hour for unauthenticated requests, 5000 requests/hour with a token.")
}

// createGitHubClient initializes a GitHub API client
func createGitHubClient(token string) *github.Client {
	ctx := context.Background()
	if token == "" {
		// Unauthenticated client (rate limited to 60 requests/hour)
		return github.NewClient(nil)
	}

	// Authenticated client (rate limited to 5000 requests/hour)
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

// getGitHubRepositories fetches repositories from GitHub
func getGitHubRepositories(client *github.Client, account string, isOrg, includePrivate bool) ([]RepoInfo, error) {
	ctx := context.Background()
	var allRepos []RepoInfo

	var page int = 1

	for {
		var (
			repos []*github.Repository
			resp  *github.Response
			err   error
		)

		if isOrg {
			orgOpts := &github.RepositoryListByOrgOptions{
				ListOptions: github.ListOptions{PerPage: 100, Page: page},
			}
			repos, resp, err = client.Repositories.ListByOrg(ctx, account, orgOpts)
		} else {
			opts := &github.RepositoryListOptions{
				ListOptions: github.ListOptions{PerPage: 100, Page: page},
			}
			repos, resp, err = client.Repositories.List(ctx, account, opts)
		}

		if err != nil {
			return nil, err
		}

		// Convert and filter repositories
		for _, repo := range repos {
			// Skip private repos if not explicitly included
			if *repo.Private && !includePrivate {
				continue
			}

			allRepos = append(allRepos, RepoInfo{
				Name:        *repo.Name,
				FullName:    *repo.FullName,
				Description: stringOrEmpty(repo.Description),
				CloneURL:    *repo.CloneURL,
				HTMLURL:     *repo.HTMLURL,
			})
		}
		// Check if we need to get more pages
		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
		// just do the first 100 for now.
		break
	}

	return allRepos, nil
}

// mirrorRepositories creates mirror repositories in Gitea
func mirrorRepositories(client *gitea.Client, repos []RepoInfo, targetOwner string) {
	var (
		succeeded int
		failed    int
	)

	utils.PrintHeader(fmt.Sprintf("Starting mirror process for %d repositories...", len(repos)))

	for _, repo := range repos {
		utils.PrintInfo(fmt.Sprintf("Mirroring %s...", repo.FullName))

		// Create the mirror repository in Gitea
		mirrorData := map[string]interface{}{
			"clone_addr":  repo.CloneURL,
			"repo_name":   repo.Name,
			"mirror":      true,
			"private":     repo.IsPrivate,
			"description": repo.Description,
			"repo_owner":  targetOwner,
			"service":     "git",
			"wiki":        true,
			"issues":      true,
			"labels":      true,
			"milestones":  true,
			"releases":    true,
		}

		var result map[string]interface{}
		err := client.Post("repos/migrate", mirrorData, &result)

		if err != nil {
			utils.PrintError(fmt.Sprintf("Failed to mirror %s: %v", repo.FullName, err))
			failed++
		} else {
			utils.PrintSuccess(fmt.Sprintf("Successfully mirrored %s", repo.FullName))
			succeeded++
		}

		// Avoid hitting rate limits
		time.Sleep(1 * time.Second)
	}

	fmt.Println()
	utils.PrintInfo(fmt.Sprintf("Mirror summary: %d succeeded, %d failed", succeeded, failed))
}

// Helper function for nil string pointers
func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
