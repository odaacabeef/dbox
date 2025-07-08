package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds application configuration
type Config struct {
	DropboxAccessToken string
	DownloadPath       string
}

// LoadConfig loads configuration from environment variables and files
func LoadConfig() (*Config, error) {

	dlpath, err := getDefaultDownloadPath()
	if err != nil {
		return nil, err
	}

	config := &Config{
		DropboxAccessToken: os.Getenv("DROPBOX_ACCESS_TOKEN"),
		DownloadPath:       dlpath,
	}

	// Validate required configuration
	if config.DropboxAccessToken == "" {
		return nil, fmt.Errorf("DROPBOX_ACCESS_TOKEN environment variable is required")
	}

	return config, nil
}

// getDefaultDownloadPath returns the default download path
func getDefaultDownloadPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".dbox"), nil
}

// EnsureDownloadPath creates the download directory if it doesn't exist
func (c *Config) EnsureDownloadPath() error {
	return os.MkdirAll(c.DownloadPath, 0755)
}
