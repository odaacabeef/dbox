package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// UploadStatus is the per-file state shown in management mode.
type UploadStatus int

const (
	StatusChecking UploadStatus = iota // sync state not yet determined
	StatusNew                          // not present on the remote
	StatusSynced                       // present on the remote, identical
	StatusModified                     // present on the remote, content differs
	StatusUploaded                     // uploaded during this session
	StatusSkipped                      // skipped during a push (unchanged)
	StatusError
)

// ManageFileItem is a local file considered for upload.
type ManageFileItem struct {
	Rel    string // path relative to cwd, slash-separated (also the display name)
	Path   string // absolute local path
	Size   int64
	Status UploadStatus
	Err    string
}

// UploadCompleteMsg reports the result of a push. Each slice holds file names
// (errors are "name: message").
type UploadCompleteMsg struct {
	Uploaded []string
	Skipped  []string
	Errors   []string
}

// SyncStatusMsg carries each file's launch-time sync state, keyed by relative
// path. Errors holds the message for any file whose check failed.
type SyncStatusMsg struct {
	Statuses map[string]UploadStatus
	Errors   map[string]string
}

// CollabStatus is the sync state of a collaborator relative to the config.
type CollabStatus int

const (
	CollabOwner    CollabStatus = iota // folder owner (never changed)
	CollabInSync                       // in config and on the folder
	CollabToAdd                        // in config, not yet on the folder
	CollabToRemove                     // on the folder, not in config
)

// CollaboratorItem is a single row in the collaborators diff.
type CollaboratorItem struct {
	Email  string
	Status CollabStatus
}

// CollaboratorsLoadedMsg carries the current collaborator diff. Shared reports
// whether the remote folder is a shared folder yet.
type CollaboratorsLoadedMsg struct {
	Items  []CollaboratorItem
	Shared bool
}

// ReconcileCompleteMsg reports the result of reconciling collaborators.
type ReconcileCompleteMsg struct {
	Added   []string
	Removed []string
	Errors  []string
}

// ManageModel is the management-mode TUI: it lists the local files matching the
// config and pushes them to Dropbox on demand.
type ManageModel struct {
	config Config
	dbox   *DboxConfig
	cwd    string

	files  []ManageFileItem
	cursor int

	width  int
	height int

	pushing  bool
	showHelp bool

	// Collaborator management (only active when the config lists collaborators).
	collaborators []CollaboratorItem
	collabLoading bool
	collabShared  bool
	reconciling   bool

	status     string
	statusTime time.Time
	error      string
	errorTime  time.Time
}

// managesCollaborators reports whether the config opts into collaborator
// management. An empty list disables it (so it can't mean "remove everyone").
func (m ManageModel) managesCollaborators() bool {
	return len(m.dbox.Collaborators) > 0
}

// initialManageModel scans the working directory and builds the model.
func initialManageModel(config *Config, dbox *DboxConfig, cwd string) ManageModel {
	m := ManageModel{
		config:     *config,
		dbox:       dbox,
		cwd:        cwd,
		width:      80,
		height:     24,
		statusTime: time.Now(),
	}

	files, err := scanLocalFiles(cwd, dbox)
	if err != nil {
		m.error = fmt.Sprintf("Failed to scan %s: %v", cwd, err)
		m.errorTime = time.Now()
	} else {
		m.files = files
	}

	m.collabLoading = m.managesCollaborators()

	switch {
	case m.error != "":
		// keep the scan error
	case len(m.files) == 0:
		m.status = fmt.Sprintf("no files matching %s in %s", strings.Join(dbox.FileTypes, ", "), cwd)
	case m.managesCollaborators():
		m.status = "press P to push files · C to reconcile collaborators"
	default:
		m.status = "press P to push files"
	}

	return m
}

// Init initializes the model. It enters the alt screen, checks which files are
// already in sync with the remote, and (when configured) loads the
// collaborator diff.
func (m ManageModel) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.EnterAltScreen}
	if len(m.files) > 0 {
		cmds = append(cmds, checkSyncStatusCmd(m.dbox, m.files))
	}
	if m.managesCollaborators() {
		cmds = append(cmds, loadCollaboratorsCmd(m.dbox))
	}
	return tea.Batch(cmds...)
}

// Update handles messages and returns the updated model.
func (m ManageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case StatusMsg:
		m.status = msg.Message
		m.statusTime = time.Now()
		return m, nil
	case ErrorMsg:
		m.pushing = false
		m.reconciling = false
		m.collabLoading = false
		m.error = msg.Error
		m.errorTime = time.Now()
		return m, nil
	case UploadCompleteMsg:
		m.pushing = false
		m.applyResults(msg)
		m.status = fmt.Sprintf("Push complete. Uploaded: %d, Skipped: %d, Errors: %d",
			len(msg.Uploaded), len(msg.Skipped), len(msg.Errors))
		m.statusTime = time.Now()
		return m, nil
	case SyncStatusMsg:
		for i := range m.files {
			// Only resolve files still awaiting their status, so a late result
			// never clobbers post-push state.
			if m.files[i].Status != StatusChecking {
				continue
			}
			if st, ok := msg.Statuses[m.files[i].Rel]; ok {
				m.files[i].Status = st
				m.files[i].Err = msg.Errors[m.files[i].Rel]
			}
		}
		return m, nil
	case CollaboratorsLoadedMsg:
		m.collaborators = msg.Items
		m.collabShared = msg.Shared
		m.collabLoading = false
		return m, nil
	case ReconcileCompleteMsg:
		m.reconciling = false
		m.status = fmt.Sprintf("Collaborators reconciled. Added: %d, Removed: %d, Errors: %d",
			len(msg.Added), len(msg.Removed), len(msg.Errors))
		m.statusTime = time.Now()
		if len(msg.Errors) > 0 {
			m.error = strings.Join(msg.Errors, "; ")
			m.errorTime = time.Now()
		}
		// Refresh the diff to reflect the new state.
		m.collabLoading = true
		return m, loadCollaboratorsCmd(m.dbox)
	}
	return m, nil
}

// applyResults maps the completion result back onto each file's status.
func (m *ManageModel) applyResults(msg UploadCompleteMsg) {
	uploaded := toSet(msg.Uploaded)
	skipped := toSet(msg.Skipped)
	// Error entries are "<rel>: message"; index the message by relative path.
	errByRel := make(map[string]string)
	for _, e := range msg.Errors {
		rel := e
		if i := strings.Index(e, ":"); i >= 0 {
			rel = e[:i]
		}
		errByRel[rel] = e
	}

	for i := range m.files {
		rel := m.files[i].Rel
		switch {
		case uploaded[rel]:
			m.files[i].Status = StatusUploaded
			m.files[i].Err = ""
		case skipped[rel]:
			m.files[i].Status = StatusSkipped
			m.files[i].Err = ""
		case errByRel[rel] != "":
			m.files[i].Status = StatusError
			m.files[i].Err = errByRel[rel]
		}
	}
}

func toSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, it := range items {
		set[it] = true
	}
	return set
}

// handleKeyPress processes keyboard input.
func (m ManageModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While a push or reconcile is in flight, ignore everything so the
	// operation isn't disturbed.
	if m.pushing || m.reconciling {
		return m, nil
	}
	// While help is open, only allow closing it or quitting.
	if m.showHelp {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?", "esc":
			m.showHelp = false
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.files)-1 {
			m.cursor++
		}
	case "g":
		m.cursor = 0
	case "G":
		if len(m.files) > 0 {
			m.cursor = len(m.files) - 1
		}
	case "R":
		files, err := scanLocalFiles(m.cwd, m.dbox)
		if err != nil {
			m.error = fmt.Sprintf("Failed to scan %s: %v", m.cwd, err)
			m.errorTime = time.Now()
			return m, nil
		}
		m.files = files
		m.cursor = 0
		m.status = fmt.Sprintf("rescanned: %d file(s)", len(files))
		m.statusTime = time.Now()
		if len(files) > 0 {
			return m, checkSyncStatusCmd(m.dbox, files)
		}
	case "P":
		if len(m.files) == 0 {
			return m, func() tea.Msg { return StatusMsg{Message: "nothing to push"} }
		}
		m.pushing = true
		return m, pushFilesCmd(m.dbox, m.files)
	case "C":
		if !m.managesCollaborators() {
			return m, func() tea.Msg { return StatusMsg{Message: "no collaborators configured"} }
		}
		if m.collabLoading {
			return m, nil // wait for the current diff to finish loading
		}
		m.reconciling = true
		return m, reconcileCollaboratorsCmd(m.dbox)
	}
	return m, nil
}

// View renders the UI.
func (m ManageModel) View() string {
	if m.pushing {
		return "📤 Pushing...\n"
	}
	if m.reconciling {
		return "🔧 Reconciling collaborators...\n"
	}
	if m.width == 0 {
		return "Loading..."
	}
	if m.showHelp {
		return m.renderHelpView()
	}

	var s strings.Builder

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))

	s.WriteString(titleStyle.Render("dbox — management mode") + "\n")
	s.WriteString(headerStyle.Render("remote:     "+m.dbox.Remote) + "\n")
	s.WriteString(headerStyle.Render("file types: "+strings.Join(m.dbox.FileTypes, ", ")) + "\n")
	s.WriteString(headerStyle.Render("source:     "+m.cwd) + "\n\n")

	if len(m.files) == 0 {
		s.WriteString("🪹 No matching files\n")
	} else {
		s.WriteString(m.renderFileList())
	}

	if m.managesCollaborators() {
		s.WriteString("\n" + m.renderCollaborators())
	}

	// Status/Error line, matching the browse model's behavior.
	if m.error != "" && time.Since(m.errorTime) < 5*time.Second {
		s.WriteString("\n" + m.renderMessage("❌ "+m.error, "203"))
	} else if m.status != "" && time.Since(m.statusTime) < 30*time.Second {
		s.WriteString("\n" + m.renderMessage("ℹ️  "+m.status, "156"))
	}

	return s.String()
}

// renderMessage renders a status/error line wrapped to the terminal width,
// mirroring the browse model's status rendering.
func (m ManageModel) renderMessage(text, color string) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Padding(0, 1)
	if m.width > 0 {
		if maxWidth := m.width - 4; maxWidth > 0 {
			text = lipgloss.NewStyle().Width(maxWidth).Render(text)
		}
	}
	return style.Render(text)
}

// renderFileList renders each file with its size and upload status.
func (m ManageModel) renderFileList() string {
	var s strings.Builder

	for i, file := range m.files {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		status, color := statusLabel(file)

		style := lipgloss.NewStyle()
		if color != "" {
			style = style.Foreground(lipgloss.Color(color))
		}
		if m.cursor == i {
			style = style.Bold(true)
			if color == "" {
				style = style.Foreground(lipgloss.Color("63"))
			}
		}

		line := fmt.Sprintf("%s 📄 %-40s %10s   %s", cursor, file.Rel, humanizeSize(file.Size), status)
		s.WriteString(style.Render(line) + "\n")
	}

	return s.String()
}

// renderCollaborators renders the collaborator diff section.
func (m ManageModel) renderCollaborators() string {
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var s strings.Builder
	s.WriteString(headerStyle.Render("collaborators (editor):") + "\n")

	if m.collabLoading {
		s.WriteString(headerStyle.Render("  loading…") + "\n")
		return s.String()
	}
	if len(m.collaborators) == 0 {
		s.WriteString(headerStyle.Render("  (none)") + "\n")
		return s.String()
	}

	for _, c := range m.collaborators {
		marker, label, color := collabLabel(c.Status)
		style := lipgloss.NewStyle()
		if color != "" {
			style = style.Foreground(lipgloss.Color(color))
		}
		line := fmt.Sprintf("  %s %-36s %s", marker, c.Email, label)
		s.WriteString(style.Render(line) + "\n")
	}
	return s.String()
}

// collabLabel returns the marker, label, and color for a collaborator status.
func collabLabel(status CollabStatus) (marker, label, color string) {
	switch status {
	case CollabOwner:
		return "👑", "owner", "240"
	case CollabToAdd:
		return "+", "to add", "156"
	case CollabToRemove:
		return "−", "to remove (not in config)", "203"
	default: // CollabInSync
		return "✓", "in sync", "156"
	}
}

// statusLabel returns the display label and lipgloss color for a file's status.
func statusLabel(file ManageFileItem) (string, string) {
	switch file.Status {
	case StatusChecking:
		return "checking…", "240"
	case StatusNew:
		return "new", ""
	case StatusSynced:
		return "✓ in sync", "156"
	case StatusModified:
		return "● modified", "214"
	case StatusUploaded:
		return "✓ uploaded", "156"
	case StatusSkipped:
		return "↷ skipped (unchanged)", "156"
	case StatusError:
		msg := file.Err
		if msg == "" {
			return "✗ error", "203"
		}
		if i := strings.Index(msg, ":"); i >= 0 {
			msg = strings.TrimSpace(msg[i+1:])
		}
		return "✗ error: " + msg, "203"
	default:
		return "", ""
	}
}

// humanizeSize formats a byte count as a short human-readable string.
func humanizeSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// renderHelpView renders the management-mode help screen.
func (m ManageModel) renderHelpView() string {
	var s strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("156"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	type binding struct {
		keys string
		desc string
	}
	sections := []struct {
		title    string
		bindings []binding
	}{
		{
			title: "Navigation",
			bindings: []binding{
				{"up / k", "move up"},
				{"down / j", "move down"},
				{"g", "jump to top"},
				{"G", "jump to bottom"},
			},
		},
		{
			title: "Actions",
			bindings: []binding{
				{"P", "push files to Dropbox"},
				{"C", "reconcile collaborators (make remote match config)"},
				{"R", "rescan the local folder"},
			},
		},
		{
			title: "General",
			bindings: []binding{
				{"?", "toggle this help"},
				{"q / ctrl+c", "quit"},
			},
		},
	}

	keyWidth := 0
	for _, section := range sections {
		for _, b := range section.bindings {
			if len(b.keys) > keyWidth {
				keyWidth = len(b.keys)
			}
		}
	}

	s.WriteString(titleStyle.Render("dbox — management mode help") + "\n\n")
	for _, section := range sections {
		s.WriteString(titleStyle.Render(section.title) + "\n")
		for _, b := range section.bindings {
			key := keyStyle.Render(fmt.Sprintf("%-*s", keyWidth, b.keys))
			s.WriteString("  " + key + "  " + descStyle.Render(b.desc) + "\n")
		}
		s.WriteString("\n")
	}
	s.WriteString(descStyle.Render("press ? or esc to close") + "\n")

	return s.String()
}
