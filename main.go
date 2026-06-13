package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Load configuration (the access token is required in both modes).
	config, err := LoadConfig()
	if err != nil {
		fmt.Printf("Configuration error: %v\n", err)
		os.Exit(1)
	}

	// With a config-file argument we enter management mode (push local files
	// up to Dropbox); otherwise we open the browse/download TUI.
	var m tea.Model
	if len(os.Args) >= 2 {
		m = newManageProgram(config, os.Args[1])
	} else {
		// Ensure download directory exists
		if err := config.EnsureDownloadPath(); err != nil {
			fmt.Printf("Error creating download directory: %v\n", err)
			os.Exit(1)
		}
		m = initialModel(config)
	}

	// Create and run the program
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}

// newManageProgram loads the management-mode config and current directory,
// exiting with a clear message on any error before the TUI starts.
func newManageProgram(config *Config, configPath string) tea.Model {
	dboxCfg, err := LoadDboxConfig(configPath)
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error determining current directory: %v\n", err)
		os.Exit(1)
	}

	return initialManageModel(config, dboxCfg, cwd)
}
