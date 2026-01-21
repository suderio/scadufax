package gitops

import (
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// InitRepo initializes a new git repository at the given path, adds a remote, and pulls the main branch.
// It assumes the directory exists (or allows go-git to create it).
func InitRepo(path string, remoteURL string) error {
	// 1. git init
	repo, err := git.PlainInit(path, false)
	if err != nil {
		// If it's already a repo, we might want to proceed or fail.
		// "scadu init" usually implies fresh start.
		// If err is 'repository already exists', maybe we just open it?
		if err == git.ErrRepositoryAlreadyExists {
			repo, err = git.PlainOpen(path)
			if err != nil {
				return fmt.Errorf("failed to open existing repo at %s: %w", path, err)
			}
		} else {
			return fmt.Errorf("failed to git init at %s: %w", path, err)
		}
	}

	// 2. git remote add origin
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	if err != nil {
		if err != git.ErrRemoteExists {
			return fmt.Errorf("failed to add remote origin: %w", err)
		}
		// If remote exists, we ignore or update? Let's assume ignore for now as safe default.
	}

	// 3. git pull origin main
	// We need a worktree to pull.
	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Pull
	err = w.Pull(&git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: "refs/heads/main",
	})
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		// If empty repo (no main branch yet on remote), this might fail.
		// But requirement says "git pull origin main".
		// We can return error or wrap it.
		return fmt.Errorf("failed to pull origin main: %w", err)
	}

	return nil
}

// CreateBranch creates a new branch with the given name pointing to HEAD.
// If the branch already exists, it does nothing and returns nil.
func CreateBranch(path string, branchName string) error {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("failed to open repo at %s: %w", path, err)
	}

	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Check if branch exists
	branchRefName := "refs/heads/" + branchName
	_, err = repo.Reference(plumbing.ReferenceName(branchRefName), true)
	if err == nil {
		// Exists
		return nil
	}

	// Create branch
	ref := plumbing.NewHashReference(plumbing.ReferenceName(branchRefName), headRef.Hash())
	err = repo.Storer.SetReference(ref)
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}

	return nil
}

// IsDirty checks if the given path (or worktree if path is ".") has uncommitted changes.
func IsDirty(repoPath string, targetPath string) (bool, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return false, fmt.Errorf("failed to open repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get status: %w", err)
	}

	if targetPath == "." {
		return !status.IsClean(), nil
	}

	// Check specific file status
	FileStatus := status.File(targetPath)
	if FileStatus.Worktree == git.Unmodified {
		return false, nil
	}

	return true, nil
}

// CommitFile stages a specific file and commits it with the message.
func CommitFile(repoPath string, filePath string, message string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add file
	// filePath must be relative to worktree root
	_, err = w.Add(filePath)
	if err != nil {
		return fmt.Errorf("failed to add file %s: %w", filePath, err)
	}

	// Commit
	_, err = w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Scadu Tool",
			Email: "scadu@local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// Checkout switches the repo to the specified branch.
// It tries to find local branch, if not found tries to find remote branch and create local tracking branch.
func Checkout(repoPath string, branchName string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Try checkout local
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName("refs/heads/" + branchName),
	})
	if err == nil {
		return nil
	}

	// Check if remote branch exists
	remoteRefName := "refs/remotes/origin/" + branchName
	_, err = repo.Reference(plumbing.ReferenceName(remoteRefName), true)
	if err != nil {
		return fmt.Errorf("branch %s not found locally or on remote: %w", branchName, err)
	}

	// Create local branch tracking remote
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName("refs/heads/" + branchName),
		Create: true,
	})
	if err != nil {
		return fmt.Errorf("failed to checkout new branch %s: %w", branchName, err)
	}

	return nil
}

// Pull updates the current branch from its upstream (assumed origin).
func Pull(repoPath string) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Pull
	// We assume remote is 'origin'
	err = w.Pull(&git.PullOptions{
		RemoteName: "origin",
	})
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		return fmt.Errorf("failed to pull: %w", err)
	}

	return nil
}
