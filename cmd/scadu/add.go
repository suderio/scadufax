package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/suderio/scadufax/pkg/gitops"
)

var addWithEdit bool

var addCmd = &cobra.Command{
	Use:   "add [file]...",
	Short: "Add local files to the scadu repository",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Resolve Configuration
		localDir := viper.GetString("scadufax.local_dir")
		if localDir == "" {
			home, _ := os.UserHomeDir()
			localDir = filepath.Join(home, ".local", "share", "scadufax")
		}

		homeDir := viper.GetString("scadufax.home_dir")
		if homeDir == "" {
			homeDir, _ = os.UserHomeDir()
		}

		// 2. Ensure Main Branch
		fmt.Println("Switching to branch main...")
		if err := gitops.Checkout(localDir, "main"); err != nil {
			return fmt.Errorf("failed to checkout main: %w", err)
		}
		// Also ensure up to date?
		// Plan said "Pre-checks: Checkout main". editCmd logic isn't enforcing pull, but Check does.
		// Usually add doesn't require pull unless we fear conflict. Let's stick to checkout.

		// 3. Process Arguments
		var repoFiles []string
		var relPaths []string

		for _, arg := range args {
			// Resolve Abs Path
			absPath, err := filepath.Abs(arg)
			if err != nil {
				return fmt.Errorf("failed to resolve path %s: %w", arg, err)
			}

			// Validate inside Home
			if !strings.HasPrefix(absPath, homeDir) {
				return fmt.Errorf("file %s is not in home directory %s", arg, homeDir)
			}

			rel, err := filepath.Rel(homeDir, absPath)
			if err != nil {
				return fmt.Errorf("failed to get relative path: %w", err)
			}

			if rel == "." || rel == ".." {
				return fmt.Errorf("invalid path %s", arg)
			}

			repoPath := filepath.Join(localDir, rel)

			// Validate NOT in Repo
			if _, err := os.Stat(repoPath); err == nil {
				return fmt.Errorf("file %s already exists in repo. Use 'scadu edit' to modify it", rel)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("failed to check repo path %s: %w", repoPath, err)
			}

			// Copy Home -> Repo
			fmt.Printf("Adding %s to repo...\n", rel)
			if err := copyFile(absPath, repoPath); err != nil {
				return fmt.Errorf("failed to copy %s to repo: %w", rel, err)
			}

			repoFiles = append(repoFiles, repoPath)
			relPaths = append(relPaths, rel)
		}

		// 4. Post-Add Workflow
		if addWithEdit {
			// Use PerformEdit
			// PerformEdit handles editing, reifying, installing, committing
			return PerformEdit(repoFiles, localDir, homeDir)
		}

		// 5. Direct Commit
		for _, rel := range relPaths {
			fmt.Printf("Committing %s...\n", rel)
			msg := GenerateCommitMessage(fmt.Sprintf("Add %s via scadu add", rel))
			if err := gitops.CommitFile(localDir, rel, msg); err != nil {
				return fmt.Errorf("failed to commit %s: %w", rel, err)
			}
		}

		fmt.Println("Done.")
		return nil
	},
}

func init() {
	addCmd.Flags().BoolVar(&addWithEdit, "edit", false, "Edit the files after adding")
	rootCmd.AddCommand(addCmd)
}

func copyFile(src, dst string) error {
	// Ensure dest dir exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Copy mode
	info, err := os.Stat(src)
	if err == nil {
		os.Chmod(dst, info.Mode())
	}

	return nil
}
