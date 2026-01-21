package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/suderio/scadufax/pkg/gitops"
)

func TestAddCommand_Integration(t *testing.T) {
	// Setup
	rootDir := setupTestDir(t)
	homeDir := filepath.Join(rootDir, "home")
	localDir := filepath.Join(rootDir, "local")

	err := os.MkdirAll(homeDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(localDir, 0755)
	require.NoError(t, err)

	gitops.InitRepo(localDir, "git@github.com:test/repo.git")
	// We need main branch commit to avoid empty repo issues if checkout happens
	repo, _ := git.PlainOpen(localDir)
	w, _ := repo.Worktree()

	// Create dummy file for init commit
	dummyFile := filepath.Join(localDir, "README.md")
	err = os.WriteFile(dummyFile, []byte("repo"), 0644)
	require.NoError(t, err)
	_, err = w.Add("README.md")
	require.NoError(t, err)

	_, err = w.Commit("Init", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@local", When: time.Now()},
	})
	require.NoError(t, err)

	// Explicitly create main branch if it doesn't exist (go-git init might default to master)
	headRef, err := repo.Head()
	require.NoError(t, err)
	// Force create refs/heads/main pointing to HEAD
	mainRef := plumbing.NewHashReference("refs/heads/main", headRef.Hash())
	err = repo.Storer.SetReference(mainRef)
	require.NoError(t, err)

	viper.Reset()
	viper.Set("scadufax.local_dir", localDir)
	viper.Set("scadufax.home_dir", homeDir)

	t.Run("Basic Add", func(t *testing.T) {
		// Create file in home
		fName := ".config/newapp/config.toml"
		fPath := filepath.Join(homeDir, fName)
		err := os.MkdirAll(filepath.Dir(fPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fPath, []byte("theme = \"dark\""), 0644)
		require.NoError(t, err)

		// Run Add
		cmd := rootCmd
		cmd.SetArgs([]string{"add", fPath})
		err = cmd.Execute()
		require.NoError(t, err)

		// Verify Repo
		repoPath := filepath.Join(localDir, fName)
		content, err := os.ReadFile(repoPath)
		require.NoError(t, err)
		assert.Equal(t, "theme = \"dark\"", string(content))

		// Verify Commit
		headRef, err := repo.Head()
		require.NoError(t, err)
		commit, err := repo.CommitObject(headRef.Hash())
		require.NoError(t, err)
		assert.Contains(t, commit.Message, "Add .config/newapp/config.toml via scadu add")
	})

	t.Run("Add Existing File Fails", func(t *testing.T) {
		// Create file in repo
		fName := ".bashrc"
		repoPath := filepath.Join(localDir, fName)
		err := os.WriteFile(repoPath, []byte("# bashrc"), 0644)
		require.NoError(t, err)
		_, err = w.Add(fName)
		require.NoError(t, err)
		_, err = w.Commit("Add bashrc", &git.CommitOptions{
			Author: &object.Signature{Name: "Test", Email: "test@local", When: time.Now()},
		})
		require.NoError(t, err)

		// Create file in home
		homePath := filepath.Join(homeDir, fName)
		err = os.WriteFile(homePath, []byte("# bashrc home"), 0644)
		require.NoError(t, err)

		// Run Add
		cmd := rootCmd
		cmd.SetArgs([]string{"add", homePath})
		err = cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists in repo")
	})

	t.Run("Add With Edit", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Skipping shell script mock editor test on Windows")
		}

		// Mock Editor
		mockEditorPath := filepath.Join(rootDir, "mock_editor_add.sh")
		scriptContent := `#!/bin/sh
echo "# Added by mock" >> "$1"
`
		err := os.WriteFile(mockEditorPath, []byte(scriptContent), 0755)
		require.NoError(t, err)

		os.Setenv("EDITOR", mockEditorPath)
		defer os.Unsetenv("EDITOR")

		// Create file in home
		fName := ".config/edited/config.ini"
		fPath := filepath.Join(homeDir, fName)
		err = os.MkdirAll(filepath.Dir(fPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fPath, []byte("setting = 1"), 0644)
		require.NoError(t, err)

		// Run Add --edit
		cmd := rootCmd
		// We need to reset flags? cobra commands reuse flags.
		addWithEdit = false // Reset global flag just in case
		defer func() { addWithEdit = false }()
		cmd.SetArgs([]string{"add", "--edit", fPath})

		err = cmd.Execute()
		require.NoError(t, err)

		// Verify Repo Content (Should have edit)
		repoPath := filepath.Join(localDir, fName)
		content, err := os.ReadFile(repoPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "# Added by mock")

		// Verify Home Content (Should have edit via Reify/Install)
		homeContent, err := os.ReadFile(fPath)
		require.NoError(t, err)
		assert.Contains(t, string(homeContent), "# Added by mock")
	})
}
