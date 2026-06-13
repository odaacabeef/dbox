package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/async"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/sharing"
)

const (
	// jobPollInterval and jobPollMax bound how long we wait for an async
	// sharing job (member removal, folder share) to finish.
	jobPollInterval = 1 * time.Second
	jobPollMax      = 30
)

// loadCollaboratorsCmd reads the folder's current Dropbox membership and diffs
// it against the configured collaborators. It is strictly read-only: it never
// creates or shares the folder.
func loadCollaboratorsCmd(cfg *DboxConfig) tea.Cmd {
	return func() tea.Msg {
		token := os.Getenv("DROPBOX_ACCESS_TOKEN")
		if token == "" {
			return ErrorMsg{Error: "DROPBOX_ACCESS_TOKEN environment variable not set"}
		}
		fc := files.New(dropbox.Config{Token: token})
		sc := sharing.New(dropbox.Config{Token: token})

		id, shared, err := resolveSharedFolderID(fc, sc, cfg.Remote, false)
		if err != nil {
			return ErrorMsg{Error: fmt.Sprintf("Failed to inspect %s: %v", cfg.Remote, err)}
		}

		var owner string
		current := map[string]bool{}
		if shared {
			accepted, invitees, err := listAllMembers(sc, id)
			if err != nil {
				return ErrorMsg{Error: fmt.Sprintf("Failed to list collaborators: %v", err)}
			}
			owner, current = currentMembers(accepted, invitees)
		}

		return CollaboratorsLoadedMsg{
			Items:  buildCollaboratorItems(cfg.Collaborators, current, owner),
			Shared: shared,
		}
	}
}

// reconcileCollaboratorsCmd makes the remote folder's membership match the
// configured collaborators: it shares the folder if needed, adds anyone
// missing (as editor), and removes anyone present who isn't in the config. The
// owner is never removed.
func reconcileCollaboratorsCmd(cfg *DboxConfig) tea.Cmd {
	return func() tea.Msg {
		token := os.Getenv("DROPBOX_ACCESS_TOKEN")
		if token == "" {
			return ErrorMsg{Error: "DROPBOX_ACCESS_TOKEN environment variable not set"}
		}
		fc := files.New(dropbox.Config{Token: token})
		sc := sharing.New(dropbox.Config{Token: token})

		if err := ensureRemoteFolder(fc, cfg.Remote); err != nil {
			return ErrorMsg{Error: fmt.Sprintf("Failed to create remote folder %s: %v", cfg.Remote, err)}
		}

		id, _, err := resolveSharedFolderID(fc, sc, cfg.Remote, true)
		if err != nil {
			return ErrorMsg{Error: fmt.Sprintf("Failed to share %s: %v", cfg.Remote, err)}
		}

		accepted, invitees, err := listAllMembers(sc, id)
		if err != nil {
			return ErrorMsg{Error: fmt.Sprintf("Failed to list collaborators: %v", err)}
		}
		owner, current := currentMembers(accepted, invitees)
		toAdd, toRemove := diffCollaborators(cfg.Collaborators, current, owner)

		var added, removed, errs []string

		if len(toAdd) > 0 {
			members := make([]*sharing.AddMember, 0, len(toAdd))
			for _, email := range toAdd {
				m := sharing.NewAddMember(emailSelector(email))
				m.AccessLevel = editorAccess()
				members = append(members, m)
			}
			arg := sharing.NewAddFolderMemberArg(id, members)
			arg.Quiet = false // send the invite email
			if err := sc.AddFolderMember(arg); err != nil {
				errs = append(errs, fmt.Sprintf("add %s: %v", strings.Join(toAdd, ", "), err))
			} else {
				added = toAdd
			}
		}

		for _, email := range toRemove {
			arg := sharing.NewRemoveFolderMemberArg(id, emailSelector(email), false)
			res, err := sc.RemoveFolderMember(arg)
			if err != nil {
				errs = append(errs, fmt.Sprintf("remove %s: %v", email, err))
				continue
			}
			if err := pollJob(sc, res.AsyncJobId); err != nil {
				errs = append(errs, fmt.Sprintf("remove %s: %v", email, err))
				continue
			}
			removed = append(removed, email)
		}

		return ReconcileCompleteMsg{Added: added, Removed: removed, Errors: errs}
	}
}

// resolveSharedFolderID returns the shared-folder ID for the remote path. If
// the folder isn't shared yet and allowShare is true, it shares it (handling
// the async case); when allowShare is false it reports shared=false without
// mutating anything.
func resolveSharedFolderID(fc files.Client, sc sharing.Client, remote string, allowShare bool) (id string, shared bool, err error) {
	meta, err := fc.GetMetadata(files.NewGetMetadataArg(remote))
	if err != nil {
		if isNotFoundErr(err) {
			return "", false, nil // folder doesn't exist yet
		}
		return "", false, err
	}
	if folder, ok := meta.(*files.FolderMetadata); ok {
		if folder.SharingInfo != nil && folder.SharingInfo.SharedFolderId != "" {
			return folder.SharingInfo.SharedFolderId, true, nil
		}
	}
	if !allowShare {
		return "", false, nil
	}

	launch, err := sc.ShareFolder(sharing.NewShareFolderArg(remote))
	if err != nil {
		return "", false, err
	}
	if launch.Complete != nil {
		return launch.Complete.SharedFolderId, true, nil
	}
	// Asynchronous share job: poll until it completes.
	for i := 0; i < jobPollMax; i++ {
		time.Sleep(jobPollInterval)
		status, err := sc.CheckShareJobStatus(async.NewPollArg(launch.AsyncJobId))
		if err != nil {
			return "", false, err
		}
		switch status.Tag {
		case "complete":
			return status.Complete.SharedFolderId, true, nil
		case "failed":
			return "", false, fmt.Errorf("share job failed: %v", status.Failed)
		}
	}
	return "", false, fmt.Errorf("share job did not finish in time")
}

// listAllMembers returns every accepted user and pending invitee of a shared
// folder, following the pagination cursor.
func listAllMembers(sc sharing.Client, id string) (accepted []*sharing.UserMembershipInfo, invitees []*sharing.InviteeMembershipInfo, err error) {
	res, err := sc.ListFolderMembers(sharing.NewListFolderMembersArgs(id))
	if err != nil {
		return nil, nil, err
	}
	accepted = append(accepted, res.Users...)
	invitees = append(invitees, res.Invitees...)
	for res.Cursor != "" {
		res, err = sc.ListFolderMembersContinue(sharing.NewListFolderMembersContinueArg(res.Cursor))
		if err != nil {
			return nil, nil, err
		}
		accepted = append(accepted, res.Users...)
		invitees = append(invitees, res.Invitees...)
	}
	return accepted, invitees, nil
}

// currentMembers reduces the raw membership lists to the owner's email and the
// set of current collaborator emails (accepted non-owners plus pending
// invitees), all lowercased. The owner is excluded from the set so it can never
// be flagged for removal.
func currentMembers(accepted []*sharing.UserMembershipInfo, invitees []*sharing.InviteeMembershipInfo) (owner string, current map[string]bool) {
	current = map[string]bool{}
	for _, u := range accepted {
		if u.User == nil {
			continue
		}
		email := strings.ToLower(u.User.Email)
		if u.AccessType != nil && u.AccessType.Tag == sharing.AccessLevelOwner {
			owner = email
			continue
		}
		current[email] = true
	}
	for _, inv := range invitees {
		if inv.Invitee != nil && inv.Invitee.Email != "" {
			current[strings.ToLower(inv.Invitee.Email)] = true
		}
	}
	return owner, current
}

// diffCollaborators compares the desired collaborator emails (from config)
// against the current members and returns who to add and who to remove. Inputs
// are assumed lowercased; the owner is never returned for removal or addition.
func diffCollaborators(desired []string, current map[string]bool, owner string) (toAdd, toRemove []string) {
	desiredSet := make(map[string]bool, len(desired))
	for _, email := range desired {
		if email == owner {
			continue // can't add the owner; nothing to do
		}
		desiredSet[email] = true
		if !current[email] {
			toAdd = append(toAdd, email)
		}
	}
	for email := range current {
		if email == owner {
			continue // never remove the owner
		}
		if !desiredSet[email] {
			toRemove = append(toRemove, email)
		}
	}
	sort.Strings(toAdd)
	sort.Strings(toRemove)
	return toAdd, toRemove
}

// buildCollaboratorItems produces the display rows for the TUI: the owner
// first, then every desired/current email tagged with its sync status.
func buildCollaboratorItems(desired []string, current map[string]bool, owner string) []CollaboratorItem {
	var items []CollaboratorItem
	if owner != "" {
		items = append(items, CollaboratorItem{Email: owner, Status: CollabOwner})
	}

	toAdd, toRemove := diffCollaborators(desired, current, owner)
	add := toSet(toAdd)

	var rows []CollaboratorItem
	// Every configured email is either pending an add or already in sync.
	for _, email := range desired {
		if email == owner {
			continue
		}
		if add[email] {
			rows = append(rows, CollaboratorItem{Email: email, Status: CollabToAdd})
		} else {
			rows = append(rows, CollaboratorItem{Email: email, Status: CollabInSync})
		}
	}
	// Plus anyone on the folder who isn't configured.
	for _, email := range toRemove {
		rows = append(rows, CollaboratorItem{Email: email, Status: CollabToRemove})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Email < rows[j].Email })
	return append(items, rows...)
}

// pollJob waits for an async sharing job (e.g. member removal) to finish.
func pollJob(sc sharing.Client, jobID string) error {
	if jobID == "" {
		return nil // completed synchronously
	}
	for i := 0; i < jobPollMax; i++ {
		status, err := sc.CheckJobStatus(async.NewPollArg(jobID))
		if err != nil {
			return err
		}
		switch status.Tag {
		case "complete":
			return nil
		case "failed":
			return fmt.Errorf("%v", status.Failed)
		}
		time.Sleep(jobPollInterval)
	}
	return fmt.Errorf("job did not finish in time")
}

// emailSelector builds a member selector for an email address.
func emailSelector(email string) *sharing.MemberSelector {
	return &sharing.MemberSelector{
		Tagged: dropbox.Tagged{Tag: "email"},
		Email:  email,
	}
}

// editorAccess returns the editor access level.
func editorAccess() *sharing.AccessLevel {
	return &sharing.AccessLevel{Tagged: dropbox.Tagged{Tag: sharing.AccessLevelEditor}}
}
