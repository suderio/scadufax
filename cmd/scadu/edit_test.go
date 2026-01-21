package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditCommand_Integration(t *testing.T) {
	// Setup Directories
	rootDir := setupTestDir(t)
	homeDir := filepath.Join(rootDir, "home")
	localDir := filepath.Join(rootDir, "local")

	err := os.MkdirAll(homeDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(localDir, 0755)
	require.NoError(t, err)

	// Setup Configuration
	viper.Reset()
	viper.Set("scadufax.local_dir", localDir)
	viper.Set("scadufax.home_dir", homeDir)
	viper.Set("scadufax.fork", "testfork")

	// Setup Repo
	repo, err := git.PlainInit(localDir, false)
	require.NoError(t, err)

	// Create a template file in repo
	tplName := ".config/app/config.conf"
	tplPath := filepath.Join(localDir, tplName)
	err = os.MkdirAll(filepath.Dir(tplPath), 0755)
	require.NoError(t, err)

	err = os.WriteFile(tplPath, []byte("fork = {{.scadufax.fork}}\nsecret = {{secret \"MY_SECRET\"}}"), 0644)
	require.NoError(t, err)

	// Commit initial state
	w, err := repo.Worktree()
	require.NoError(t, err)
	_, err = w.Add(".")
	require.NoError(t, err)
	_, err = w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@local", When: time.Now()},
	})
	require.NoError(t, err)

	// Setup User File (simulate existing installation)
	userFilePath := filepath.Join(homeDir, tplName)
	err = os.MkdirAll(filepath.Dir(userFilePath), 0755)
	require.NoError(t, err)
	err = os.WriteFile(userFilePath, []byte("fork = oldfork\nsecret = oldsecret"), 0644)
	require.NoError(t, err)

	// Setup .env in Home
	envPath := filepath.Join(homeDir, ".env")
	err = os.WriteFile(envPath, []byte("MY_SECRET=supersecret"), 0600)
	require.NoError(t, err)

	// Create Mock Editor
	// We use a shell script that appends a comment to the file
	mockEditorPath := filepath.Join(rootDir, "mock_editor.sh")
	scriptContent := `#!/bin/sh
echo "# Edited by mock" >> "$1"
`
	if runtime.GOOS == "windows" {
		t.Skip("Skipping shell script mock editor test on Windows")
	}

	err = os.WriteFile(mockEditorPath, []byte(scriptContent), 0755)
	require.NoError(t, err)

	// Execution
	// We need to set EDITOR env var to our mock script
	os.Setenv("EDITOR", mockEditorPath)
	defer os.Unsetenv("EDITOR")

	// Args: edit command + path to user file
	cmd := rootCmd
	cmd.SetArgs([]string{"edit", userFilePath})

	// Run
	err = cmd.Execute()
	require.NoError(t, err)

	// Verification

	// 1. Repo file should be modified
	newTplContent, err := os.ReadFile(tplPath)
	require.NoError(t, err)
	assert.Contains(t, string(newTplContent), "# Edited by mock")

	// 2. Repo should have a new commit
	headRef, err := repo.Head()
	require.NoError(t, err)
	commit, err := repo.CommitObject(headRef.Hash())
	require.NoError(t, err)
	assert.Contains(t, commit.Message, "Update .config/app/config.conf via scadu edit")

	// 3. User file should be reified (updated content + rendered template)
	// The mock editor appended to the TEMPLATE.
	// So the reified file should have:
	// fork = testfork
	// secret = supersecret
	// # Edited by mock

	newUserContent, err := os.ReadFile(userFilePath)
	require.NoError(t, err)
	sContent := string(newUserContent)

	assert.Contains(t, sContent, "fork = testfork")
	assert.Contains(t, sContent, "secret = supersecret")
	assert.Contains(t, sContent, "# Edited by mock")
}
