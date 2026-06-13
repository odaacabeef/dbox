package main

import "testing"

func TestCredentialsFromEnv(t *testing.T) {
	t.Setenv(envAppKey, "key")
	t.Setenv(envAppSecret, "secret")
	t.Setenv(envRefreshToken, "refresh")

	k, s, r, err := credentials()
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if k != "key" || s != "secret" || r != "refresh" {
		t.Errorf("got (%q, %q, %q), want (key, secret, refresh)", k, s, r)
	}
}

func TestCredentialsMissing(t *testing.T) {
	t.Setenv(envAppKey, "key")
	t.Setenv(envAppSecret, "secret")
	t.Setenv(envRefreshToken, "") // unset

	if _, _, _, err := credentials(); err == nil {
		t.Error("expected an error when a credential is missing")
	}
}

func TestFormatCredentialExports(t *testing.T) {
	got := formatCredentialExports("key", "secret", "refresh")
	want := "export DROPBOX_APP_KEY='key'\n" +
		"export DROPBOX_APP_SECRET='secret'\n" +
		"export DROPBOX_REFRESH_TOKEN='refresh'\n"
	if got != want {
		t.Errorf("formatCredentialExports =\n%q\nwant\n%q", got, want)
	}
}
