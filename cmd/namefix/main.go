package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/go-i2p/gitlab-to-gitea/config"
	"github.com/go-i2p/gitlab-to-gitea/gitea"
	"github.com/go-i2p/gitlab-to-gitea/utils"
)

const scriptVersion = "1.0.0"

func main() {
	utils.PrintHeader("---=== Gitea Repository Name Inconsistency Checker ===---")
	fmt.Printf("Version: %s\n\n", scriptVersion)

	// Define command line flags
	sourceOwner := flag.String("source-owner", "", "The Gitea username or organization whose repositories will be used as reference")
	targetOwner := flag.String("target-owner", "", "The Gitea username or organization whose repositories will be checked for naming inconsistencies")
	outputFormat := flag.String("output-format", "both", "Format for results output (options: 'json', 'text', or 'both')")
	help := flag.Bool("help", false, "Show help")

	flag.Parse()

	// Display help if requested or if no arguments provided
	if *help || flag.NFlag() == 0 {
		printUsage()
		return
	}

	// Load env file
	err := config.LoadEnv()
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to load environment variables: %v", err))
		os.Exit(1)
	}

	// Set defaults for environment variables
	config.SetEnvDefaults()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to load configuration: %v", err))
		os.Exit(1)
	}

	// Use environment variables as defaults if not provided in the command line
	if *sourceOwner == "" {
		*sourceOwner = os.Getenv("SOURCE_OWNER")
		if *sourceOwner == "" {
			utils.PrintError("Source owner is required. Provide it via --source-owner flag or SOURCE_OWNER environment variable.")
			os.Exit(1)
		}
	}

	if *targetOwner == "" {
		*targetOwner = os.Getenv("TARGET_OWNER")
		if *targetOwner == "" {
			utils.PrintError("Target owner is required. Provide it via --target-owner flag or TARGET_OWNER environment variable.")
			os.Exit(1)
		}
	}

	// Validate output format
	if *outputFormat != "json" && *outputFormat != "text" && *outputFormat != "both" {
		utils.PrintError("Invalid output format. Use 'json', 'text', or 'both'.")
		os.Exit(1)
	}

	// Initialize Gitea client
	giteaClient, err := gitea.NewClient(cfg.GiteaURL, cfg.GiteaToken, cfg.InsecureSkipTLSVerify)
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to connect to Gitea: %v", err))
		os.Exit(1)
	}

	// Verify connection
	gtVersion, err := giteaClient.GetVersion()
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to get Gitea version: %v", err))
		os.Exit(1)
	}
	utils.PrintInfo(fmt.Sprintf("Connected to Gitea, version: %s", gtVersion))

	// Create and run the NameFixer
	nameFixer := NewNameFixer(giteaClient, *sourceOwner, *targetOwner)
	utils.PrintHeader("Starting repository name inconsistency check...")
	inconsistencies, err := nameFixer.Run()
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to check for name inconsistencies: %v", err))
		os.Exit(1)
	}

	// Output results
	if len(inconsistencies) == 0 {
		utils.PrintSuccess("No repository naming inconsistencies found.")
		return
	}

	if *outputFormat == "text" || *outputFormat == "both" {
		outputText(inconsistencies)
	}

	if *outputFormat == "json" || *outputFormat == "both" {
		outputJSON(inconsistencies)
	}

	utils.PrintSuccess("Repository name inconsistency check completed.")
}

// printUsage displays the help information
func printUsage() {
	fmt.Println("Gitea Repository Name Inconsistency Checker")
	fmt.Println("\nThis tool identifies repository naming inconsistencies between original repositories and their forks.")
	fmt.Println("\nUsage:")
	fmt.Println("  namefix --source-owner=\"original-org\" --target-owner=\"forking-user\"")
	fmt.Println("\nOptions:")
	flag.PrintDefaults()
}

// outputText prints the inconsistencies in human-readable text format
func outputText(inconsistencies []Inconsistency) {
	utils.PrintHeader(fmt.Sprintf("Found %d repository naming inconsistencies:", len(inconsistencies)))
	fmt.Println()

	for i, inc := range inconsistencies {
		fmt.Printf("%d. [%s/%s] -> [%s/%s]\n", i+1,
			inc.SourceRepo.Owner, inc.SourceRepo.Name,
			inc.TargetRepo.Owner, inc.TargetRepo.Name)
		fmt.Printf("   Initial commit: %s\n\n", inc.SourceRepo.InitialCommitHash)
	}
}

// outputJSON prints the inconsistencies in JSON format
func outputJSON(inconsistencies []Inconsistency) {
	jsonData, err := json.MarshalIndent(inconsistencies, "", "  ")
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to generate JSON output: %v", err))
		return
	}

	fmt.Println(string(jsonData))
}
