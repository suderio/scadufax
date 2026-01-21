package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/suderio/scadufax/pkg/gitops"
)

var listAll bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List files in the main branch and their status",
	RunE: func(cmd *cobra.Command, args []string) error {
		localDir := viper.GetString("scadufax.local_dir")
		if localDir == "" {
			home, _ := os.UserHomeDir()
			localDir = filepath.Join(home, ".local", "share", "scadufax")
		}
		homeDir := viper.GetString("scadufax.home_dir")
		if homeDir == "" {
			homeDir, _ = os.UserHomeDir()
		}

		// Checkout Main
		if err := gitops.Checkout(localDir, "main"); err != nil {
			return fmt.Errorf("failed to checkout main: %w", err)
		}

		ignorePatterns := viper.GetStringSlice("root.ignore")
		red := color.New(color.FgRed).SprintFunc()

		// 1. List files in Main
		err := filepath.WalkDir(localDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}

			rel, err := filepath.Rel(localDir, path)
			if err != nil {
				return err
			}

			// Should we respect ignore patterns for Main files?
			// Usually yes, but if it is in Repo, it is tracked.
			// Ignores usually apply to what we verify/copy.
			// Let's assume checked in files are valid content.

			homePath := filepath.Join(homeDir, rel)

			if _, err := os.Stat(homePath); os.IsNotExist(err) {
				fmt.Printf("%s   %s\n", red("MISSING"), homePath)
			} else {
				fmt.Printf("          %s\n", homePath)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to list main files: %w", err)
		}

		// 2. List UNMANAGED (if --all)
		if listAll {
			err := filepath.WalkDir(homeDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					if d.Name() == ".git" {
						return filepath.SkipDir
					}
					return nil
				}

				rel, err := filepath.Rel(homeDir, path)
				if err != nil {
					return err
				}

				if isIgnored(rel, ignorePatterns) {
					return nil
				}

				repoPath := filepath.Join(localDir, rel)
				if _, err := os.Stat(repoPath); os.IsNotExist(err) {
					fmt.Printf("%s %s\n", red("UNMANAGED"), path)
				}

				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to check unmanaged files: %w", err)
			}
		}

		return nil
	},
}

func init() {
	listCmd.Flags().BoolVar(&listAll, "all", false, "Show unmanaged files in home directory")
	rootCmd.AddCommand(listCmd)
}
