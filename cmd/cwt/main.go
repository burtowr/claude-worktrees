package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wburton/cwt/internal/tui"
)

func main() {
	// Find git repository root
	repoRoot, err := findGitRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please run cwt from within a git repository.\n")
		os.Exit(1)
	}

	// Check if claude is installed
	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: 'claude' command not found in PATH\n")
		fmt.Fprintf(os.Stderr, "Please install Claude Code first: https://claude.ai/code\n")
		os.Exit(1)
	}

	// Create and run TUI
	model, err := tui.NewModel(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// findGitRoot finds the root of the current git repository
func findGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		// Check current directory
		cwd, _ := os.Getwd()
		if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
			return cwd, nil
		}
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(output)), nil
}
