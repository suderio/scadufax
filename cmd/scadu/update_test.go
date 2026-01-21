package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/suderio/scadufax/pkg/gitops"
)

func TestUpdateCommand_Integration(t *testing.T) {
	// Setup Complex Git Environment: Origin, LocalRepo, Home
	// We need an "origin" bare repo to test Push/Pull interaction
	rootDir := setupTestDir(t)
	originPath := filepath.Join(rootDir, "origin")
	localPath := filepath.Join(rootDir, "local") // This will be our "localDir"
	homePath := filepath.Join(rootDir, "home")

	// 1. Setup Origin (Bare)
	_, err := git.PlainInit(originPath, true)
	require.NoError(t, err)

	// 2. Setup "Remote Side" to seed content into origin (simulate someone else pushing)
	// Or we can just Init local and Push to origin first.

	// Let's Init Local and Push "Initial State"
	repo, err := git.PlainInit(localPath, false)
	require.NoError(t, err)

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{originPath},
	})
	require.NoError(t, err)

	w, _ := repo.Worktree()

	// Create main
	fMain := filepath.Join(localPath, "file.txt")
	os.WriteFile(fMain, []byte("v1"), 0644)
	w.Add("file.txt")
	w.Commit("Initial", &git.CommitOptions{Author: &object.Signature{Name: "T", Email: "t", When: time.Now()}})

	// Push to origin main
	// But wait, if push fails because main doesn't exist locally (it's master?), we should rename first.
	// Rename current branch to main.
	head, err := repo.Head()
	require.NoError(t, err)
	err = repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/main"), head.Hash()))
	require.NoError(t, err)
	err = repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main")))
	require.NoError(t, err)

	err = repo.Push(&git.PushOptions{RemoteName: "origin"})
	require.NoError(t, err)

	// Create Fork locally
	// Note: Command expects "fork" to exist.
	// Usually "init" creates fork.
	w.Checkout(&git.CheckoutOptions{Branch: plumbing.ReferenceName("refs/heads/fork"), Create: true})

	// Setup Config
	viper.Reset()
	viper.Set("scadufax.local_dir", localPath)
	viper.Set("scadufax.home_dir", homePath)
	viper.Set("scadufax.fork", "fork")

	t.Run("Update_No_Changes", func(t *testing.T) {
		// Home has "v1" (synced)
		os.MkdirAll(homePath, 0755)
		os.WriteFile(filepath.Join(homePath, "file.txt"), []byte("v1"), 0644)

		cmd := rootCmd
		updateWait = false // Reset flag
		cmd.SetArgs([]string{"update"})
		err := cmd.Execute()
		require.NoError(t, err)

		// Expect no output/error.
	})

	t.Run("Update_With_Changes_And_Sync", func(t *testing.T) {
		// Scenario:
		// 1. Home has "v1".
		// 2. Main (Origin) gets updated to "v2" (Simulate external push or previous edit).
		// 3. We run Update. Update should: Checkout Main -> Push (No-op here) -> Get ID.
		//    Then Checkout Fork -> Pull.
		//    Wait, Pull pulls 'origin/main' into 'fork'? Or 'pull' pulls 'origin/fork'?
		//    gitops.Pull pulls current branch from upstream.
		//    Usually Fork tracks Origin/Main or Origin/Fork?
		//    In "init", we create fork from HEAD (main).
		//    Usually we want Fork to rebase on Main? Or Pull Main?
		//    The Readme says: "pulls the fork branch".
		//    If fork tracks origin/fork, it gets updates from remote fork.
		//    If user wants to update Home from "Main", Fork should be updated from Main?
		//    "checks if fork branch has difference from home".
		//    So logic is: Fork is Source. Home is Target.
		//    How does Fork get new content? "pulls the fork branch".
		//    If I developed feature on Fork A, and I want to update Home...
		//    Or is this "Update Scadu tool itself"? No, update home dir.
		//    Ah, maybe "Pull" implies getting latest from remote FORK.
		//    If I used 'edit' covering 'main' -> 'reify' -> 'commit' -> 'push main?'.
		//    The `edit` command commits to Repo (Main? Edit checks out Main?).
		//    Wait, `edit` says "Editing ... in repo (branch main)".
		//    So Main has the latest config.
		//    Why "Pull Fork"?
		//    If Main has latest, we should compare Main vs Home?
		//    Check command: "Compares Fork vs Home".
		//    Why Fork? Because Fork might have machine-specific divergence?
		//    Or Fork is just the local working state?
		//    "checks if fork branch has difference from home directory".
		//    If I want to sync Home to Main, I should Merge Main to Fork?
		//    The user request only says: "pushes main... pulls fork".
		//    It does NOT say "Merge Main into Fork".
		//    Maybe the workflow implies Fork tracks Main?
		//    Or maybe Fork is where we keep machine specific state, but we pull it to ensure it is up to date with remote machine specific state?
		//    Let's stick to EXPLICIT logic requested:
		//    1. Push Main.
		//    2. Pull Fork.
		//    3. Check Fork vs Home.
		//    So if Fork is outdated compared to Main, and Home is outdated, Update cmd won't fix it unless Pull Fork brings in Main changes?
		//    Unless Fork IS tracking Main?
		//    `gitops.Init` pulls origin main. `CreateBranch` fork from HEAD.
		//    If `gitops.Pull` is called on Fork, it pulls from its upstream.
		//    Does Fork have upstream? `CreateBranch` (local) doesn't set upstream unless we push/set it.
		//    If upstream is not set, Pull fails.
		//    Assume Fork tracks origin/fork (if it exists) or we setup upstream?
		//    Let's assume Fork tracks origin/main ? No, that's weird.
		//    Let's assume the user handles branch tracking or it's implicitly "origin/main".
		//    For test, let's assume Fork has "v2" (Simulate we pulled v2).

		// Update "file.txt" in Fork to "v2"
		gitops.Checkout(localPath, "fork")
		os.WriteFile(filepath.Join(localPath, "file.txt"), []byte("v2"), 0644)
		w.Add("file.txt")
		w.Commit("Update v2", &git.CommitOptions{Author: &object.Signature{Name: "T", Email: "t", When: time.Now()}})

		// Mock Stdin "y"
		r, wPipe, _ := os.Pipe()
		oldStdin := os.Stdin
		defer func() { os.Stdin = oldStdin }()
		os.Stdin = r
		wPipe.Write([]byte("y\n"))
		wPipe.Close()

		cmd := rootCmd
		cmd.SetArgs([]string{"update"})
		err = cmd.Execute()
		require.NoError(t, err)

		// Verify Home updated to v2
		content, _ := os.ReadFile(filepath.Join(homePath, "file.txt"))
		assert.Equal(t, "v2", string(content))
	})
}
