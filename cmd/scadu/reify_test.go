package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create temp dir and fles
func setupTestDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "scadu-test-*")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// Reset viper
func resetViper() {
	viper.Reset()
}

func TestReifyCommand_SecretFlagBehavior(t *testing.T) {
	// Tests:
	// 1. --secret=false: preserves string
	// 2. --secret=true: loads .env from root and replaces string
	// 3. --secret=true: fails if .env missing
	// 4. --secret=true: fails if secret key missing

	tests := []struct {
		name          string
		secretFlag    bool
		createEnv     bool
		envContent    string
		template      string
		expected      string
		expectError   bool
		errorContains string
	}{
		{
			name:        "Secret False Preserves Tag",
			secretFlag:  false,
			createEnv:   false,
			template:    `Val: {{ "KEY" | secret }}`,
			expected:    `Val: {{ "KEY" | secret }}`,
			expectError: false,
		},
		{
			name:        "Secret True Replaces Tag",
			secretFlag:  true,
			createEnv:   true,
			envContent:  "KEY=MySecret",
			template:    `Val: {{ "KEY" | secret }}`,
			expected:    `Val: MySecret`,
			expectError: false,
		},
		{
			name:          "Secret True Fails Missing Env",
			secretFlag:    true,
			createEnv:     false,
			template:      `Val: {{ "KEY" | secret }}`,
			expectError:   true,
			errorContains: "failed to read .env",
		},
		{
			name:          "Secret True Fails Missing Key",
			secretFlag:    true,
			createEnv:     true,
			envContent:    "OTHER=MySecret",
			template:      `Val: {{ "KEY" | secret }}`,
			expectError:   true,
			errorContains: "secret key \"KEY\" not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetViper()
			rootDir := setupTestDir(t)

			// Setup files
			tmplPath := filepath.Join(rootDir, "file.txt")
			err := os.WriteFile(tmplPath, []byte(tc.template), 0644)
			require.NoError(t, err)

			if tc.createEnv {
				envPath := filepath.Join(rootDir, ".env")
				err := os.WriteFile(envPath, []byte(tc.envContent), 0644)
				require.NoError(t, err)
			}

			// Setup Command
			// Since reifyCmd is a global var in package main (shared state), we have to be careful.
			// But we are running in a separate test function.
			// Reset flags?
			// secretFlag is a var. We can set it manually or via SetArgs if we define flags in Init.
			// reifyCmd is defined in reify.go and flag is bound in init().
			// Cobra flags parse into the var.

			// We need to simulate the execution.
			// The simplest way to test specific logic is running the RunE function validation logic directly if possible,
			// or executing the command.

			cmd := rootCmd
			// Use full command invocation including "reify"
			args := []string{"reify", rootDir, fmt.Sprintf("--secret=%v", tc.secretFlag)}
			cmd.SetArgs(args)

			// EXECUTE
			err = cmd.Execute()

			if tc.expectError {
				require.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				// Verify file NOT changed (atomicity)
				content, _ := os.ReadFile(tmplPath)
				assert.Equal(t, tc.template, string(content), "File should not change on error")
			} else {
				require.NoError(t, err)
				// Verify content
				content, _ := os.ReadFile(tmplPath)
				assert.Equal(t, tc.expected, string(content))
			}
		})
	}
}

func TestReifyCommand_EnvLocationStrictness(t *testing.T) {
	// Ambiguity 1: ".env file must be in the directory passed as parameter"
	// Verify that if we have nested directories, it still uses the root .env

	rootDir := setupTestDir(t)
	subDir := filepath.Join(rootDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	// Root .env
	err = os.WriteFile(filepath.Join(rootDir, ".env"), []byte("KEY=RootSecret"), 0644)
	require.NoError(t, err)

	// Subdir .env (should be IGNORED)
	err = os.WriteFile(filepath.Join(subDir, ".env"), []byte("KEY=SubSecret"), 0644)
	require.NoError(t, err)

	// Template in subdir
	tmplPath := filepath.Join(subDir, "file.txt")
	err = os.WriteFile(tmplPath, []byte(`{{ "KEY" | secret }}`), 0644)
	require.NoError(t, err)

	// Run command targeting rootDir
	resetViper()
	cmd := rootCmd
	cmd.SetArgs([]string{"reify", rootDir, "--secret=true"})

	err = cmd.Execute()
	require.NoError(t, err)

	content, _ := os.ReadFile(tmplPath)
	assert.Equal(t, "RootSecret", string(content), "Should use root .env, not subdir .env")
}
