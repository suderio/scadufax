package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"github.com/suderio/scadufax/pkg/gitops"
)

// Defaults
const (
	defaultRepo = "dotfiles"
)

var (
	flagConfigDir string
	flagLocalDir  string
	flagHomeDir   string
	flagFork      string
)

// Config structures for toml encoding
type ScadufaxConfig struct {
	ConfigDir string `toml:"config_dir,omitempty"`
	LocalDir  string `toml:"local_dir,omitempty"`
	HomeDir   string `toml:"home_dir,omitempty"`
	Fork      string `toml:"fork"`
	Confirm   *bool  `toml:"confirm,omitempty"`
}

type RootConfig struct {
	Name   string   `toml:"name,omitempty"`
	Email  string   `toml:"email,omitempty"`
	Secret string   `toml:"secret,omitempty"`
	Ignore []string `toml:"ignore,omitempty"`
}

type ConfigFile struct {
	Scadufax ScadufaxConfig `toml:"scadufax"`
	Root     RootConfig     `toml:"root,omitempty"`
}

var initCmd = &cobra.Command{
	Use:   "init user[/repo]",
	Short: "Initialize configuration and dotfiles repository",
	Args:  cobra.RangeArgs(1, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse user/repo
		userRepo := args[0]
		parts := strings.Split(userRepo, "/")
		user := parts[0]
		repo := defaultRepo
		if len(parts) > 1 {
			repo = parts[1]
		}

		remoteURL := fmt.Sprintf("https://github.com/%s/%s.git", user, repo)

		// Determine defaults
		userConfigDir, _ := os.UserConfigDir()
		defaultConfigDir := filepath.Join(userConfigDir, "scadufax")

		userHomeDir, _ := os.UserHomeDir()
		// Clarification was: local-dir defaults to ~/.local/share/scadufax on Linux
		defaultLocalDir := filepath.Join(userHomeDir, ".local", "share", "scadufax")

		hostname, _ := os.Hostname()

		// Prepare Config
		cfg := ScadufaxConfig{
			Fork: flagFork,
		}
		if flagFork == "" {
			cfg.Fork = hostname
		}

		// Handle dirs
		targetConfigDir := flagConfigDir
		if targetConfigDir == "" {
			targetConfigDir = defaultConfigDir
		} else {
			// If provided and different from default, write to config
			if targetConfigDir != defaultConfigDir {
				cfg.ConfigDir = targetConfigDir
			}
		}

		targetLocalDir := flagLocalDir
		if targetLocalDir == "" {
			targetLocalDir = defaultLocalDir
		} else {
			if targetLocalDir != defaultLocalDir {
				cfg.LocalDir = targetLocalDir
			}
		}

		targetHomeDir := flagHomeDir
		if targetHomeDir == "" {
			targetHomeDir = userHomeDir
		} else {
			if targetHomeDir != userHomeDir {
				cfg.HomeDir = targetHomeDir
			}
		}

		// Create directories
		if err := os.MkdirAll(targetConfigDir, 0755); err != nil {
			return fmt.Errorf("failed to create config dir: %w", err)
		}
		if err := os.MkdirAll(targetLocalDir, 0755); err != nil {
			return fmt.Errorf("failed to create local dir: %w", err)
		}

		// Write config.toml
		fullConfig := ConfigFile{
			Scadufax: cfg,
			// Root: ... not setting default name/email here, usually empty or prompted?
			// Request says "config.toml has only one mandatory field, fork".
		}

		configPath := filepath.Join(targetConfigDir, "config.toml")
		f, err := os.Create(configPath)
		if err != nil {
			return fmt.Errorf("failed to create config.toml: %w", err)
		}
		encoder := toml.NewEncoder(f)
		if err := encoder.Encode(fullConfig); err != nil {
			f.Close()
			return fmt.Errorf("failed to encode config.toml: %w", err)
		}
		f.Close()

		// Write blank local.toml
		localPath := filepath.Join(targetConfigDir, "local.toml")
		if err := os.WriteFile(localPath, []byte(""), 0644); err != nil {
			return fmt.Errorf("failed to create local.toml: %w", err)
		}

		// Git Operations
		fmt.Printf("Initializing repository in %s...\n", targetLocalDir)
		if err := gitops.InitRepo(targetLocalDir, remoteURL); err != nil {
			return fmt.Errorf("failed to init repo: %w", err)
		}

		// Ensure Fork Branch
		if cfg.Fork != "" {
			fmt.Printf("Ensuring fork branch '%s'...\n", cfg.Fork)
			if err := gitops.CreateBranch(targetLocalDir, cfg.Fork); err != nil {
				return fmt.Errorf("failed to create fork branch: %w", err)
			}
		}

		fmt.Println("Initialization complete.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVar(&flagConfigDir, "config-dir", "", "config directory")
	initCmd.Flags().StringVar(&flagLocalDir, "local-dir", "", "local directory for repo")
	initCmd.Flags().StringVar(&flagHomeDir, "home-dir", "", "home directory")
	initCmd.Flags().StringVar(&flagFork, "fork", "", "fork name (defaults to hostname)")
}
