package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveCommand_Integration(t *testing.T) {
	// Setup
	rootDir := setupTestDir(t)
	homeDir := filepath.Join(rootDir, "home")
	localDir := filepath.Join(rootDir, "local")

	err := os.MkdirAll(homeDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(localDir, 0755)
	require.NoError(t, err)

	// Init Repo
	repo, err := git.PlainInit(localDir, false)
	require.NoError(t, err)
	w, _ := repo.Worktree()

	// Initial Commit (with dummy content to have main)
	dummyFile := filepath.Join(localDir, "README.md")
	os.WriteFile(dummyFile, []byte("init"), 0644)
	w.Add("README.md")
	w.Commit("Init", &git.CommitOptions{Author: &object.Signature{Name: "T", Email: "t", When: time.Now()}})

	// Ensure main ref exists
	headRef, _ := repo.Head()
	mainRef := plumbing.NewHashReference("refs/heads/main", headRef.Hash())
	repo.Storer.SetReference(mainRef)

	viper.Reset()
	viper.Set("scadufax.local_dir", localDir)
	viper.Set("scadufax.home_dir", homeDir)

	t.Run("Remove Repo Only", func(t *testing.T) {
		// Create file in Repo and Home
		fName := ".config/app/conf.toml"
		repoPath := filepath.Join(localDir, fName)
		homePath := filepath.Join(homeDir, fName)

		os.MkdirAll(filepath.Dir(repoPath), 0755)
		os.MkdirAll(filepath.Dir(homePath), 0755)

		os.WriteFile(repoPath, []byte("repo"), 0644)
		os.WriteFile(homePath, []byte("home"), 0644)

		w.Add(fName)
		w.Commit("Add file", &git.CommitOptions{Author: &object.Signature{Name: "T", Email: "t", When: time.Now()}})

		// Remove
		cmd := rootCmd
		removeLocal = false // Ensure flag reset
		// removeCmd.Flags().Set("local", "false") // Not needed if we reset VAR, but safe to check
		cmd.SetArgs([]string{"remove", homePath})
		err := cmd.Execute()
		require.NoError(t, err)

		// Verify Repo (Deleted)
		_, err = os.Stat(repoPath)
		assert.True(t, os.IsNotExist(err))

		// Verify Repo Status (Committed deletion)
		status, _ := w.Status()
		assert.True(t, status.IsClean())

		// Verify Home (Exists)
		_, err = os.Stat(homePath)
		assert.NoError(t, err)
	})

	t.Run("Remove Local with No Confirmation (Default True, Input No)", func(t *testing.T) {
		// Mock Stdin?
		// We can pipe stdin
		r, wPipe, _ := os.Pipe()
		oldStdin := os.Stdin
		defer func() { os.Stdin = oldStdin }()
		os.Stdin = r

		// Write "n"
		wPipe.Write([]byte("n\n"))
		wPipe.Close()

		fName := ".config/app/conf2.toml"
		repoPath := filepath.Join(localDir, fName)
		homePath := filepath.Join(homeDir, fName)
		os.MkdirAll(filepath.Dir(repoPath), 0755)
		os.MkdirAll(filepath.Dir(homePath), 0755)
		os.WriteFile(repoPath, []byte("repo"), 0644)
		os.WriteFile(homePath, []byte("home"), 0644)

		w.Add(fName)
		w.Commit("Add file 2", &git.CommitOptions{Author: &object.Signature{Name: "T", Email: "t", When: time.Now()}})

		// viper confirm not set -> defaults true logic
		viper.Set("scadufax.confirm", nil) // unset

		removeLocal = true
		defer func() { removeLocal = false }()

		cmd := rootCmd
		cmd.SetArgs([]string{"remove", "--local", homePath})
		err := cmd.Execute()
		require.NoError(t, err)

		// Verify Repo (Deleted)
		_, err = os.Stat(repoPath)
		assert.True(t, os.IsNotExist(err))

		// Verify Home (Exists - Skipped)
		_, err = os.Stat(homePath)
		assert.NoError(t, err)
	})

	t.Run("Remove Local with No Confirm Config", func(t *testing.T) {
		fName := ".config/app/conf3.toml"
		repoPath := filepath.Join(localDir, fName)
		homePath := filepath.Join(homeDir, fName)
		os.MkdirAll(filepath.Dir(repoPath), 0755)
		os.MkdirAll(filepath.Dir(homePath), 0755)
		os.WriteFile(repoPath, []byte("repo"), 0644)
		os.WriteFile(homePath, []byte("home"), 0644)

		w.Add(fName)
		w.Commit("Add file 3", &git.CommitOptions{Author: &object.Signature{Name: "T", Email: "t", When: time.Now()}})

		viper.Set("scadufax.confirm", false)

		removeLocal = true
		defer func() { removeLocal = false }()

		cmd := rootCmd
		cmd.SetArgs([]string{"remove", "--local", homePath})
		err := cmd.Execute()
		require.NoError(t, err)

		// Verify Home (Deleted)
		_, err = os.Stat(homePath)
		assert.True(t, os.IsNotExist(err))
	})
}
