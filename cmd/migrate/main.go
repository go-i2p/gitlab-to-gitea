// main.go

// Package main provides the entry point for the GitLab to Gitea migration tool
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-i2p/gitlab-to-gitea/config"
	"github.com/go-i2p/gitlab-to-gitea/gitea"
	"github.com/go-i2p/gitlab-to-gitea/gitlab"
	"github.com/go-i2p/gitlab-to-gitea/migration"
	"github.com/go-i2p/gitlab-to-gitea/utils"
)

const (
	scriptVersion = "1.0.0"
)

func main() {
	insecure := flag.Bool("insecure-skip-tls-verify", false,
		"Do not verify TLS certificates of the GitLab and Gitea servers (overrides INSECURE_SKIP_TLS_VERIFY)")
	flag.Parse()

	utils.PrintHeader("---=== GitLab to Gitea migration ===---")
	fmt.Printf("Version: %s\n\n", scriptVersion)

	// Load env file
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

	if *insecure {
		cfg.InsecureSkipTLSVerify = true
	}
	if cfg.InsecureSkipTLSVerify {
		utils.PrintWarning("TLS certificate verification is DISABLED for GitLab and Gitea API connections.")
		utils.PrintWarning("Gitea's own clone from GitLab is governed separately by [migrations] SKIP_TLS_VERIFY in Gitea's app.ini.")
	}

	// Initialize clients
	gitlabClient, err := gitlab.NewClient(cfg.GitLabURL, cfg.GitLabToken, cfg.InsecureSkipTLSVerify)
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to connect to GitLab: %v", err))
		os.Exit(1)
	}

	giteaClient, err := gitea.NewClient(cfg.GiteaURL, cfg.GiteaToken, cfg.InsecureSkipTLSVerify)
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to connect to Gitea: %v", err))
		os.Exit(1)
	}

	// Verify connections
	glVersion, err := gitlabClient.GetVersion()
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to get GitLab version: %v", err))
		os.Exit(1)
	}
	utils.PrintInfo(fmt.Sprintf("Connected to GitLab, version: %s", glVersion))

	gtVersion, err := giteaClient.GetVersion()
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to get Gitea version: %v", err))
		os.Exit(1)
	}
	utils.PrintInfo(fmt.Sprintf("Connected to Gitea, version: %s", gtVersion))

	// Initialize migration manager
	migrationManager := migration.NewManager(gitlabClient, giteaClient, cfg)

	// Perform migration
	migrateWithErrorHandling(migrationManager)
}

func migrateWithErrorHandling(migrator *migration.Manager) {
	defer func() {
		if r := recover(); r != nil {
			utils.PrintError(fmt.Sprintf("Migration failed with panic: %v", r))
			utils.PrintWarning("You can resume the migration later using the state file.")
		}
	}()

	errCount := 0
	var err error

	utils.PrintHeader("Starting users and groups migration...")
	// Import users and groups
	err = migrator.ImportUsersGroups()
	if err != nil {
		errCount++
		utils.PrintError(fmt.Sprintf("Error during user and group migration: %v", err))
	}
	utils.PrintSuccess("Completed users and groups migration")

	utils.PrintHeader("Starting projects migration...")
	// Import projects
	err = migrator.ImportProjects()
	if err != nil {
		errCount++
		utils.PrintError(fmt.Sprintf("Error during project migration: %v", err))
	}
	utils.PrintSuccess("Completed projects migration")

	fmt.Println()
	if errCount == 0 {
		utils.PrintSuccess("Migration finished with no errors!")
	} else {
		utils.PrintError(fmt.Sprintf("Migration finished with %d errors!", errCount))
	}
}
