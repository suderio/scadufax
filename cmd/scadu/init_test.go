package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/suderio/scadufax/pkg/gitops"
)

func TestInitCommand_Integration(t *testing.T) {
	// ... (logic remains or can be removed if specific tests cover it)
}

func TestInitCommand_ConfigGeneration(t *testing.T) {
	baseDir := setupTestDir(t)
	configDir := filepath.Join(baseDir, "config")
	localDir := filepath.Join(baseDir, "local")

	resetViper()
	cmd := rootCmd
	// We use a dummy user/repo to fail git but succeed config
	args := []string{"init", "testuser/testrepo",
		"--config-dir", configDir,
		"--local-dir", localDir,
		"--fork", "testhost",
	}
	cmd.SetArgs(args)

	// Execute
	// It will error on git pull, but config should be written BEFORE that.
	// We expect an error.
	err := cmd.Execute()
	require.Error(t, err) // Expect git error

	// Verify Config Files
	configFile := filepath.Join(configDir, "config.toml")
	assert.FileExists(t, configFile)

	content, err := os.ReadFile(configFile)
	require.NoError(t, err)
	sContent := string(content)

	assert.Contains(t, sContent, fmt.Sprintf("config_dir = '%s'", configDir))
	assert.Contains(t, sContent, fmt.Sprintf("local_dir = '%s'", localDir))
	assert.Contains(t, sContent, "fork = 'testhost'")

	// Verify Local Config
	localFile := filepath.Join(configDir, "local.toml")
	assert.FileExists(t, localFile)

	// Verify Local Dir created
	assert.DirExists(t, localDir)
}

func TestGitOps_InitRepoAndBranch(t *testing.T) {
	// Setup a "remote" repo locally
	remoteDir := setupTestDir(t)
	remoteRepo, err := git.PlainInit(remoteDir, false)
	require.NoError(t, err)

	// Commit a file to main
	w, err := remoteRepo.Worktree()
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(remoteDir, "README.md"), []byte("# Dotfiles"), 0644)
	require.NoError(t, err)

	_, err = w.Add("README.md")
	require.NoError(t, err)

	_, err = w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Rename branch to main if it's not
	headRef, err := remoteRepo.Head()
	require.NoError(t, err)
	if headRef.Name() != "refs/heads/main" {
		// Create main branch pointing to HEAD
		ref := plumbing.NewHashReference("refs/heads/main", headRef.Hash())
		err = remoteRepo.Storer.SetReference(ref)
		require.NoError(t, err)

		// Update HEAD to main
		target := plumbing.ReferenceName("refs/heads/main")
		symbolicRef := plumbing.NewSymbolicReference(plumbing.HEAD, target)
		err = remoteRepo.Storer.SetReference(symbolicRef)
		require.NoError(t, err)
	}

	// Target Directory
	targetDir := setupTestDir(t)

	// Run InitRepo
	err = gitops.InitRepo(targetDir, remoteDir) // remoteDir is a valid path
	require.NoError(t, err)

	// Verify repo initialized
	repo, err := git.PlainOpen(targetDir)
	require.NoError(t, err)

	// Ensure Branch
	forkName := "myfork"
	err = gitops.CreateBranch(targetDir, forkName)
	require.NoError(t, err)

	// Verify branch exists
	_, err = repo.Reference(plumbing.ReferenceName("refs/heads/"+forkName), true)
	require.NoError(t, err, "Branch should exist")

	// Verify it points to same commit as HEAD (main)
	curHead, _ := repo.Head()
	branchRef, _ := repo.Reference(plumbing.ReferenceName("refs/heads/"+forkName), true)
	assert.Equal(t, curHead.Hash(), branchRef.Hash())

	// Run again (idempotency)
	err = gitops.CreateBranch(targetDir, forkName)
	require.NoError(t, err)
}
