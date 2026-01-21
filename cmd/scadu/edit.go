package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/suderio/scadufax/pkg/gitops"
	"github.com/suderio/scadufax/pkg/processor"
)

var editCmd = &cobra.Command{
	Use:   "edit [file]...",
	Short: "Open files in the system editor",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Resolve Configuration
		localDir := viper.GetString("scadufax.local_dir")
		if localDir == "" {
			// Default per README/Request if not set in config
			home, _ := os.UserHomeDir()
			localDir = filepath.Join(home, ".local", "share", "scadufax")
		}

		homeDir := viper.GetString("scadufax.home_dir")
		if homeDir == "" {
			homeDir, _ = os.UserHomeDir()
		}

		// 2. Map arguments to Repo Paths
		// We expect args to be user paths (e.g. ~/.bashrc or .config/nvim/init.lua)
		// We need to find the corresponding file in repo (localDir).
		// Assumption: The structure in repo matches home directory structure relative to home?
		// User Example: "if the user edited a .config/nvim/init.lua file in the main branch... reified... copied to ~/.config/nvim/init.lua"
		// This implies the CLI args are paths RELATIVE TO HOME or Absolute Paths matching Home?
		// OR: "the edit command must actually open ~/.local/share/scadufax/.bashrc ... when user asks for ~/.bashrc"
		// So we map: Arg -> Relative to Home -> LocalDir + Relative.

		var templateFiles []string
		var relPaths []string

		for _, arg := range args {
			// Convert arg to absolute
			absPath, err := filepath.Abs(arg)
			if err != nil {
				return fmt.Errorf("failed to get abs path for %s: %w", arg, err)
			}

			// Check if inside Home
			if !strings.HasPrefix(absPath, homeDir) {
				return fmt.Errorf("file %s is not in home directory %s", arg, homeDir)
			}

			rel, err := filepath.Rel(homeDir, absPath)
			if err != nil {
				return fmt.Errorf("failed to get relative path: %w", err)
			}

			// Target in Repo
			repoPath := filepath.Join(localDir, rel)
			templateFiles = append(templateFiles, repoPath)
			relPaths = append(relPaths, rel)
		}

		// 3. Perform Edit Workflow
		return PerformEdit(templateFiles, localDir, homeDir)
	},
}

// PerformEdit handles the editing, verification, reification, installation, and committing of files.
func PerformEdit(templateFiles []string, localDir, homeDir string) error {
	// 1. Open Editor on Repo Paths
	editor, err := resolveEditor()
	if err != nil {
		return err
	}

	cmd := exec.Command(editor, templateFiles...)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Editing %v in repo (branch main)...\n", templateFiles)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	// 2. Post-Edit Logic
	fmt.Println("Editor closed. Checking for changes...")

	var dirtyFiles []string
	var relPaths []string

	for _, repoPath := range templateFiles {
		fileRel, err := filepath.Rel(localDir, repoPath)
		if err != nil {
			return fmt.Errorf("path error: %w", err)
		}

		isDirty, err := gitops.IsDirty(localDir, fileRel)
		if err != nil {
			return fmt.Errorf("failed to check status for %s: %w", repoPath, err)
		}

		if isDirty {
			dirtyFiles = append(dirtyFiles, fileRel)
			relPaths = append(relPaths, fileRel)
		}
	}

	if len(dirtyFiles) == 0 {
		fmt.Println("No changes detected.")
		return nil
	}

	fmt.Printf("Detected changes in: %v\n", dirtyFiles)

	// 3. Reify and Install
	tempDir, err := os.MkdirTemp("", "scadu-reify-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	envPath := filepath.Join(homeDir, ".env")
	secretFn, err := processor.GetSecretFn(envPath, false)
	if err != nil {
		return err
	}

	data := viper.AllSettings()

	for _, rel := range dirtyFiles {
		repoPath := filepath.Join(localDir, rel)
		tempPath := filepath.Join(tempDir, rel)
		finalPath := filepath.Join(homeDir, rel)

		fmt.Printf("Reifying %s...\n", rel)
		if err := processor.ReifyFile(repoPath, tempPath, data, secretFn); err != nil {
			return fmt.Errorf("reification failed for %s: %w", rel, err)
		}

		fmt.Printf("Installing to %s...\n", finalPath)
		content, err := os.ReadFile(tempPath)
		if err != nil {
			return err
		}

		// Attempt to preserve mode from repo file
		info, err := os.Stat(repoPath)
		mode := os.FileMode(0644)
		if err == nil {
			mode = info.Mode()
		}

		if err := os.WriteFile(finalPath, content, mode); err != nil {
			return fmt.Errorf("failed to install file: %w", err)
		}

		fmt.Printf("Committing %s...\n", rel)
		msg := GenerateCommitMessage(fmt.Sprintf("Update %s via scadu edit", rel))
		if err := gitops.CommitFile(localDir, rel, msg); err != nil {
			return fmt.Errorf("failed to commit %s: %w", rel, err)
		}
	}

	fmt.Println("Done.")
	return nil
}

func resolveEditor() (string, error) {
	if env := os.Getenv("EDITOR"); env != "" {
		if path, err := exec.LookPath(env); err == nil {
			return path, nil
		}
	}
	if env := os.Getenv("VISUAL"); env != "" {
		if path, err := exec.LookPath(env); err == nil {
			return path, nil
		}
	}
	for _, fb := range []string{"nvim", "vim", "vi"} {
		if path, err := exec.LookPath(fb); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no valid editor found")
}

func init() {
	rootCmd.AddCommand(editCmd)
}
