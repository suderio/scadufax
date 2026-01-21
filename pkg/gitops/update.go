package gitops

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
)

// Push pushes the current branch to origin.
func Push(repoPath string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	// We assume remote is 'origin'
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
	})
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}

// GetHeadID returns the SCADUFAX_ID from the HEAD commit message.
// Returns empty string if not found.
func GetHeadID(repoPath string) (string, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open repo: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", fmt.Errorf("failed to get commit object: %w", err)
	}

	// Parse message for "SCADUFAX_ID: <uuid>"
	// It should be in the last line or distinct line.
	lines := strings.Split(commit.Message, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "SCADUFAX_ID:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "SCADUFAX_ID:")), nil
		}
	}

	return "", nil
}
