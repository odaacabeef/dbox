package main

import (
	"os"
	"path/filepath"
)

// Config holds application configuration
type Config struct {
	DownloadPath string
}

// LoadConfig loads configuration. Dropbox credentials are handled separately
// (see auth.go); this just resolves filesystem settings.
func LoadConfig() (*Config, error) {
	dlpath, err := getDefaultDownloadPath()
	if err != nil {
		return nil, err
	}
	return &Config{DownloadPath: dlpath}, nil
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
