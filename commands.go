package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
)

// loadFilesCmd returns a command that loads files from Dropbox
func loadFilesCmd(path string) tea.Cmd {
	return func() tea.Msg {
		// Get access token from environment
		accessToken := os.Getenv("DROPBOX_ACCESS_TOKEN")
		if accessToken == "" {
			return ErrorMsg{Error: "DROPBOX_ACCESS_TOKEN environment variable not set"}
		}

		// Create Dropbox client
		dbx := files.New(dropbox.Config{
			Token: accessToken,
		})

		// List files in the specified path
		arg := files.NewListFolderArg(path)
		if path == "" {
			arg = files.NewListFolderArg("")
		}

		result, err := dbx.ListFolder(arg)
		if err != nil {
			// Try to get more detailed error information
			return ErrorMsg{Error: fmt.Sprintf("Failed to load files from path '%s': %v", path, err)}
		}

		var fileItems []FileItem

		// Process entries
		for _, entry := range result.Entries {
			// Skip deleted files
			if _, ok := entry.(*files.DeletedMetadata); ok {
				continue
			}

			var item FileItem

			switch v := entry.(type) {
			case *files.FileMetadata:
				item = FileItem{
					Name:     v.Name,
					Path:     v.PathLower,
					IsFolder: false,
					Size:     int64(v.Size),
					Modified: v.ServerModified,
				}
			case *files.FolderMetadata:
				item = FileItem{
					Name:     v.Name,
					Path:     v.PathLower,
					IsFolder: true,
					Size:     0,
					Modified: time.Now(), // Folders don't have modification time in Dropbox API
				}
			default:
				continue
			}

			fileItems = append(fileItems, item)
		}

		// Sort files: folders first, then by name
		sort.Slice(fileItems, func(i, j int) bool {
			if fileItems[i].IsFolder != fileItems[j].IsFolder {
				return fileItems[i].IsFolder
			}
			return strings.ToLower(fileItems[i].Name) < strings.ToLower(fileItems[j].Name)
		})

		return FilesLoadedMsg{
			Files: fileItems,
			Path:  path,
		}
	}
}

// downloadFileCmd returns a command that downloads a file from Dropbox
func downloadFileCmd(path string, localPath string) tea.Cmd {
	return func() tea.Msg {
		// Get access token from environment
		accessToken := os.Getenv("DROPBOX_ACCESS_TOKEN")
		if accessToken == "" {
			return ErrorMsg{Error: "DROPBOX_ACCESS_TOKEN environment variable not set"}
		}

		// Create Dropbox client for files API
		dbx := files.New(dropbox.Config{
			Token: accessToken,
		})

		// Download file
		arg := files.NewDownloadArg(path)
		_, contents, err := dbx.Download(arg)
		if err != nil {
			return ErrorMsg{Error: fmt.Sprintf("Failed to download file: %v", err)}
		}
		defer contents.Close()

		// Read all content
		contentBytes, err := io.ReadAll(contents)
		if err != nil {
			return ErrorMsg{Error: fmt.Sprintf("Failed to read downloaded content: %v", err)}
		}

		// Write to local file
		err = os.WriteFile(localPath, contentBytes, 0644)
		if err != nil {
			return ErrorMsg{Error: fmt.Sprintf("Failed to write file: %v", err)}
		}

		return StatusMsg{Message: fmt.Sprintf("Downloaded %s to %s", path, localPath)}
	}
}

// downloadFilesCmd returns a command that downloads multiple files and folders
func downloadFilesCmd(fileItems []FileItem, config *Config) tea.Cmd {
	return func() tea.Msg {
		// Synchronously download files
		accessToken := os.Getenv("DROPBOX_ACCESS_TOKEN")
		if accessToken == "" {
			return ErrorMsg{Error: "DROPBOX_ACCESS_TOKEN environment variable not set"}
		}

		dbx := files.New(dropbox.Config{
			Token: accessToken,
		})

		downloadDir := config.DownloadPath
		var downloaded, skipped, errors []string

		// Expand folders to include all their contents
		var allFilesToDownload []FileItem
		for _, fileItem := range fileItems {
			if fileItem.IsFolder {
				folderFiles, err := getAllFilesInFolder(dbx, fileItem.Path)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to list folder %s: %v", fileItem.Name, err))
					continue
				}
				// Add the folder itself first (for empty folders)
				allFilesToDownload = append(allFilesToDownload, fileItem)
				// Then add all its contents
				allFilesToDownload = append(allFilesToDownload, folderFiles...)
			} else {
				allFilesToDownload = append(allFilesToDownload, fileItem)
			}
		}

		for _, fileItem := range allFilesToDownload {
			localPath := filepath.Join(downloadDir, fileItem.Path)
			if fileItem.IsFolder {
				if err := os.MkdirAll(localPath, 0755); err != nil {
					errors = append(errors, fmt.Sprintf("Failed to create folder %s: %v", fileItem.Name, err))
					continue
				}
				// Don't count empty folders in download count
			} else {
				if _, err := os.Stat(localPath); err == nil {
					skipped = append(skipped, fileItem.Name)
					continue
				}
				parentDir := filepath.Dir(localPath)
				if err := os.MkdirAll(parentDir, 0755); err != nil {
					errors = append(errors, fmt.Sprintf("Failed to create directory for %s: %v", fileItem.Name, err))
					continue
				}
				arg := files.NewDownloadArg(fileItem.Path)
				_, contents, err := dbx.Download(arg)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to download %s: %v", fileItem.Name, err))
					continue
				}
				defer contents.Close()
				contentBytes, err := io.ReadAll(contents)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to read content of %s: %v", fileItem.Name, err))
					continue
				}
				err = os.WriteFile(localPath, contentBytes, 0644)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to write %s: %v", fileItem.Name, err))
					continue
				}
				downloaded = append(downloaded, fileItem.Name)
			}
		}

		return DownloadCompleteMsg{
			Downloaded: downloaded,
			Skipped:    skipped,
			Errors:     errors,
		}
	}
}

// getAllFilesInFolder recursively gets all files in a folder and its subfolders
func getAllFilesInFolder(dbx files.Client, folderPath string) ([]FileItem, error) {
	var allFiles []FileItem

	// List files in the current folder
	arg := files.NewListFolderArg(folderPath)
	result, err := dbx.ListFolder(arg)
	if err != nil {
		return nil, err
	}

	// Process entries
	for _, entry := range result.Entries {
		// Skip deleted files
		if _, ok := entry.(*files.DeletedMetadata); ok {
			continue
		}

		switch v := entry.(type) {
		case *files.FileMetadata:
			allFiles = append(allFiles, FileItem{
				Name:     v.Name,
				Path:     v.PathLower,
				IsFolder: false,
				Size:     int64(v.Size),
				Modified: v.ServerModified,
			})
		case *files.FolderMetadata:
			// Add the folder itself
			allFiles = append(allFiles, FileItem{
				Name:     v.Name,
				Path:     v.PathLower,
				IsFolder: true,
				Size:     0,
				Modified: time.Now(),
			})

			// Recursively get files in this subfolder
			subFiles, err := getAllFilesInFolder(dbx, v.PathLower)
			if err != nil {
				return nil, err
			}
			allFiles = append(allFiles, subFiles...)
		}
	}

	return allFiles, nil
}
