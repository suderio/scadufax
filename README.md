# Scadufax

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/suderio/scadufax)
![GitHub license](https://img.shields.io/github/license/suderio/scadufax)
![GitHub release (latest by date)](https://img.shields.io/github/v/release/suderio/scadufax)
[![Go Report Card](https://goreportcard.com/badge/github.com/suderio/scadufax)](https://goreportcard.com/report/github.com/suderio/scadufax)

**Scadufax** ("Scadu") is a robust, Git-centric dotfile manager designed for power users who want precise control over their configuration files across multiple machines. It treats your dotfiles repository as a template source, allowing you to reify configurations with machine-specific secrets and settings while maintaining a clean, synchronized Git history.

## TOC

- [About](#about)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Workflow](#workflow)
- [Philosophy](#philosophy)
- [Limitations](#limitations)
- [Build](#build)
- [Built with](#built-with)
- [Issue](#issue)
- [Contributing](#contributing)
- [License](#license)

## About

Scadufax addresses the "drift" problem in dotfile management. Instead of symlinking your home directory directly to a git repo (which risks committing secrets or machine-specific trash), Scadufax uses a "Reification" process. You edit templates in a secure repository, and Scadufax compiles ("reifies") them into your home directory, injecting secrets from a secure `.env` file and handling template logic.

## Installation

### From Binary
Download the latest pre-compiled binary for your architecture from the [Releases](https://github.com/suderio/scadufax/releases) page.

### From Source
```bash
go install github.com/suderio/scadufax/cmd/scadu@latest
```

## Configuration

Scadufax uses a TOML configuration file located at `~/.config/scadufax/config.toml`.

```toml
[scadufax]
# Local location of the dotfiles git repository
local_dir = "/home/user/.local/share/scadufax"
# Target directory for installation (usually your home)
home_dir = "/home/user"
# Name of the branch for this specific machine
fork = "laptop-work"
# Require confirmation for file deletions (default: true)
confirm = true

[root]
# Machine specific variables accessible in templates as {{ .root.name }}
name = "My Workstation"
email = "me@example.com"
# File patterns to ignore during check/list operations
ignore = ["*.tmp", ".DS_Store", "logs/*"]
```

## Usage

### `scadu init [repo-url]`
Initializes the Scadufax environment.
-   Clones the provided repository to the local storage.
-   Creates a machine-specific fork branch.
-   Generates a default configuration file.

### `scadu add [files...]`
Adds files from your home directory to the repository.
-   **Flags**:
    -   `--edit`: Opens the file in the repository after adding it, allowing you to secure secrets or template variables immediately.

### `scadu edit [files...]`
The core command. Opens the repository version of a file in your `$EDITOR`.
-   **Workflow**:
    1.  You edit the template (e.g., replace `password` with `{{ "MY_SECRET" | secret }}`).
    2.  On save/exit, Scadufax detects changes.
    3.  It **reifies** the file (injects values).
    4.  It installs the file to your home directory.
    5.  It commits the change to the repository with a unique `SCADUFAX_ID`.

### `scadu check`
Compares your home directory against the repository state.
-   **Flags**:
    -   `--local`: Compare against local repository state without pulling.
    -   `--full`: Compare against the `main` branch templates (reified) instead of the machine fork.
    -   `--all`: Show "deleted" (D) files that exist in the repo but are missing from home.
-   **Output**:
    -   `N` (Green): New file.
    -   `M` (Yellow): Modified file.
    -   `D` (Red): Deleted/Missing file.

### `scadu list`
Lists tracked files.
-   **Flags**:
    -   `--all`: Also list "UNMANAGED" files (files in home but not in the repository).
-   **Output**:
    -   `MISSING`: File present in repo but missing in home.
    -   `UNMANAGED`: File present in home but not in repo (requires `--all`).

### `scadu remove [files...]`
Removes files from the repository.
-   **Flags**:
    -   `--local`: Also delete the file from the home directory (prompts for confirmation unless `confirm=false`).

### `scadu update`
Synchronizes your machine with the upstream repository.
-   **Flags**:
    -   `--wait`: Loops and waits until the machine fork dominates the main branch state (useful for CI/CD or multi-machine sync).
-   **Process**:
    1.  Pushes `main`.
    2.  Pulls `fork`.
    3.  Compares `fork` vs `home` and prompts to apply changes.

### `scadu reify [file] [--dry-run]`
Manually processes a template file.
-   **Flags**:
    -   `--secret`: Enable secret injection using the `.env` file.

## Workflow

Scadufax relies on a "GitOps-for-Dotfiles" loop, potentially enhanced by CI/CD pipelines.

1.  **The Template (Main Branch)**: You edit agnostic blueprints.
    -   `scadu edit ~/.bashrc` -> Opens template in `main`.
    -   `scadu add ~/.vimrc` -> Adds to `main`.
2.  **The Reification (Pipeline/Process)**:
    -   When you `edit`, Scadufax locally reifies (compiles) the template to verify and install it immediately.
    -   Simultaneously, `scadu update` assumes an asynchronous pipeline (like GitHub Actions) detects changes in `main`, reifies them for specific machines, and pushes the result to your machine's **Fork Branch**.
3.  **The Reified State (Fork Branch)**:
    -   Your machine-specific branch (`laptop-work`) contains the *compiled* files (pure text, no template tags).
4.  **The Synchronization (Update)**:
    -   `scadu update --wait` ensures your fork is up-to-date with `main` (waiting for the pipeline).
    -   It then syncs the **Fork Branch** to your **Home Directory**.

## Philosophy

### The Trinity of State
Scadufax manages three distinct states of your configuration:

1.  **The Template**: The Logic. Living in the `main` branch, this is the source of truth. It contains `{{ .root.email }}` and `{{ "API_KEY" | secret }}`.
2.  **The Reified Branch**: The Reference. Living in your machine's `fork`, this is the *canonical* compiled state. It contains `me@example.com` and the actual API key (encrypted or injected).
3.  **The Workdir**: The Reality. Your actual Home Directory.

### Three-Way Comparison
Scadufax tools are designed to navigate the drift between these states:

-   **Workdir vs. Reified Branch** (`scadu check`):
    -   Detects *Local Drift*. Did you change a file manually in Home?
    -   If matches: Your home is in sync with what the system *thinks* it should be.
-   **Main Template vs. Reified Branch** (`scadu check --full`):
    -   Detects *Pipeline Drift*. Did the reification process fail? Is your fork outdated compared to the latest templates in main?
    -   Scadufax locally compiles `main` and compares it against `fork`.
-   **Workdir vs. Template** (`scadu edit`):
    -   The loop closer. You edit the *Template* to affect the *Workdir*.

### Reification
We believe your home directory should never contain git logic or template tags. It should contain plain, working configuration files. Reification is the bridge—a compilation step that turns "Code" (Templates) into "Artifacts" (Dotfiles).

## Limitations

-   **Synchronous Editing**: The `edit` command blocks until the editor closes. This is by design to capture the "after" state for committing.
-   **Conflict Resolution**: Merge conflicts in Git must be resolved manually in the repository directory.

## Build

```bash
# Standard build
go build -o scadu ./cmd/scadu

# Release build (cross-platform)
goreleaser release --snapshot --clean
```

## Built with

-   **Go** (Golang)
-   **Cobra** (CLI Framework)
-   **Viper** (Configuration)
-   **Go-Git** (Git operations)
-   **Fatih/Color** (UI)

## Issue

Please submit issues and feature requests on the [GitHub Issue Tracker](https://github.com/suderio/scadufax/issues).

## Contributing

1.  Fork the repository.
2.  Create your feature branch (`git checkout -b feature/amazing`).
3.  Commit your changes (`git commit -m 'Add amazing feature'`).
4.  Push to the branch (`git push origin feature/amazing`).
5.  Open a Pull Request.

## License

Distributed under the MIT License. See `LICENSE` for more information.
