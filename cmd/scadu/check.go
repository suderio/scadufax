package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/suderio/scadufax/pkg/gitops"
	"github.com/suderio/scadufax/pkg/processor"
)

var (
	checkFlagLocal bool
	checkFlagFull  bool
	checkFlagAll   bool
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for differences between home and dotfiles repository",
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

		forkName := viper.GetString("scadufax.fork")
		if forkName == "" {
			hostname, _ := os.Hostname()
			forkName = hostname
		}

		// Load ignores
		ignorePatterns := viper.GetStringSlice("root.ignore")

		// 1. Pull
		if !checkFlagLocal {
			fmt.Println("Pulling changes...")
			if err := gitops.Pull(localDir); err != nil {
				fmt.Printf("Warning: pull failed: %v\n", err)
			}
		}

		// 2. Fork Comparison
		fmt.Printf("Checking fork branch '%s'...\n", forkName)
		if err := gitops.Checkout(localDir, forkName); err != nil {
			return fmt.Errorf("failed to checkout fork branch %s: %w", forkName, err)
		}

		fmt.Println("Local Status:")
		if err := compareDirs(localDir, homeDir, ignorePatterns, checkFlagAll); err != nil {
			return err
		}

		// 3. Full Comparison (Main vs Fork)
		if checkFlagFull {
			fmt.Println("\nChecking main branch (template status)...")
			if err := gitops.Checkout(localDir, "main"); err != nil {
				return fmt.Errorf("failed to checkout main: %w", err)
			}

			// Reify main to temp
			// We DO NOT use processor.Reify as it is in-place.
			// We use ReifyFile to write to tempDir.
			tempDir, err := os.MkdirTemp("", "scadu-check-full-*")
			if err != nil {
				return fmt.Errorf("failed to create temp dir: %w", err)
			}
			defer os.RemoveAll(tempDir)

			// Secret Strategy: Preserve tags (compare against fork which has secrets preserved)
			secretFn := func(key string) (string, error) {
				return fmt.Sprintf("{{ %q | secret }}", key), nil
			}

			data := viper.AllSettings()

			// Walk main (localDir) and reify to tempDir
			// Wait, we need to walk localDir content, apply template, write to dest.
			// processor.ReifyFile(source, dest...)
			err = filepath.WalkDir(localDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					if d.Name() == ".git" {
						return filepath.SkipDir
					}
					return nil
				}
				rel, _ := filepath.Rel(localDir, path)
				destPath := filepath.Join(tempDir, rel)
				return processor.ReifyFile(path, destPath, data, secretFn)
			})
			if err != nil {
				return fmt.Errorf("failed to reify main to temp: %w", err)
			}

			// Checkout fork again to compare against it
			if err := gitops.Checkout(localDir, forkName); err != nil {
				return fmt.Errorf("failed to checkout fork %s: %w", forkName, err)
			}

			fmt.Println("Template Status (Main vs Fork):")
			// Compare Temp (Desired Fork State) vs Local (Actual Fork State)
			// Note: We are comparing 'tempDir' (Source) vs 'localDir' (Target)
			if err := compareDirs(tempDir, localDir, ignorePatterns, checkFlagAll); err != nil {
				return err
			}
		}

		return nil
	},
}

func compareDirs(sourceDir, targetDir string, ignores []string, checkAll bool) error {
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	// 1. Walk Source
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

		// Check existence
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			fmt.Printf("%s\t%s\n", green("N"), rel)
			return nil
		}

		// Compare
		if areFilesDifferent(path, targetPath) {
			fmt.Printf("%s\t%s\n", yellow("M"), rel)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// 2. Walk Target (if --all)
	// If checkAll is true, check for files in Target that are NOT in Source (Deleted in Source/New in Target?)
	// Check Command logic: "D ... if file exists in home folder (target), but not in branch (source)"
	if checkAll {
		err := filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// Should we skip .git in home too? Probably.
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}

			rel, err := filepath.Rel(targetDir, path)
			if err != nil {
				return err
			}

			if isIgnored(rel, ignores) {
				return nil
			}

			sourcePath := filepath.Join(sourceDir, rel)
			if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
				fmt.Printf("%s\t%s\n", red("D"), rel)
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func isIgnored(relPath string, patterns []string) bool {
	for _, p := range patterns {
		matched, _ := filepath.Match(p, relPath)
		if matched {
			return true
		}
		// Match directories with wildcard support (simple prefix match for dirs?)
		// If ignore is "tmp/*", and relPath is "tmp/foo", match?
		// "tmp/*" matches "tmp/foo" via filepath.Match?
		// No, "*" does not match separator in filepath.Match.
		// So "tmp/*" matches "tmp/file", but check behavior.
		// If user wants to ignore dir, usually they put "tmp" or "tmp/" or "tmp/*".
		// To be safe, if we are inside an ignored dir, we should likely ignore.
		// But filepath.WalkDir descends automatically. We should handle ignore logic robustly if possible.
		// Current logic: simple Filepath Match.

		// Check against dir parts for robustness?
		// For now let's stick to standard filepath.Match per Go docs.
	}
	return false
}

func areFilesDifferent(pathA, pathB string) bool {
	cA, err := os.ReadFile(pathA)
	if err != nil {
		return true
	}
	cB, err := os.ReadFile(pathB)
	if err != nil {
		return true
	}
	return !bytes.Equal(cA, cB)
}

func init() {
	rootCmd.AddCommand(checkCmd)
	checkCmd.Flags().BoolVar(&checkFlagLocal, "local", false, "do not pull from remote")
	checkCmd.Flags().BoolVar(&checkFlagFull, "full", false, "check against main branch templates")
	checkCmd.Flags().BoolVar(&checkFlagAll, "all", false, "show files present in home but missing in repo")
}
