package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
)

const (
	authURL      = "https://www.dropbox.com/oauth2/authorize"
	tokenURL     = "https://api.dropboxapi.com/oauth2/token"
	loopbackAddr = "127.0.0.1:53682"
	redirectURL  = "http://localhost:53682/"

	envAppKey       = "DROPBOX_APP_KEY"
	envAppSecret    = "DROPBOX_APP_SECRET"
	envRefreshToken = "DROPBOX_REFRESH_TOKEN"
)

// oauthConfig builds the OAuth2 config for the Dropbox confidential client.
func oauthConfig(appKey, appSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     appKey,
		ClientSecret: appSecret,
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:   authURL,
			TokenURL:  tokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

// credentials reads the Dropbox credentials from the environment. They are
// typically sourced from an encrypted store (e.g. `. <(pass …)`); nothing is
// read from or written to disk.
func credentials() (appKey, appSecret, refreshToken string, err error) {
	appKey = os.Getenv(envAppKey)
	appSecret = os.Getenv(envAppSecret)
	refreshToken = os.Getenv(envRefreshToken)
	if appKey == "" || appSecret == "" || refreshToken == "" {
		return "", "", "", fmt.Errorf(
			"missing Dropbox credentials; set %s, %s, and %s (run \"dbox login\" once to obtain them)",
			envAppKey, envAppSecret, envRefreshToken)
	}
	return appKey, appSecret, refreshToken, nil
}

// formatCredentialExports renders the credentials as sourceable shell exports.
func formatCredentialExports(appKey, appSecret, refreshToken string) string {
	return fmt.Sprintf("export %s='%s'\nexport %s='%s'\nexport %s='%s'\n",
		envAppKey, appKey,
		envAppSecret, appSecret,
		envRefreshToken, refreshToken)
}

// runLogin performs the one-time OAuth flow and prints sourceable credential
// exports to stdout (status messages go to stderr). It writes nothing to disk,
// so the output can be piped straight into an encrypted store.
func runLogin() error {
	appKey := os.Getenv(envAppKey)
	appSecret := os.Getenv(envAppSecret)
	if appKey == "" || appSecret == "" {
		return fmt.Errorf("set %s and %s (from your Dropbox app's Settings) before running \"dbox login\"", envAppKey, envAppSecret)
	}

	state, err := randomState()
	if err != nil {
		return err
	}

	cfg := oauthConfig(appKey, appSecret)

	// Start the loopback server before opening the browser so the redirect
	// can't race ahead of the listener.
	listener, err := net.Listen("tcp", loopbackAddr)
	if err != nil {
		return fmt.Errorf("could not start local server on %s (is another login in progress?): %w", loopbackAddr, err)
	}

	type result struct {
		code string
		err  error
	}
	results := make(chan result, 1)

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			fmt.Fprintf(w, "Authorization failed: %s. You can close this tab.", e)
			results <- result{err: fmt.Errorf("authorization denied: %s", e)}
			return
		}
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			results <- result{err: fmt.Errorf("state mismatch (possible CSRF)")}
			return
		}
		fmt.Fprint(w, "dbox is now authorized. You can close this tab.")
		results <- result{code: q.Get("code")}
	})}
	go srv.Serve(listener)
	defer srv.Close()

	url := cfg.AuthCodeURL(state, oauth2.SetAuthURLParam("token_access_type", "offline"))
	fmt.Fprintln(os.Stderr, "Opening your browser to authorize dbox…")
	fmt.Fprintf(os.Stderr, "If it doesn't open, visit:\n\n  %s\n\n", url)
	if err := openBrowser(url); err != nil {
		fmt.Fprintf(os.Stderr, "(couldn't open a browser automatically: %v)\n", err)
	}

	var res result
	select {
	case res = <-results:
	case <-time.After(5 * time.Minute):
		return fmt.Errorf("timed out waiting for authorization")
	}
	if res.err != nil {
		return res.err
	}

	tok, err := cfg.Exchange(context.Background(), res.code)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}
	if tok.RefreshToken == "" {
		return fmt.Errorf("Dropbox did not return a refresh token (was token_access_type=offline honored?)")
	}

	fmt.Fprintln(os.Stderr, "\nLogged in. Store these securely (e.g. with pass) and source them before running dbox:")
	fmt.Print(formatCredentialExports(appKey, appSecret, tok.RefreshToken))
	return nil
}

// randomState returns a random hex string for the OAuth state parameter.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
