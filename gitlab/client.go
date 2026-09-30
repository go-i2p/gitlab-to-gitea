// client.go

// Package gitlab provides a client for interacting with the GitLab API
package gitlab

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/xanzy/go-gitlab"
)

// Client wraps the GitLab client for custom functionality
type Client struct {
	client *gitlab.Client
}

// NewClient creates a new GitLab client with the provided URL and token.
// When insecureSkipTLSVerify is true, TLS certificates presented by the
// GitLab server are not verified.
func NewClient(url, token string, insecureSkipTLSVerify bool) (*Client, error) {
	opts := []gitlab.ClientOptionFunc{gitlab.WithBaseURL(url)}
	if insecureSkipTLSVerify {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // opt-in via config
		}
		opts = append(opts, gitlab.WithHTTPClient(&http.Client{Transport: transport}))
	}

	client, err := gitlab.NewClient(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	return &Client{
		client: client,
	}, nil
}

// GetVersion retrieves the GitLab version
func (c *Client) GetVersion() (string, error) {
	v, _, err := c.client.Version.GetVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get GitLab version: %w", err)
	}
	return v.Version, nil
}

// GetCurrentUser retrieves information about the current authenticated user
func (c *Client) GetCurrentUser() (*gitlab.User, error) {
	user, _, err := c.client.Users.CurrentUser()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	return user, nil
}

// ListUsers returns all users in the GitLab instance
func (c *Client) ListUsers() ([]*gitlab.User, error) {
	opts := &gitlab.ListUsersOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var allUsers []*gitlab.User
	for {
		users, resp, err := c.client.Users.ListUsers(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list users: %w", err)
		}
		allUsers = append(allUsers, users...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allUsers, nil
}

// ListGroups returns all groups in the GitLab instance
func (c *Client) ListGroups() ([]*gitlab.Group, error) {
	opts := &gitlab.ListGroupsOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var allGroups []*gitlab.Group
	for {
		groups, resp, err := c.client.Groups.ListGroups(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list groups: %w", err)
		}
		allGroups = append(allGroups, groups...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allGroups, nil
}

// ListProjects returns all projects in the GitLab instance
func (c *Client) ListProjects() ([]*gitlab.Project, error) {
	opts := &gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var allProjects []*gitlab.Project
	for {
		projects, resp, err := c.client.Projects.ListProjects(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list projects: %w", err)
		}
		allProjects = append(allProjects, projects...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allProjects, nil
}

// GetProjectMembers returns all members of a project
func (c *Client) GetProjectMembers(projectID int) ([]*gitlab.ProjectMember, error) {
	opts := &gitlab.ListProjectMembersOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var allMembers []*gitlab.ProjectMember
	for {
		members, resp, err := c.client.ProjectMembers.ListProjectMembers(projectID, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list project members: %w", err)
		}
		allMembers = append(allMembers, members...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allMembers, nil
}

// GetProjectLabels returns all labels of a project
func (c *Client) GetProjectLabels(projectID int) ([]*gitlab.Label, error) {
	opts := &gitlab.ListLabelsOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var allLabels []*gitlab.Label
	for {
		labels, resp, err := c.client.Labels.ListLabels(projectID, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list project labels: %w", err)
		}
		allLabels = append(allLabels, labels...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allLabels, nil
}

// GetProjectMilestones returns all milestones of a project
func (c *Client) GetProjectMilestones(projectID int) ([]*gitlab.Milestone, error) {
	opts := &gitlab.ListMilestonesOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var allMilestones []*gitlab.Milestone
	for {
		milestones, resp, err := c.client.Milestones.ListMilestones(projectID, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list project milestones: %w", err)
		}
		allMilestones = append(allMilestones, milestones...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allMilestones, nil
}

// GetProjectIssues returns all issues of a project
func (c *Client) GetProjectIssues(projectID int) ([]*gitlab.Issue, error) {
	opts := &gitlab.ListProjectIssuesOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var allIssues []*gitlab.Issue
	for {
		issues, resp, err := c.client.Issues.ListProjectIssues(projectID, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list project issues: %w", err)
		}
		allIssues = append(allIssues, issues...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allIssues, nil
}

// GetIssueNotes returns all notes of an issue
func (c *Client) GetIssueNotes(projectID, issueID int) ([]*gitlab.Note, error) {
	opts := &gitlab.ListIssueNotesOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var allNotes []*gitlab.Note
	for {
		notes, resp, err := c.client.Notes.ListIssueNotes(projectID, issueID, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list issue notes: %w", err)
		}
		allNotes = append(allNotes, notes...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allNotes, nil
}

// GetGroupMembers returns all members of a group
func (c *Client) GetGroupMembers(groupID int) ([]*gitlab.GroupMember, error) {
	opts := &gitlab.ListGroupMembersOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	var allMembers []*gitlab.GroupMember
	for {
		members, resp, err := c.client.Groups.ListGroupMembers(groupID, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list group members: %w", err)
		}
		allMembers = append(allMembers, members...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allMembers, nil
}

// GetUserKeys returns all SSH keys of a user
func (c *Client) GetUserKeys(userID int) ([]*gitlab.SSHKey, error) {
	opts := &gitlab.ListSSHKeysForUserOptions{
		PerPage: 100,
	}

	var allKeys []*gitlab.SSHKey
	for {
		keys, resp, err := c.client.Users.ListSSHKeysForUser(userID, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list user SSH keys: %w", err)
		}
		allKeys = append(allKeys, keys...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return allKeys, nil
}
