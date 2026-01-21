package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/suderio/scadufax/pkg/gitops"
)

var removeLocal bool

var removeCmd = &cobra.Command{
	Use:   "remove [file]...",
	Short: "Remove files from the scadu repository",
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
		// Remove command operates on main branch
		if err := gitops.Checkout(localDir, "main"); err != nil {
			return fmt.Errorf("failed to checkout main: %w", err)
		}

		// 3. Process Arguments
		for _, arg := range args {
			// Resolve Abs Path
			absPath, err := filepath.Abs(arg)
			if err != nil {
				return fmt.Errorf("failed to resolve path %s: %w", arg, err)
			}

			// Validate inside Home (if user passed home path)
			// OR user passed repo rel path? Scadu usually takes home paths.
			// Let's assume standard behavior: user passes e.g. ~/.bashrc
			if !strings.HasPrefix(absPath, homeDir) {
				return fmt.Errorf("file %s is not in home directory %s", arg, homeDir)
			}

			rel, err := filepath.Rel(homeDir, absPath)
			if err != nil {
				return fmt.Errorf("failed to get relative path: %w", err)
			}

			repoPath := filepath.Join(localDir, rel)

			// Check if exists in repo
			if _, err := os.Stat(repoPath); os.IsNotExist(err) {
				fmt.Printf("File %s not found in repository. Skipping.\n", rel)
				continue
			}

			// 4. Repo Removal
			fmt.Printf("Removing %s from repository...\n", rel)
			// repoPath for Remove needs to be absolute?
			// gitops.Remove opens repo. And does w.Remove(filePath).
			// go-git w.Remove documentation says: "removes the given file from the worktree and the index".
			// Argument is filepath. "must be relative to the worktree root".
			// So we pass 'rel'.
			if err := gitops.Remove(localDir, rel); err != nil {
				return fmt.Errorf("failed to remove %s from repo: %w", rel, err)
			}

			msg := GenerateCommitMessage(fmt.Sprintf("Remove %s via scadu remove", rel))
			if err := gitops.CommitFile(localDir, rel, msg); err != nil {
				return fmt.Errorf("failed to commit removal of %s: %w", rel, err)
			}

			// 5. Local Removal
			if removeLocal {
				// Check configuration for confirm
				confirm := viper.GetBool("scadufax.confirm")
				// Default is true if not set? Viper default handling needs to be checked or set in init.
				// User said: "default true ... unless ... false".
				// In init.go we defined pointer in struct but viper.GetBool returns false if not set?
				// Viper defaults need to be set. Or we assume true if not explicitly false.
				// Since we didn't set default in init(), getting default might be false.
				// We should check if key exists or set default.
				// Let's assume we want default TRUE.
				if !viper.IsSet("scadufax.confirm") {
					confirm = true
				}

				if confirm {
					fmt.Printf("Delete %s from home directory? [y/N]: ", absPath)
					reader := bufio.NewReader(os.Stdin)
					response, err := reader.ReadString('\n')
					if err != nil {
						return fmt.Errorf("failed to read input: %w", err)
					}
					response = strings.TrimSpace(strings.ToLower(response))
					if response != "y" && response != "yes" {
						fmt.Println("Skipping local deletion.")
						continue
					}
				}

				fmt.Printf("Removing %s from home directory...\n", absPath)
				if err := os.Remove(absPath); err != nil {
					return fmt.Errorf("failed to remove local file %s: %w", absPath, err)
				}
			}
		}

		fmt.Println("Done.")
		return nil
	},
}

func init() {
	removeCmd.Flags().BoolVar(&removeLocal, "local", false, "Also remove the file from the home directory")
	rootCmd.AddCommand(removeCmd)
}
