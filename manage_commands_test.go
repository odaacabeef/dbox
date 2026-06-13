package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanLocalFilesRecursive(t *testing.T) {
	root := t.TempDir()
	// Build a tree exercising recursion plus the skip rules:
	//   kick.wav            -> included
	//   notes.txt           -> wrong type
	//   .hidden.wav         -> hidden file
	//   drums/snare.wav     -> included (nested)
	//   drums/loops/hat.wav -> included (deeper)
	//   .git/ignored.wav    -> inside a hidden dir
	for _, f := range []string{
		"kick.wav",
		"notes.txt",
		".hidden.wav",
		"drums/snare.wav",
		"drums/loops/hat.wav",
		".git/ignored.wav",
	} {
		path := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &DboxConfig{Remote: "/x", FileTypes: []string{"wav"}}
	items, err := scanLocalFiles(root, cfg)
	if err != nil {
		t.Fatalf("scanLocalFiles: %v", err)
	}

	var got []string
	for _, it := range items {
		got = append(got, it.Rel)
	}
	// scanLocalFiles already returns items sorted by Rel.
	want := []string{"drums/loops/hat.wav", "drums/snare.wav", "kick.wav"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("scanned = %v, want %v", got, want)
	}
}
