package main

import (
	"fmt"
	"os"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/sharing"
)

// dropboxToken returns the Dropbox access token from the environment, or an
// error if it isn't set. It is the single source of the token for all Dropbox
// API calls.
func dropboxToken() (string, error) {
	token := os.Getenv("DROPBOX_ACCESS_TOKEN")
	if token == "" {
		return "", fmt.Errorf("DROPBOX_ACCESS_TOKEN environment variable not set")
	}
	return token, nil
}

// newFilesClient builds a Dropbox files client from the configured token.
func newFilesClient() (files.Client, error) {
	token, err := dropboxToken()
	if err != nil {
		return nil, err
	}
	return files.New(dropbox.Config{Token: token}), nil
}

// newSharingClient builds a Dropbox sharing client from the configured token.
func newSharingClient() (sharing.Client, error) {
	token, err := dropboxToken()
	if err != nil {
		return nil, err
	}
	return sharing.New(dropbox.Config{Token: token}), nil
}
