package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Reference hashes were computed independently with openssl using the Dropbox
// content-hash algorithm (4 MiB blocks → sha256 each → concat raw digests →
// sha256 → hex).
func TestDropboxContentHash(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "single block",
			data: []byte("hello world"),
			want: "bc62d4b80d9e36da29c16c5d4d9f11731f36052c72401a76c23c0fb5a9b74423",
		},
		{
			name: "spans block boundary", // 5 MiB = one 4 MiB block + one 1 MiB block
			data: bytes.Repeat([]byte("a"), 5*1024*1024),
			want: "31eebee77ad19453ca9817312ea1a938a001f5507fda7c721601319ad84aa857",
		},
		{
			name: "empty file",
			data: []byte{},
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f")
			if err := os.WriteFile(path, tc.data, 0644); err != nil {
				t.Fatalf("write temp file: %v", err)
			}
			got, err := dropboxContentHash(path)
			if err != nil {
				t.Fatalf("dropboxContentHash: %v", err)
			}
			if got != tc.want {
				t.Errorf("hash mismatch\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}
