package main

import (
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FileItem represents a file or folder in Dropbox
type FileItem struct {
	Name     string
	Path     string
	IsFolder bool
	Size     int64
	Modified time.Time
}

// Model represents the application state
type Model struct {
	// File browser state
	currentPath string
	files       []FileItem
	cursor      int
	selected    map[int]bool

	// Cache for folder contents
	folderCache map[string][]FileItem

	// UI state
	width  int
	height int

	// Status messages
	status     string
	statusTime time.Time

	// Loading state
	loading bool

	// Error state
	error     string
	errorTime time.Time

	// Download state
	downloading bool

	// Configuration
	config Config
}

// Msg represents messages that can be sent to the model
type Msg interface{}

// StatusMsg represents a status message
type StatusMsg struct {
	Message string
}

// ErrorMsg represents an error message
type ErrorMsg struct {
	Error string
}

// LoadingMsg represents a loading state change
type LoadingMsg struct {
	Loading bool
}

// FilesLoadedMsg represents when files have been loaded
type FilesLoadedMsg struct {
	Files []FileItem
	Path  string
}

// DownloadMsg represents a download operation
type DownloadMsg struct {
	Files []FileItem
}

// DownloadCompleteMsg represents when download is complete
type DownloadCompleteMsg struct {
	Downloaded []string
	Skipped    []string
	Errors     []string
}

// initialModel creates a new model with default values
func initialModel(config *Config) Model {
	return Model{
		currentPath: "",
		files:       []FileItem{},
		cursor:      0,
		selected:    make(map[int]bool),
		folderCache: make(map[string][]FileItem),
		width:       80,
		height:      24,
		status:      "welcome to dbox",
		statusTime:  time.Now(),
		loading:     false,
		downloading: false,
		config:      *config,
	}
}

// Init initializes the model and returns initial commands
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			// Set loading state for initial file load
			return LoadingMsg{Loading: true}
		},
		loadFilesCmd(""),
		tea.EnterAltScreen,
	)
}

// Update handles messages and returns the updated model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.downloading {
			return m, nil
		}
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case StatusMsg:
		m.status = msg.Message
		m.statusTime = time.Now()
		return m, nil
	case ErrorMsg:
		m.downloading = false
		m.error = msg.Error
		m.errorTime = time.Now()
		return m, nil
	case LoadingMsg:
		m.loading = msg.Loading
		return m, nil
	case FilesLoadedMsg:
		m.files = msg.Files
		m.currentPath = msg.Path
		m.cursor = 0
		m.selected = make(map[int]bool)
		m.loading = false
		// Cache the loaded files
		m.folderCache[msg.Path] = msg.Files
		return m, nil
	case DownloadMsg:
		m.downloading = true
		return m, downloadFilesCmd(msg.Files, &m.config)

	case DownloadCompleteMsg:
		// Return to file list
		m.downloading = false
		message := fmt.Sprintf("Download complete. Downloaded: %d, Skipped: %d, Errors: %d",
			len(msg.Downloaded), len(msg.Skipped), len(msg.Errors))
		if len(msg.Errors) > 0 {
			message += fmt.Sprintf(" - Errors: %s", strings.Join(msg.Errors, ", "))
		}
		// Store completion message in status
		m.status = message
		m.statusTime = time.Now()
		return m, nil
	}
	return m, nil
}

// View renders the UI
func (m Model) View() string {
	if m.downloading {
		return "📥 Downloading...\n"
	}
	if m.width == 0 {
		return "Loading..."
	}

	var s strings.Builder

	// Current path
	pathStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	currentPath := m.currentPath
	s.WriteString(pathStyle.Render(currentPath+"/") + "\n\n")

	// File list
	if m.loading {
		s.WriteString("Loading files...\n")
	} else if len(m.files) == 0 {
		s.WriteString("🪹 No files found\n")
	} else {
		fileList := m.renderFileList()
		s.WriteString(fileList)
	}

	// Status/Error messages
	if m.error != "" && time.Since(m.errorTime) < 5*time.Second {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Padding(0, 1)

		// Wrap error message to fit terminal width
		errorText := "❌ " + m.error
		if m.width > 0 {
			// Reserve some space for padding and ensure we don't exceed terminal width
			maxWidth := m.width - 4 // Account for padding and margins
			if maxWidth > 0 {
				errorText = lipgloss.NewStyle().Width(maxWidth).Render(errorText)
			}
		}
		s.WriteString("\n" + errorStyle.Render(errorText))
	} else if m.status != "" && time.Since(m.statusTime) < 3*time.Second {
		statusStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("156")).
			Padding(0, 1)

		// Wrap status message to fit terminal width
		statusText := "ℹ️  " + m.status
		if m.width > 0 {
			// Reserve some space for padding and ensure we don't exceed terminal width
			maxWidth := m.width - 4 // Account for padding and margins
			if maxWidth > 0 {
				statusText = lipgloss.NewStyle().Width(maxWidth).Render(statusText)
			}
		}
		s.WriteString("\n" + statusStyle.Render(statusText))
	}

	return s.String()
}

// handleKeyPress processes keyboard input
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.downloading {
		return m, nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.files)-1 {
			m.cursor++
		}
	case "g":
		// Jump to top
		m.cursor = 0
	case "G":
		// Jump to bottom
		if len(m.files) > 0 {
			m.cursor = len(m.files) - 1
		}
	case "ctrl+u":
		// Go up 5 items
		m.cursor = max(0, m.cursor-5)
	case "ctrl+d":
		// Go down 5 items
		if len(m.files) > 0 {
			m.cursor = min(len(m.files)-1, m.cursor+5)
		}
	case "enter":
		if len(m.files) > 0 && m.cursor < len(m.files) {
			file := m.files[m.cursor]
			if file.IsFolder {
				// Check if folder is cached
				if cachedFiles, exists := m.folderCache[file.Path]; exists {
					m.files = cachedFiles
					m.currentPath = file.Path
					m.cursor = 0
					m.selected = make(map[int]bool)
					return m, nil
				} else {
					m.loading = true
					return m, loadFilesCmd(file.Path)
				}
			} else {
				// TODO: Handle file opening
				return m, func() tea.Msg {
					return StatusMsg{Message: "Opening file: " + file.Name}
				}
			}
		}
	case " ":
		if len(m.files) > 0 && m.cursor < len(m.files) {
			if m.selected[m.cursor] {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = true
			}
		}
	case "esc":
		if m.currentPath != "" {
			parent := filepath.Dir(m.currentPath)
			if parent == "." || parent == "/" {
				parent = ""
			}
			// Check if parent is cached
			if cachedFiles, exists := m.folderCache[parent]; exists {
				m.files = cachedFiles
				m.currentPath = parent
				m.cursor = 0
				m.selected = make(map[int]bool)
				return m, nil
			} else {
				m.loading = true
				return m, loadFilesCmd(parent)
			}
		}
	case "R":
		m.loading = true
		return m, loadFilesCmd(m.currentPath)
	case "C":
		// Clear the cache
		m.folderCache = make(map[string][]FileItem)
		return m, func() tea.Msg {
			return StatusMsg{Message: "Cache cleared"}
		}
	case "b":
		// Open current folder in Dropbox web UI
		webPath := m.currentPath
		if webPath == "" {
			webPath = "/"
		}
		// Properly URL encode the path for the web URL
		encodedPath := url.PathEscape(webPath)
		dropboxURL := fmt.Sprintf("https://www.dropbox.com/home%s", encodedPath)

		// Open the URL in the default browser
		return m, func() tea.Msg {
			// Use the system's default browser to open the URL
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				cmd = exec.Command("open", dropboxURL)
			case "linux":
				cmd = exec.Command("xdg-open", dropboxURL)
			case "windows":
				cmd = exec.Command("cmd", "/c", "start", dropboxURL)
			default:
				return StatusMsg{Message: "Cannot open browser on this platform"}
			}

			if err := cmd.Start(); err != nil {
				return StatusMsg{Message: fmt.Sprintf("Failed to open browser: %v", err)}
			}

			return StatusMsg{Message: fmt.Sprintf("Opened %s in browser", webPath)}
		}
	case "d":
		// Download selected files
		if len(m.selected) > 0 {
			var selectedFiles []FileItem
			for i, selected := range m.selected {
				if selected && i < len(m.files) {
					selectedFiles = append(selectedFiles, m.files[i])
				}
			}
			if len(selectedFiles) > 0 {
				return m, func() tea.Msg {
					return DownloadMsg{Files: selectedFiles}
				}
			}
		} else {
			return m, func() tea.Msg {
				return StatusMsg{Message: "No files selected for download"}
			}
		}
	}
	return m, nil
}

// handleWindowSize processes window size changes
func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	return m, nil
}

// renderFileList renders the list of files
func (m Model) renderFileList() string {
	var s strings.Builder

	for i, file := range m.files {
		// Cursor indicator
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		// Selection indicator
		selected := " "
		if m.selected[i] {
			selected = "✓"
		}

		// File icon and name
		icon := "📄"
		if file.IsFolder {
			icon = "📁"
		}

		// Style based on selection and cursor
		style := lipgloss.NewStyle()
		if m.cursor == i {
			style = style.Bold(true).Foreground(lipgloss.Color("63"))
		}
		if m.selected[i] {
			style = style.Foreground(lipgloss.Color("156"))
		}

		line := fmt.Sprintf("%s %s %s %s", cursor, selected, icon, file.Name)
		s.WriteString(style.Render(line) + "\n")
	}

	return s.String()
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
