package main

import (
	"context"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/sharing"
	"golang.org/x/oauth2"
)

// newConfig builds the SDK config from the credentials in the environment. It
// returns an auto-refreshing HTTP client (built from the refresh token + app
// key/secret), so access tokens are minted and renewed transparently.
func newConfig() (dropbox.Config, error) {
	appKey, appSecret, refreshToken, err := credentials()
	if err != nil {
		return dropbox.Config{}, err
	}
	cfg := oauthConfig(appKey, appSecret)
	client := cfg.Client(context.Background(), &oauth2.Token{RefreshToken: refreshToken})
	return dropbox.Config{Client: client}, nil
}

// newFilesClient builds a Dropbox files client from stored credentials.
func newFilesClient() (files.Client, error) {
	cfg, err := newConfig()
	if err != nil {
		return nil, err
	}
	return files.New(cfg), nil
}

// newSharingClient builds a Dropbox sharing client from stored credentials.
func newSharingClient() (sharing.Client, error) {
	cfg, err := newConfig()
	if err != nil {
		return nil, err
	}
	return sharing.New(cfg), nil
}
