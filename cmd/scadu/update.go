package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/suderio/scadufax/pkg/gitops"
)

var updateWait bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update home directory from fork branch",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Resolve Config
		localDir := viper.GetString("scadufax.local_dir")
		if localDir == "" {
			home, _ := os.UserHomeDir()
			localDir = filepath.Join(home, ".local", "share", "scadufax")
		}
		homeDir := viper.GetString("scadufax.home_dir")
		if homeDir == "" {
			homeDir, _ = os.UserHomeDir()
		}
		forkName := viper.GetString("scadufax.fork")
		if forkName == "" {
			hostname, _ := os.Hostname()
			forkName = hostname
		}
		ignorePatterns := viper.GetStringSlice("root.ignore")

		// 2. Push Main
		fmt.Println("Switching to main...")
		if err := gitops.Checkout(localDir, "main"); err != nil {
			return fmt.Errorf("failed to checkout main: %w", err)
		}

		fmt.Println("Pushing main to origin...")
		if err := gitops.Push(localDir); err != nil {
			return fmt.Errorf("failed to push main: %w", err)
		}

		mainID, err := gitops.GetHeadID(localDir)
		if err != nil {
			return fmt.Errorf("failed to get main ID: %w", err)
		}
		fmt.Printf("Main SCADUFAX_ID: %s\n", mainID)

		// 3. Pull Fork and Wait
		fmt.Printf("Switching to fork '%s'...\n", forkName)
		if err := gitops.Checkout(localDir, forkName); err != nil {
			return fmt.Errorf("failed to checkout fork: %w", err)
		}

		fmt.Println("Pulling fork...")
		if err := gitops.Pull(localDir); err != nil {
			// Pull fail might be ok if remote branch doesn't exist yet/matches local
			fmt.Printf("Pull warning: %v\n", err)
		}

		if updateWait && mainID != "" {
			fmt.Println("Waiting for fork to sync with main ID...")
			for {
				forkID, err := gitops.GetHeadID(localDir)
				if err != nil {
					return fmt.Errorf("failed to get fork ID: %w", err)
				}

				if forkID == mainID {
					fmt.Println("Fork synced with Main ID.")
					break
				}

				fmt.Printf("Fork ID (%s) != Main ID (%s). Retrying in 5s...\n", forkID, mainID)
				time.Sleep(5 * time.Second)

				fmt.Println("Pulling fork...")
				if err := gitops.Pull(localDir); err != nil {
					fmt.Printf("Pull warning: %v\n", err)
				}
			}
		}

		// 4. Check Differences (Fork vs Home)
		// We reuse compareDirs logic but need to capture changed files.
		// Since compareDirs in check.go prints, let's just use it to show diffs?
		// Requirement: "shows differences in the same way of the check command"
		// Requirement: "ask if user wants to update ... copies changed/new files"
		// We need to know IF there are differences and WHICH files.
		// Refactoring check.go to return list of diffs is best.
		// For now, let's implement a local walker here that does both: print and collect.

		diffs, err := getDiffs(localDir, homeDir, ignorePatterns)
		if err != nil {
			return err
		}

		if len(diffs) == 0 {
			fmt.Println("No differences found. Home is up to date.")
			return nil
		}

		// confirm update
		fmt.Print("Update home directory with these changes? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp != "y" && resp != "yes" {
			fmt.Println("Update aborted.")
			return nil
		}

		// Copy files
		for _, rel := range diffs {
			src := filepath.Join(localDir, rel)
			dst := filepath.Join(homeDir, rel)
			fmt.Printf("Updating %s...\n", rel)
			if err := copyFile(src, dst); err != nil {
				return fmt.Errorf("failed to update %s: %w", rel, err)
			}
		}

		fmt.Println("Update complete.")
		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateWait, "wait", false, "wait for fork branch to catch up with main")
	rootCmd.AddCommand(updateCmd)
}

// getDiffs prints diffs (like check) and returns list of modified/new files in source (Repo).
func getDiffs(sourceDir, targetDir string, ignores []string) ([]string, error) {
	var changes []string

	// Reuse check logic colors? We can import color or just plain text if needed.
	// We want to match check command output.
	// Copy pasted/adapted from check.go (or we should export functions from check.go)
	// Since check.go is package main, we can create a shared compare.go in cmd/scadu if we want.
	// But simple logic reproduction here is fine for now to avoid large refactor risks.
	// NOTE: check.go uses green/yellow/red from fatih/color.

	err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		if isIgnored(rel, ignores) {
			return nil
		}

		targetPath := filepath.Join(targetDir, rel)

		isNew := false
		isMod := false

		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			isNew = true
		} else {
			if areFilesDifferent(path, targetPath) {
				isMod = true
			}
		}

		if isNew {
			fmt.Printf("N\t%s\n", rel) // Green N ideally
			changes = append(changes, rel)
		} else if isMod {
			fmt.Printf("M\t%s\n", rel) // Yellow M ideally
			changes = append(changes, rel)
		}

		return nil
	})

	return changes, err
}
