// repositories.go

// Package migration handles the migration of data from GitLab to Gitea
package migration

import (
	"fmt"

	"github.com/xanzy/go-gitlab"

	"github.com/go-i2p/gitlab-to-gitea/utils"
)

// repositoryMigrateRequest is the payload for Gitea's POST /repos/migrate
// endpoint. With Service set to "gitlab", Gitea uses its native GitLab
// importer, which pulls the git data together with issues, merge requests,
// labels, milestones, releases, wiki and LFS objects.
type repositoryMigrateRequest struct {
	CloneAddr   string `json:"clone_addr"`
	Service     string `json:"service"`
	AuthToken   string `json:"auth_token,omitempty"`
	RepoOwner   string `json:"repo_owner"`
	RepoName    string `json:"repo_name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	Mirror      bool   `json:"mirror"`

	Issues       bool `json:"issues"`
	Labels       bool `json:"labels"`
	Milestones   bool `json:"milestones"`
	PullRequests bool `json:"pull_requests"`
	Releases     bool `json:"releases"`
	Wiki         bool `json:"wiki"`
	LFS          bool `json:"lfs"`
}

// ImportProject imports a GitLab project to Gitea using Gitea's native
// GitLab migration service, then maps project members to collaborators.
func (m *Manager) ImportProject(project *gitlab.Project) error {
	cleanName := utils.CleanName(project.Name)

	utils.PrintInfo(fmt.Sprintf("Importing project %s from owner %s", cleanName, project.Namespace.Name))

	// Get the owner information first, so we use the correct name format
	ownerInfo, err := m.getOwner(project)
	if err != nil {
		return fmt.Errorf("failed to get project owner: %w", err)
	}

	// Get the correct owner username from the result
	owner, ok := ownerInfo["username"].(string)
	if !ok || owner == "" {
		return fmt.Errorf("failed to get valid username for project owner")
	}

	utils.PrintInfo(fmt.Sprintf("Using owner %s for project %s", owner, cleanName))

	// Check if repository already exists
	if exists, err := m.repoExists(owner, cleanName); err != nil {
		return fmt.Errorf("failed to check if repository exists: %w", err)
	} else if exists {
		utils.PrintWarning(fmt.Sprintf("Project %s already exists in Gitea, skipping repository migration!", cleanName))
	} else {
		migrateReq := repositoryMigrateRequest{
			CloneAddr:   project.HTTPURLToRepo,
			Service:     "gitlab",
			AuthToken:   m.config.GitLabToken,
			RepoOwner:   owner,
			RepoName:    cleanName,
			Description: project.Description,
			Private:     project.Visibility == "private" || project.Visibility == "internal",
			Mirror:      false,

			Issues:       true,
			Labels:       true,
			Milestones:   true,
			PullRequests: true,
			Releases:     true,
			Wiki:         true,
			LFS:          true,
		}

		// The native migration runs synchronously and can take a long time
		// for large projects, so use a client with an extended timeout.
		var result map[string]interface{}
		err = m.giteaClient.WithTimeout(m.config.MigrationTimeout).Post("/repos/migrate", migrateReq, &result)
		if err != nil {
			return fmt.Errorf("failed to migrate repository %s: %w", cleanName, err)
		}

		utils.PrintInfo(fmt.Sprintf("Project %s imported!", cleanName))
	}

	// Gitea's migrator does not carry over project members, so map them to
	// collaborators ourselves.
	collaborators, err := m.gitlabClient.GetProjectMembers(project.ID)
	if err != nil {
		utils.PrintWarning(fmt.Sprintf("Error fetching collaborators for project %s: %v", project.Name, err))
	} else {
		utils.PrintInfo(fmt.Sprintf("Found %d collaborators for project %s", len(collaborators), cleanName))
		if err := m.importProjectCollaborators(collaborators, project); err != nil {
			utils.PrintWarning(fmt.Sprintf("Error importing collaborators: %v", err))
		}
	}

	return nil
}

// getOwner retrieves the user or organization info for a project
func (m *Manager) getOwner(project *gitlab.Project) (map[string]interface{}, error) {
	namespacePath := utils.NormalizeUsername(project.Namespace.Path)

	// Try to get as a user first
	var result map[string]interface{}
	err := m.giteaClient.Get("/users/"+namespacePath, &result)
	if err == nil && result != nil {
		// Verify required fields exist
		if username, ok := result["username"].(string); ok && username != "" {
			return result, nil
		}
	}

	// Try to get as an organization
	orgName := utils.CleanName(project.Namespace.Name)
	err = m.giteaClient.Get("/orgs/"+orgName, &result)
	if err == nil && result != nil {
		// Verify required fields exist
		if username, ok := result["username"].(string); ok && username != "" {
			return result, nil
		}
	}

	// Create a placeholder user instead of failing
	utils.PrintWarning(fmt.Sprintf("Could not find owner for project %s, creating placeholder user", project.Name))
	if err := m.ImportPlaceholderUser(namespacePath); err != nil {
		return nil, fmt.Errorf("failed to create placeholder user: %w", err)
	}

	// Try to get the newly created user
	err = m.giteaClient.Get("/users/"+namespacePath, &result)
	if err == nil && result != nil {
		return result, nil
	}

	return nil, fmt.Errorf("failed to find or create owner for project: %s", project.Path)
}

// repoExists checks if a repository exists in Gitea
func (m *Manager) repoExists(owner, repo string) (bool, error) {
	var repository map[string]interface{}
	err := m.giteaClient.Get(fmt.Sprintf("/repos/%s/%s", owner, repo), &repository)
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("error checking if repository exists: %w", err)
	}
	return true, nil
}

// ensureMentionedUsersExist makes sure all users mentioned in issues exist in Gitea
func (m *Manager) ensureMentionedUsersExist(issues []*gitlab.Issue) {
	mentionedUsers := make(map[string]struct{})

	// Extract mentions from issues
	for _, issue := range issues {
		if issue.Description != "" {
			for _, mention := range utils.ExtractUserMentions(issue.Description) {
				mentionedUsers[mention] = struct{}{}
			}
		}
	}

	// Create placeholder users for any missing mentioned users
	for username := range mentionedUsers {
		exists, err := m.userExists(utils.NormalizeUsername(username))
		if err != nil {
			utils.PrintWarning(fmt.Sprintf("Error checking if user %s exists: %v", username, err))
			continue
		}

		if !exists {
			if err := m.ImportPlaceholderUser(username); err != nil {
				utils.PrintWarning(fmt.Sprintf("Failed to create placeholder user %s: %v", username, err))
			}
		}
	}
}
