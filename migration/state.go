// state.go

// Package migration handles the migration of data from GitLab to Gitea
package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/go-i2p/gitlab-to-gitea/utils"
)

// State manages the migration state to support resuming migrations
type State struct {
	filePath         string
	Users            []string            `json:"users"`
	Groups           []string            `json:"groups"`
	Projects         []string            `json:"projects"`
	ImportedComments map[string][]string `json:"imported_comments"`
	mutex            sync.RWMutex
}

// NewState creates a new migration state manager
func NewState(filePath string) *State {
	return &State{
		filePath:         filePath,
		Users:            []string{},
		Groups:           []string{},
		Projects:         []string{},
		ImportedComments: map[string][]string{},
	}
}

// Load loads the migration state from the file
func (s *State) Load() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read state file: %w", err)
	}

	err = json.Unmarshal(data, s)
	if err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	return nil
}

// Save saves the current migration state to the file
func (s *State) Save() error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.saveLocked()
}

// saveLocked writes the state to disk. The caller must hold s.mutex
// (read or write); sync.RWMutex is not reentrant, so this must never
// acquire the lock itself.
func (s *State) saveLocked() error {
	utils.PrintInfo("Saving migration state...")

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	err = os.WriteFile(s.filePath, data, 0o644)
	if err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	utils.PrintInfo("Migration state saved successfully")
	return nil
}

// Reset clears the migration state
func (s *State) Reset() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	utils.PrintInfo("Clearing migration state...")

	s.Users = []string{}
	s.Groups = []string{}
	s.Projects = []string{}
	s.ImportedComments = map[string][]string{}

	utils.PrintInfo("Migration state reset. Saving...")
	return s.saveLocked()
}

// HasImportedUser checks if a user has been imported
func (s *State) HasImportedUser(username string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, u := range s.Users {
		if u == username {
			return true
		}
	}
	return false
}

// MarkUserImported marks a user as imported
func (s *State) MarkUserImported(username string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check directly without calling HasImportedUser
	alreadyImported := false
	for _, u := range s.Users {
		if u == username {
			alreadyImported = true
			break
		}
	}

	if !alreadyImported {
		s.Users = append(s.Users, username)
	}
}

// HasImportedGroup checks if a group has been imported
func (s *State) HasImportedGroup(group string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, g := range s.Groups {
		if g == group {
			return true
		}
	}
	return false
}

// MarkGroupImported marks a group as imported
func (s *State) MarkGroupImported(group string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check directly without calling HasImportedGroup
	alreadyImported := false
	for _, g := range s.Groups {
		if g == group {
			alreadyImported = true
			break
		}
	}

	if !alreadyImported {
		s.Groups = append(s.Groups, group)
	}
}

// HasImportedProject checks if a project has been imported
func (s *State) HasImportedProject(project string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, p := range s.Projects {
		if p == project {
			return true
		}
	}
	return false
}

func (s *State) MarkProjectImported(project string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check directly without calling HasImportedProject
	alreadyImported := false
	for _, p := range s.Projects {
		if p == project {
			alreadyImported = true
			break
		}
	}

	if !alreadyImported {
		s.Projects = append(s.Projects, project)
	}
}

// HasImportedComment checks if a comment has been imported
func (s *State) HasImportedComment(issueKey, commentID string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	comments, exists := s.ImportedComments[issueKey]
	if !exists {
		return false
	}

	for _, id := range comments {
		if id == commentID {
			return true
		}
	}
	return false
}

// MarkCommentImported marks a comment as imported
func (s *State) MarkCommentImported(issueKey, commentID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.ImportedComments[issueKey]; !exists {
		s.ImportedComments[issueKey] = []string{}
	}

	// Check directly without calling HasImportedComment
	alreadyImported := false
	comments, exists := s.ImportedComments[issueKey]
	if exists {
		for _, id := range comments {
			if id == commentID {
				alreadyImported = true
				break
			}
		}
	}

	if !alreadyImported {
		s.ImportedComments[issueKey] = append(s.ImportedComments[issueKey], commentID)
	}
}
