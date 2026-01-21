package main

import (
	"bytes"
	"io"
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

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestCheckCommand_Integration(t *testing.T) {
	// Setup
	rootDir := setupTestDir(t)
	homeDir := filepath.Join(rootDir, "home")
	localDir := filepath.Join(rootDir, "local")

	err := os.MkdirAll(homeDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(localDir, 0755)
	require.NoError(t, err)

	viper.Reset()
	viper.Set("scadufax.local_dir", localDir)
	viper.Set("scadufax.home_dir", homeDir)
	viper.Set("scadufax.fork", "testfork")
	viper.Set("root.ignore", []string{"ignored.txt"})

	// Setup Repo
	repo, err := git.PlainInit(localDir, false)
	require.NoError(t, err)
	w, err := repo.Worktree()
	require.NoError(t, err)

	// Create "main" content (Templates)
	// Explicitly set Head to main (PlainInit defaults to master)
	headRef := plumbing.ReferenceName("refs/heads/main")
	err = repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, headRef))
	require.NoError(t, err)

	// file1: simple content
	// file2: template content
	err = os.WriteFile(filepath.Join(localDir, "file1.txt"), []byte("base content"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(localDir, "file2.conf"), []byte("val = {{.key}}"), 0644)
	require.NoError(t, err)

	_, err = w.Add(".")
	require.NoError(t, err)
	_, err = w.Commit("Initial main", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@local", When: time.Now()},
	})
	require.NoError(t, err)

	// Create "testfork" branch
	// We simulate that the fork has reified values
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName("refs/heads/testfork"),
		Create: true,
	})
	require.NoError(t, err)

	// Modify files in fork to represent "reified" state
	viper.Set("key", "testvalue")

	// file1 remains "base content"
	// file2 becomes "val = testvalue"
	err = os.WriteFile(filepath.Join(localDir, "file2.conf"), []byte("val = testvalue"), 0644)
	require.NoError(t, err)

	_, err = w.Add(".")
	require.NoError(t, err)
	_, err = w.Commit("Fork reified state", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@local", When: time.Now()},
	})
	require.NoError(t, err)

	// Setup Home Directory (Real User State)
	// file1: Matches fork
	err = os.WriteFile(filepath.Join(homeDir, "file1.txt"), []byte("base content"), 0644)
	require.NoError(t, err)

	// file2: Modified in Home
	err = os.WriteFile(filepath.Join(homeDir, "file2.conf"), []byte("val = user_modified"), 0644)
	require.NoError(t, err)

	// file3: Only in Home (Extra)
	err = os.WriteFile(filepath.Join(homeDir, "extra.txt"), []byte("extra"), 0644)
	require.NoError(t, err)

	// file4: Only in Fork (Missing in Home)
	err = os.WriteFile(filepath.Join(localDir, "missing_in_home.txt"), []byte("repo only"), 0644)
	require.NoError(t, err)
	_, err = w.Add("missing_in_home.txt")
	require.NoError(t, err)
	w.Commit("Add missing file", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test", When: time.Now()},
	})

	// ignored.txt: Ignored
	err = os.WriteFile(filepath.Join(localDir, "ignored.txt"), []byte("ignore me"), 0644)
	require.NoError(t, err)
	w.Add("ignored.txt")
	w.Commit("Add ignored", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test", When: time.Now()},
	})

	// Test 1: Basic Check (Local Status)
	// Should show:
	// M file2.conf
	// D extra.txt (if --all)
	// N missing_in_home.txt

	output := captureOutput(func() {
		cmd := rootCmd
		cmd.SetArgs([]string{"check", "--local", "--all"})
		err := cmd.Execute()
		assert.NoError(t, err)
	})

	// Assertions
	// Check for filenames in output
	assert.Contains(t, output, "Local Status:")
	assert.Contains(t, output, "file2.conf")          // M
	assert.Contains(t, output, "extra.txt")           // D
	assert.Contains(t, output, "missing_in_home.txt") // N
	assert.NotContains(t, output, "file1.txt")        // Clean
	assert.NotContains(t, output, "ignored.txt")      // Ignored

	// Verify M/D/N markers (colors make strict match hard, check substrings)
	// Need to check line context theoretically, but simple existence is good start.
	// "M \t file2.conf" roughly

	// Test 2: Full Check
	// Fork has "val = testvalue". Main template has "val = {{.key}}". Config key="testvalue".
	// So Reified Main should match Fork.
	// We expect NO output in "Template Status" section except Maybe Clean?
	// Wait, the command only prints Diffs.

	// Let's diverge Main and Fork to see a diff.
	// Checkout Main
	w.Checkout(&git.CheckoutOptions{Branch: plumbing.ReferenceName("refs/heads/main")})
	// Modify template in Main
	os.WriteFile(filepath.Join(localDir, "file2.conf"), []byte("val = {{.key}} changed"), 0644)
	w.Add("file2.conf")
	w.Commit("Update main template", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test", When: time.Now()},
	})
	// Checkout Fork back (Check command expects to start at fork or handles it? It checks out fork at step 2)
	w.Checkout(&git.CheckoutOptions{Branch: plumbing.ReferenceName("refs/heads/testfork")})

	// Now comparisons:
	// Main Reified: "val = testvalue changed"
	// Fork: "val = testvalue"
	// Diff expected.

	outputFull := captureOutput(func() {
		cmd := rootCmd
		cmd.SetArgs([]string{"check", "--local", "--full"})
		err := cmd.Execute()
		assert.NoError(t, err)
	})

	assert.Contains(t, outputFull, "Template Status (Main vs Fork):")
	assert.Contains(t, outputFull, "file2.conf") // Should be Modified (M) in template status
}
