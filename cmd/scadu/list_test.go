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

func TestListCommand_Integration(t *testing.T) {
	rootDir := setupTestDir(t)
	localDir := filepath.Join(rootDir, "local")
	homeDir := filepath.Join(rootDir, "home")

	os.MkdirAll(homeDir, 0755)

	// Setup Repo with Main
	repo, err := git.PlainInit(localDir, false)
	require.NoError(t, err)
	w, _ := repo.Worktree()

	fRel := ".config/app/conf.toml"
	fPath := filepath.Join(localDir, fRel)
	os.MkdirAll(filepath.Dir(fPath), 0755)
	os.WriteFile(fPath, []byte("content"), 0644)
	w.Add(fRel)
	w.Commit("Init", &git.CommitOptions{Author: &object.Signature{Name: "T", Email: "t", When: time.Now()}})

	// Ensure main ref
	head, _ := repo.Head()
	repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/main"), head.Hash()))

	// Config
	viper.Reset()
	viper.Set("scadufax.local_dir", localDir)
	viper.Set("scadufax.home_dir", homeDir)

	t.Run("List_Missing", func(t *testing.T) {
		// Rel file exists in repo, missing in home.
		// Expected: MISSING ...
		cmd := rootCmd
		listAll = false
		cmd.SetArgs([]string{"list"})

		// We need to capture stdout to verify "MISSING"
		// Let's rely on manual capturing logic block
		r, wPipe, _ := os.Pipe()
		oldStdout := os.Stdout
		os.Stdout = wPipe

		err := cmd.Execute()

		wPipe.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		// Read pipe
		outBytes := make([]byte, 1024)
		n, _ := r.Read(outBytes)
		output := string(outBytes[:n])

		assert.Contains(t, output, "MISSING")
		assert.Contains(t, output, fRel)
	})

	t.Run("List_Present", func(t *testing.T) {
		// Create file in home
		homePath := filepath.Join(homeDir, fRel)
		os.MkdirAll(filepath.Dir(homePath), 0755)
		os.WriteFile(homePath, []byte("content"), 0644)

		cmd := rootCmd
		cmd.SetArgs([]string{"list"})

		r, wPipe, _ := os.Pipe()
		oldStdout := os.Stdout
		os.Stdout = wPipe

		err := cmd.Execute()

		wPipe.Close()
		os.Stdout = oldStdout
		require.NoError(t, err)

		outBytes := make([]byte, 1024)
		n, _ := r.Read(outBytes)
		output := string(outBytes[:n])

		assert.NotContains(t, output, "MISSING")
		assert.Contains(t, output, homePath)
	})

	t.Run("List_Unmanaged", func(t *testing.T) {
		// Create unmanaged file in home
		unmanagedRel := "unmanaged.txt"
		os.WriteFile(filepath.Join(homeDir, unmanagedRel), []byte("u"), 0644)

		cmd := rootCmd
		listAll = true // Manually set flagvar if direct cmd exec, or use args if using rootCmd
		cmd.SetArgs([]string{"list", "--all"})

		r, wPipe, _ := os.Pipe()
		oldStdout := os.Stdout
		os.Stdout = wPipe

		err := cmd.Execute()

		wPipe.Close()
		os.Stdout = oldStdout
		require.NoError(t, err)

		outBytes := make([]byte, 1024)
		n, _ := r.Read(outBytes)
		output := string(outBytes[:n])

		assert.Contains(t, output, "UNMANAGED")
		assert.Contains(t, output, unmanagedRel)
	})
}
