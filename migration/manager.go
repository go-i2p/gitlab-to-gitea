// manager.go

// Package migration handles the migration of data from GitLab to Gitea
package migration

import (
	"fmt"
	"os"

	"github.com/go-i2p/gitlab-to-gitea/utils"

	"github.com/go-i2p/gitlab-to-gitea/config"
	"github.com/go-i2p/gitlab-to-gitea/gitea"
	"github.com/go-i2p/gitlab-to-gitea/gitlab"

	gogitlab "github.com/xanzy/go-gitlab"
)

// Manager handles the migration process
type Manager struct {
	gitlabClient *gitlab.Client
	giteaClient  *gitea.Client
	config       *config.Config
	state        *State
}

func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// NewManager creates a new migration manager
func NewManager(gitlabClient *gitlab.Client, giteaClient *gitea.Client, cfg *config.Config) *Manager {
	// Initialize state
	state := NewState(cfg.MigrationStateFile)
	if FileExists(cfg.MigrationStateFile) && cfg.ResumeMigration {
		utils.PrintInfo("Resuming previous migration...")
		if err := state.Load(); err != nil {
			utils.PrintWarning(fmt.Sprintf("Could not load migration state: %v. Starting new migration.", err))
		}
	} else {
		utils.PrintInfo("Starting new migration...")
		if err := state.Reset(); err != nil {
			utils.PrintWarning(fmt.Sprintf("Could not reset migration state: %v", err))
		}
		utils.PrintInfo("Migration state reset.")
	}
	utils.PrintInfo("Migration state initialized.")

	return &Manager{
		gitlabClient: gitlabClient,
		giteaClient:  giteaClient,
		config:       cfg,
		state:        state,
	}
}

// ImportUsersGroups imports users and groups from GitLab to Gitea
func (m *Manager) ImportUsersGroups() error {
	utils.PrintInfo("Fetching users from GitLab...")
	// Get GitLab users
	users, err := m.gitlabClient.ListUsers()
	if err != nil {
		return fmt.Errorf("failed to list GitLab users: %w", err)
	}
	utils.PrintInfo(fmt.Sprintf("Found %d GitLab users", len(users)))

	utils.PrintInfo("Fetching groups from GitLab...")
	// Get GitLab groups
	groups, err := m.gitlabClient.ListGroups()
	if err != nil {
		return fmt.Errorf("failed to list GitLab groups: %w", err)
	}
	utils.PrintInfo(fmt.Sprintf("Found %d GitLab groups", len(groups)))

	utils.PrintHeader("Importing users")
	// Import users
	for _, user := range users {
		utils.PrintInfo(fmt.Sprintf("Importing user %s...", user.Username))
		if m.config.ResumeMigration && m.state.HasImportedUser(user.Username) {
			utils.PrintWarning(fmt.Sprintf("User %s already imported, skipping!", user.Username))
			continue
		}

		if err := m.ImportUser(user, false); err != nil {
			utils.PrintError(fmt.Sprintf("Failed to import user %s: %v", user.Username, err))
			continue
		}

		m.state.MarkUserImported(user.Username)
		if err := m.state.Save(); err != nil {
			utils.PrintWarning(fmt.Sprintf("Failed to save migration state: %v", err))
		}
		utils.PrintSuccess(fmt.Sprintf("Imported user %s.", user.Username))
	}

	utils.PrintHeader("Importing groups")
	// Import groups
	for _, group := range groups {
		cleanName := utils.CleanName(group.Name)
		utils.PrintInfo(fmt.Sprintf("Importing group: %s...", cleanName))
		if m.config.ResumeMigration && m.state.HasImportedGroup(cleanName) {
			utils.PrintWarning(fmt.Sprintf("Group %s already imported, skipping!", cleanName))
			continue
		}

		if err := m.ImportGroup(group); err != nil {
			utils.PrintError(fmt.Sprintf("Failed to import group %s: %v", group.Name, err))
			continue
		}

		m.state.MarkGroupImported(cleanName)
		if err := m.state.Save(); err != nil {
			utils.PrintWarning(fmt.Sprintf("Failed to save migration state: %v", err))
		}
		utils.PrintSuccess(fmt.Sprintf("Imported group: %s.", cleanName))
	}

	return nil
}

// ImportProjects imports projects from GitLab to Gitea
func (m *Manager) ImportProjects() error {
	// Get GitLab projects
	projects, err := m.gitlabClient.ListProjects()
	if err != nil {
		return fmt.Errorf("failed to list GitLab projects: %w", err)
	}
	utils.PrintInfo(fmt.Sprintf("Found %d GitLab projects", len(projects)))

	// Import projects
	utils.PrintInfo("Pre-creating all necessary users for project migration...")

	// Create a set of all usernames and namespaces that need to exist
	requiredUsers := m.collectRequiredUsers(projects)

	// Create any missing users
	utils.PrintInfo(fmt.Sprintf("Found %d users that need to exist in Gitea", len(requiredUsers)))
	for username := range requiredUsers {
		exists, err := m.userExists(utils.NormalizeUsername(username))
		if err != nil {
			utils.PrintWarning(fmt.Sprintf("Error checking if user exists: %v", err))
			continue
		}

		if !exists {
			if err := m.ImportPlaceholderUser(username); err != nil {
				utils.PrintWarning(fmt.Sprintf("Failed to create placeholder user: %v", err))
			}
		}
	}

	utils.PrintInfo("Starting project migration...")

	// Import projects
	for _, project := range projects {
		projectKey := fmt.Sprintf("%s/%s", project.Namespace.Name, utils.CleanName(project.Name))

		// Skip if project was already fully imported
		if m.config.ResumeMigration && m.state.HasImportedProject(projectKey) {
			utils.PrintWarning(fmt.Sprintf("Project %s already imported, skipping!", projectKey))
			continue
		}

		// Import project
		if err := m.ImportProject(project); err != nil {
			utils.PrintError(fmt.Sprintf("Failed to import project %s: %v", project.Name, err))
			continue
		}

		m.state.MarkProjectImported(projectKey)
		if err := m.state.Save(); err != nil {
			utils.PrintWarning(fmt.Sprintf("Failed to save migration state: %v", err))
		}
	}

	return nil
}

// collectRequiredUsers builds a set of usernames that need to exist before project migration
func (m *Manager) collectRequiredUsers(projects []*gogitlab.Project) map[string]struct{} {
	required := make(map[string]struct{})

	utils.PrintHeader("Collecting required users for project migration")

	// Helper function to add a user to the required map if not already present
	addUser := func(username string) {
		if username == "" {
			return
		}
		if _, exists := required[username]; !exists {
			required[username] = struct{}{}
			utils.PrintInfo(fmt.Sprintf("Adding required user: %s", username))
		}
	}

	// Collect users from projects
	for _, project := range projects {
		utils.PrintInfo(fmt.Sprintf("Collecting users for project %s...", project.Name))

		// Add project namespace/owner if it's a user
		if project.Namespace.Kind == "user" {
			addUser(project.Namespace.Path)
		}

		// Collect project members
		members, err := m.gitlabClient.GetProjectMembers(project.ID)
		if err != nil {
			utils.PrintWarning(fmt.Sprintf("Error collecting members for %s: %v", project.Name, err))
			continue
		}

		for _, member := range members {
			addUser(member.Username)
		}

		// Issue/merge request authors are not collected here: Gitea's native
		// GitLab importer records them as original authors on its own.
	}

	utils.PrintInfo(fmt.Sprintf("Collected a total of %d unique required users", len(required)))
	return required
}
