package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dbox.yaml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadDboxConfig(t *testing.T) {
	t.Run("valid with normalization", func(t *testing.T) {
		cfg, err := LoadDboxConfig(writeConfig(t, "remote: sequences/airy-dissonance/\nfile_types: [WAV, .aiff]\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Remote != "/sequences/airy-dissonance" {
			t.Errorf("remote = %q, want /sequences/airy-dissonance", cfg.Remote)
		}
		if len(cfg.FileTypes) != 2 || cfg.FileTypes[0] != "wav" || cfg.FileTypes[1] != "aiff" {
			t.Errorf("file_types = %v, want [wav aiff]", cfg.FileTypes)
		}
		if !cfg.matchesFileType("kick.WAV") || cfg.matchesFileType("notes.txt") {
			t.Errorf("matchesFileType behaved unexpectedly")
		}
	})

	t.Run("missing remote", func(t *testing.T) {
		if _, err := LoadDboxConfig(writeConfig(t, "file_types: [wav]\n")); err == nil {
			t.Error("expected error for missing remote")
		}
	})

	t.Run("empty file_types", func(t *testing.T) {
		if _, err := LoadDboxConfig(writeConfig(t, "remote: /x\n")); err == nil {
			t.Error("expected error for empty file_types")
		}
	})

	t.Run("unknown key rejected", func(t *testing.T) {
		if _, err := LoadDboxConfig(writeConfig(t, "remote: /x\nfile_types: [wav]\nfiletypes: [wav]\n")); err == nil {
			t.Error("expected error for unknown key")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if _, err := LoadDboxConfig(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
			t.Error("expected error for missing file")
		}
	})
}
