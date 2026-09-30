package main

import (
	"fmt"
	"os"

	"github.com/go-i2p/gitlab-to-gitea/config"
	"github.com/go-i2p/gitlab-to-gitea/gitea"
	"github.com/go-i2p/gitlab-to-gitea/utils"
)

func main() {
	utils.PrintHeader("---=== Gitea Unmigration Tool ===---")
	fmt.Println("This tool will remove all entities from Gitea except the admin user.\n")

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

	// Get current user (admin)
	var currentUser map[string]interface{}
	err = giteaClient.Get("/api/v1/user", &currentUser)
	if err != nil {
		utils.PrintError(fmt.Sprintf("Failed to get current user: %v", err))
		os.Exit(1)
	}
	adminUsername := currentUser["login"].(string)
	utils.PrintInfo(fmt.Sprintf("Logged in as: %s", adminUsername))

	// Confirm deletion
	utils.PrintWarning("WARNING: This will delete ALL repositories, organizations, and users (except admin).")
	utils.PrintWarning("This operation CANNOT be undone!")
	fmt.Print("Type 'yes' to continue: ")
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "yes" {
		utils.PrintInfo("Operation cancelled.")
		return
	}

	// Start unmigration process
	unmigrator := NewUnmigrator(giteaClient, adminUsername)
	if err := unmigrator.Run(); err != nil {
		utils.PrintError(fmt.Sprintf("Unmigration failed: %v", err))
		os.Exit(1)
	}

	utils.PrintSuccess("Unmigration completed successfully.")
}

// Unmigrator handles the clean-up of Gitea entities
type Unmigrator struct {
	client        *gitea.Client
	adminUsername string
}

// NewUnmigrator creates a new Unmigrator instance
func NewUnmigrator(client *gitea.Client, adminUsername string) *Unmigrator {
	return &Unmigrator{
		client:        client,
		adminUsername: adminUsername,
	}
}

// Run executes the unmigration process
func (u *Unmigrator) Run() error {
	utils.PrintHeader("Starting unmigration process...")

	// Delete all repositories first (this will also delete issues, milestones, comments, etc.)
	if err := u.deleteAllRepositories(); err != nil {
		return fmt.Errorf("failed to delete repositories: %w", err)
	}

	// Delete all organizations
	if err := u.deleteAllOrganizations(); err != nil {
		return fmt.Errorf("failed to delete organizations: %w", err)
	}

	// Delete all non-admin users
	if err := u.deleteAllNonAdminUsers(); err != nil {
		return fmt.Errorf("failed to delete users: %w", err)
	}

	return nil
}

// deleteAllRepositories deletes all repositories in Gitea
func (u *Unmigrator) deleteAllRepositories() error {
	utils.PrintHeader("Deleting repositories...")

	// Get all repositories using the search function
	repos, err := u.client.SearchRepositories()
	if err != nil {
		return fmt.Errorf("failed to get repositories: %w", err)
	}

	utils.PrintInfo(fmt.Sprintf("Found %d repositories to delete", len(repos)))

	// Delete each repository
	for _, repo := range repos {
		fullName, ok := repo["full_name"].(string)
		if !ok {
			utils.PrintWarning("Could not get repository name, skipping")
			continue
		}

		utils.PrintInfo(fmt.Sprintf("Deleting repository: %s", fullName))
		err := u.client.Delete("/repos/" + fullName)
		if err != nil {
			utils.PrintWarning(fmt.Sprintf("Failed to delete repository %s: %v", fullName, err))
		} else {
			utils.PrintSuccess(fmt.Sprintf("Repository %s deleted", fullName))
		}
	}

	utils.PrintSuccess("Repository deletion complete")
	return nil
}

// deleteAllOrganizations deletes all organizations in Gitea
func (u *Unmigrator) deleteAllOrganizations() error {
	utils.PrintHeader("Deleting organizations...")

	// Get all organizations
	var orgs []map[string]interface{}
	err := u.client.Get("/orgs", &orgs)
	if err != nil {
		return fmt.Errorf("failed to get organizations: %w", err)
	}

	utils.PrintInfo(fmt.Sprintf("Found %d organizations to delete", len(orgs)))

	// Delete each organization
	for _, org := range orgs {
		orgName, ok := org["username"].(string)
		if !ok {
			utils.PrintWarning("Could not get organization name, skipping")
			continue
		}

		utils.PrintInfo(fmt.Sprintf("Deleting organization: %s", orgName))
		err := u.client.Delete("/orgs/" + orgName)
		if err != nil {
			utils.PrintWarning(fmt.Sprintf("Failed to delete organization %s: %v", orgName, err))
		} else {
			utils.PrintSuccess(fmt.Sprintf("Organization %s deleted", orgName))
		}
	}

	utils.PrintSuccess("Organization deletion complete")
	return nil
}

// deleteAllNonAdminUsers deletes all users except admins and the current user
func (u *Unmigrator) deleteAllNonAdminUsers() error {
	utils.PrintHeader("Deleting users...")

	// Get all users
	var users []map[string]interface{}
	err := u.client.Get("admin/users?limit=1000", &users)
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	// Count users to delete and preserve
	preserveCount := 0
	deleteCount := 0

	for _, user := range users {
		username, ok := user["login"].(string)
		if !ok {
			continue
		}

		isAdmin, _ := user["is_admin"].(bool)
		isCurrentUser := username == u.adminUsername

		if isAdmin || isCurrentUser {
			preserveCount++
		} else {
			deleteCount++
		}
	}

	utils.PrintInfo(fmt.Sprintf("Found %d users to preserve (admins or current user)", preserveCount))
	utils.PrintInfo(fmt.Sprintf("Found %d users to delete", deleteCount))

	// Delete each non-admin, non-current user
	deletedCount := 0
	failedCount := 0

	for _, user := range users {
		username, ok := user["login"].(string)
		if !ok {
			utils.PrintWarning("Could not get username, skipping")
			continue
		}

		// Check if the user is an admin or the current user
		isAdmin, _ := user["is_admin"].(bool)
		isCurrentUser := username == u.adminUsername

		// Skip if user is an admin OR is the authenticated user
		if isAdmin || isCurrentUser {
			reason := ""
			if isAdmin {
				reason += "admin"
			}
			if isCurrentUser {
				if reason != "" {
					reason += ", "
				}
				reason += "current user"
			}
			utils.PrintInfo(fmt.Sprintf("Skipping user: %s (%s)", username, reason))
			continue
		}

		// Try deleting by username instead of ID
		utils.PrintInfo(fmt.Sprintf("Deleting user: %s", username))
		err := u.client.Delete(fmt.Sprintf("admin/users/%s", username))
		if err != nil {
			// If that fails, try with the ID as fallback
			userID, ok := user["id"].(float64)
			if ok {
				utils.PrintInfo(fmt.Sprintf("Retrying deletion with ID: %d", int(userID)))
				err = u.client.Delete(fmt.Sprintf("admin/users/%d", int(userID)))
				if err != nil {
					utils.PrintWarning(fmt.Sprintf("Failed to delete user %s: %v", username, err))
					failedCount++
					continue
				}
			} else {
				utils.PrintWarning(fmt.Sprintf("Failed to delete user %s and could not get ID", username))
				failedCount++
				continue
			}
		}

		utils.PrintSuccess(fmt.Sprintf("User %s deleted", username))
		deletedCount++
	}

	utils.PrintSuccess(fmt.Sprintf("User deletion complete. Deleted: %d, Failed: %d", deletedCount, failedCount))
	return nil
}
