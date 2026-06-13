package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
)

const (
	// uploadSessionThreshold is the size at which we switch from a single
	// Upload call to a chunked upload session. The simple endpoint rejects
	// files larger than 150 MB, so stay well under it.
	uploadSessionThreshold = 140 * 1024 * 1024 // 140 MB

	// uploadChunkSize is the number of bytes sent per upload-session request.
	// Must be under 150 MB; smaller keeps memory bounded.
	uploadChunkSize = 16 * 1024 * 1024 // 16 MB
)

// scanLocalFiles returns the top-level files in cwd that match the configured
// file types, sorted by name. Subdirectories and hidden files are skipped.
func scanLocalFiles(cwd string, cfg *DboxConfig) ([]ManageFileItem, error) {
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return nil, err
	}

	var items []ManageFileItem
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue // skip hidden files
		}
		if !cfg.matchesFileType(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, ManageFileItem{
			Name:   name,
			Path:   filepath.Join(cwd, name),
			Size:   info.Size(),
			Status: StatusPending,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

// pushFilesCmd uploads each file to the configured remote folder, skipping any
// whose content already matches what's on Dropbox. It mirrors downloadFilesCmd:
// the whole batch runs synchronously and reports a single completion message.
func pushFilesCmd(cfg *DboxConfig, items []ManageFileItem) tea.Cmd {
	return func() tea.Msg {
		token := os.Getenv("DROPBOX_ACCESS_TOKEN")
		if token == "" {
			return ErrorMsg{Error: "DROPBOX_ACCESS_TOKEN environment variable not set"}
		}
		dbx := files.New(dropbox.Config{Token: token})

		if err := ensureRemoteFolder(dbx, cfg.Remote); err != nil {
			return ErrorMsg{Error: fmt.Sprintf("Failed to create remote folder %s: %v", cfg.Remote, err)}
		}

		var uploaded, skipped, errs []string

		for _, item := range items {
			// Dropbox paths are always "/"-separated and are not OS paths.
			remotePath := cfg.Remote + "/" + item.Name

			localHash, err := dropboxContentHash(item.Path)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: hashing failed: %v", item.Name, err))
				continue
			}

			// Skip the upload if the remote file already has the same content.
			meta, err := dbx.GetMetadata(files.NewGetMetadataArg(remotePath))
			if err != nil {
				if !isNotFoundErr(err) {
					errs = append(errs, fmt.Sprintf("%s: lookup failed: %v", item.Name, err))
					continue
				}
				// not found: fall through and upload as a new file
			} else if fileMeta, ok := meta.(*files.FileMetadata); ok {
				if fileMeta.ContentHash == localHash {
					skipped = append(skipped, item.Name)
					continue
				}
			}

			if item.Size >= uploadSessionThreshold {
				err = uploadFileSession(dbx, item.Path, remotePath, localHash, item.Size)
			} else {
				err = uploadFile(dbx, item.Path, remotePath, localHash)
			}
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", item.Name, err))
				continue
			}
			uploaded = append(uploaded, item.Name)
		}

		return UploadCompleteMsg{Uploaded: uploaded, Skipped: skipped, Errors: errs}
	}
}

// ensureRemoteFolder creates the remote folder, treating an "already exists"
// conflict as success so repeated pushes don't error.
func ensureRemoteFolder(dbx files.Client, remote string) error {
	_, err := dbx.CreateFolderV2(files.NewCreateFolderArg(remote))
	if err == nil {
		return nil
	}
	if apiErr, ok := err.(files.CreateFolderV2APIError); ok {
		if apiErr.EndpointError != nil &&
			apiErr.EndpointError.Path != nil &&
			apiErr.EndpointError.Path.Tag == "conflict" {
			return nil // folder already exists
		}
	}
	return err
}

// uploadFile uploads a file in a single request. Use only for files under
// uploadSessionThreshold. The content hash is passed so Dropbox verifies
// integrity server-side, and overwrite mode replaces any existing file.
func uploadFile(dbx files.Client, localPath, remotePath, contentHash string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	arg := files.NewUploadArg(remotePath)
	arg.Mode = overwriteMode()
	arg.ContentHash = contentHash
	_, err = dbx.Upload(arg, f)
	return err
}

// uploadFileSession uploads a large file in chunks via an upload session.
func uploadFileSession(dbx files.Client, localPath, remotePath, contentHash string, size int64) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, uploadChunkSize)

	// Start the session with the first chunk.
	n, err := readChunk(f, buf)
	if err != nil {
		return err
	}
	res, err := dbx.UploadSessionStart(files.NewUploadSessionStartArg(), bytes.NewReader(buf[:n]))
	if err != nil {
		return err
	}
	sessionID := res.SessionId
	var offset = uint64(n)

	// Append the remaining chunks, finishing on the last one.
	for offset < uint64(size) {
		n, err := readChunk(f, buf)
		if err != nil {
			return err
		}
		cursor := files.NewUploadSessionCursor(sessionID, offset)
		if offset+uint64(n) >= uint64(size) {
			return finishSession(dbx, cursor, remotePath, contentHash, buf[:n])
		}
		appendArg := files.NewUploadSessionAppendArg(cursor)
		if err := dbx.UploadSessionAppendV2(appendArg, bytes.NewReader(buf[:n])); err != nil {
			return err
		}
		offset += uint64(n)
	}

	// Reached only when the file fit in the first chunk; finish with no data.
	cursor := files.NewUploadSessionCursor(sessionID, offset)
	return finishSession(dbx, cursor, remotePath, contentHash, nil)
}

// finishSession commits an upload session at remotePath, overwriting any
// existing file.
func finishSession(dbx files.Client, cursor *files.UploadSessionCursor, remotePath, contentHash string, content []byte) error {
	commit := files.NewCommitInfo(remotePath)
	commit.Mode = overwriteMode()
	arg := files.NewUploadSessionFinishArg(cursor, commit)
	arg.ContentHash = contentHash
	_, err := dbx.UploadSessionFinish(arg, bytes.NewReader(content))
	return err
}

// overwriteMode returns a WriteMode that overwrites any existing file.
func overwriteMode() *files.WriteMode {
	return &files.WriteMode{Tagged: dropbox.Tagged{Tag: files.WriteModeOverwrite}}
}

// readChunk reads up to len(buf) bytes, returning the count read and treating
// end-of-file (full or partial final block) as a non-error.
func readChunk(r io.Reader, buf []byte) (int, error) {
	n, err := io.ReadFull(r, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return n, nil
	}
	return n, err
}

// isNotFoundErr reports whether a GetMetadata error means the path doesn't
// exist on Dropbox (as opposed to a real failure).
func isNotFoundErr(err error) bool {
	apiErr, ok := err.(files.GetMetadataAPIError)
	if !ok {
		return false
	}
	return apiErr.EndpointError != nil &&
		apiErr.EndpointError.Path != nil &&
		apiErr.EndpointError.Path.Tag == "not_found"
}
