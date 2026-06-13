package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// dropboxBlockSize is the block size Dropbox uses when computing a file's
// content hash.
const dropboxBlockSize = 4 * 1024 * 1024 // 4 MB

// dropboxContentHash computes the Dropbox content hash for a local file.
//
// The algorithm (https://www.dropbox.com/developers/reference/content-hash):
// split the file into 4 MB blocks, take the SHA-256 of each block, concatenate
// those raw (binary) digests in order, then take the SHA-256 of the
// concatenation and hex-encode the result. This lets us compare a local file
// against the ContentHash returned in Dropbox file metadata to decide whether
// an upload is needed.
func dropboxContentHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	outer := sha256.New()
	buf := make([]byte, dropboxBlockSize)

	for {
		n, err := io.ReadFull(f, buf)
		if n > 0 {
			blockSum := sha256.Sum256(buf[:n])
			// Write the raw digest (not its hex encoding) into the outer hash.
			outer.Write(blockSum[:])
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// Reached the end of the file; the final (possibly short) block
			// was handled above.
			break
		}
		if err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(outer.Sum(nil)), nil
}
