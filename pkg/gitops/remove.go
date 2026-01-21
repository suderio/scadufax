package gitops

import (
	"fmt"

	"github.com/go-git/go-git/v5"
)

// Remove deletes a file from the worktree.
// It does NOT commit the deletion. Use CommitFile for that.
func Remove(repoPath string, filePath string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Remove file (git rm)
	_, err = w.Remove(filePath)
	if err != nil {
		return fmt.Errorf("failed to remove file %s: %w", filePath, err)
	}

	return nil
}
