package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/suderio/scadufax/pkg/processor"
)

// reifyCmd represents the reify command
var reifyCmd = &cobra.Command{
	Use:   "reify [path]",
	Short: "Apply templates in the given directory",
	Long: `Applies the values found in the configuration files to all the files in the directory, recursively.

Values are read from config.toml and local.toml in ~/.config/scadufax/.
If --secret is passed, values for the secret command are read from .env in the target directory.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := args[0]

		// Ensure target exists
		info, err := os.Stat(targetPath)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}

		// Prepare data
		data := viper.AllSettings()

		// Get flag value
		useSecret, err := cmd.Flags().GetBool("secret")
		if err != nil {
			return err
		}

		// Prepare secret handler
		var secretFn func(string) (string, error)

		if useSecret {
			// Load .env from the target directory (or parent if target is a file)
			envDir := targetPath
			if !info.IsDir() {
				envDir = filepath.Dir(targetPath)
			}
			envPath := filepath.Join(envDir, ".env")

			// Requirement: If --secret is true, we strictly verify .env exists.
			var err error
			secretFn, err = processor.GetSecretFn(envPath, true)
			if err != nil {
				return err
			}
		} else {
			// Default behavior: preserve template tags
			secretFn = func(key string) (string, error) {
				return fmt.Sprintf("{{ %q | secret }}", key), nil
			}
		}

		// Run Processor
		return processor.Reify(targetPath, data, secretFn, false)
	},
}

func init() {
	rootCmd.AddCommand(reifyCmd)
	reifyCmd.Flags().Bool("secret", false, "enable secret processing using .env")
}
