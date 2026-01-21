package processor

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"

	"github.com/joho/godotenv"
)

// Reify walks the root directory and applies the template to each file.
// If dryRun is true, it only checks for errors and does not write to files.
func Reify(root string, data map[string]any, secretFn func(string) (string, error), dryRun bool) error {
	// First pass: Dry-run/Validation
	err := walkAndProcess(root, data, secretFn, true)
	if err != nil {
		return fmt.Errorf("dry-run failed: %w", err)
	}

	if dryRun {
		return nil
	}

	// Second pass: Execution
	return walkAndProcess(root, data, secretFn, false)
}

// ReifyFile processes a single file from sourcePath and writes it to destPath.
func ReifyFile(sourcePath, destPath string, data map[string]any, secretFn func(string) (string, error)) error {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}

	// Make sure dest directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create dest dir: %w", err)
	}

	return renderAndWrite(sourcePath, destPath, content, data, secretFn, false)
}

func walkAndProcess(root string, data map[string]any, secretFn func(string) (string, error), dryRun bool) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		// Process file: source and dest are the same for recursive reify
		return processFile(path, data, secretFn, dryRun)
	})
}

func processFile(path string, data map[string]any, secretFn func(string) (string, error), dryRun bool) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return renderAndWrite(path, path, content, data, secretFn, dryRun)
}

func renderAndWrite(name, destPath string, content []byte, data map[string]any, secretFn func(string) (string, error), dryRun bool) error {
	// Define FuncMap
	funcMap := template.FuncMap{
		"secret": secretFn,
	}

	// Parse template
	tmpl, err := template.New(filepath.Base(name)).Funcs(funcMap).Option("missingkey=error").Parse(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if dryRun {
		if err := tmpl.Execute(io.Discard, data); err != nil {
			return fmt.Errorf("failed to execute template %s: %w", name, err)
		}
		return nil
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template %s: %w", name, err)
	}

	// Overwrite file
	perm := os.FileMode(0644)
	if info, err := os.Stat(destPath); err == nil {
		perm = info.Mode()
	}

	return os.WriteFile(destPath, buf.Bytes(), perm)
}

// Helper to load env
func LoadEnv(dir string) (map[string]string, error) {
	return godotenv.Read(filepath.Join(dir, ".env"))
}

// GetSecretFn returns a function that retrieves secrets from the .env file at envPath.
// If required is true, missing .env or missing keys will return an error.
// If required is false, missing .env is ignored (empty secrets), but missing keys still error if called?
// Wait, original logic:
// Reify: if --secret is false, return {{ "key" | secret }} literal.
// Reify: if --secret is true, load .env. If .env fails, error. If key missing, error.
// Edit: always loads .env (if present?). If key missing, error.
// The "required" param here might map to "useSecret" flag in reify?
// Actually reify logic is: if !useSecret, return formatted string.
// So GetSecretFn should probably handle the extraction of values from the map only.
// Consolidating:
// 1. Load .env (handled by caller or helper?). Reify loads from targetDir, Edit from homeDir.
// 2. Closure to lookup key.
// Let's make GetSecretFn take the envMap or the envPath?
// Reify: "if --secret passed... read .env at targetPath".
// Edit: "read .env at homeDir".
// So passing envPath seems correct.

func GetSecretFn(envPath string, required bool) (func(string) (string, error), error) {
	var envMap map[string]string
	var err error

	if required {
		envMap, err = godotenv.Read(envPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read .env at %s: %w", envPath, err)
		}
	} else {
		// If not required, try to read but ignore not exists
		envMap, err = godotenv.Read(envPath)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read .env at %s: %w", envPath, err)
		}
	}

	return func(key string) (string, error) {
		// If map is nil (file didn't exist and not required), or key missing:
		val, ok := envMap[key]
		if !ok {
			if required {
				return "", fmt.Errorf("secret key %q not found in .env at %s", key, envPath)
			}
			// If not required (e.g. edit command might allow partial validation?)
			// But wait, Reify command if !useSecret returns the literal template string.
			// That logic is specific to "generating" vs "reverting" or "dry-run"?
			// No, reify without secret flag generates files WITH the secret tag preserved.
			// This logic seems specific to reify command's flag.
			// The requested refactor is to unify "how secrets are retrieved".
			// Let's assume GetSecretFn returns the lookup function.
			// Reify logic for !useSecret should stay in Reify command?
			// The user said: "The secretFn function is actually a placeholder... Can you refactor so that we use the same version in the reify and the edit?"

			// So let's implement the "Look up key in loaded .env" part as the shared logic.
			return "", fmt.Errorf("secret key %q not found in .env at %s", key, envPath)
		}
		return val, nil
	}, nil
}
