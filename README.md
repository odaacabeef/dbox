# dbox

A terminal application for working with Dropbox.

`dbox` has two modes:

- **Browse mode** (default) — browse your Dropbox and selectively download
  files and folders.
- **Management mode** (`dbox <config>.yaml`) — push local files from the
  current directory up to a Dropbox folder, skipping anything that hasn't
  changed.

## Installation

Install the latest version directly with Go:

```sh
go install github.com/odaacabeef/dbox@latest
```

Or build from a checkout:

```sh
git clone https://github.com/odaacabeef/dbox
cd dbox
go build -o dbox .
```

## Authentication

`dbox` authenticates with Dropbox via OAuth and reads its credentials from three
environment variables — `DROPBOX_APP_KEY`, `DROPBOX_APP_SECRET`, and
`DROPBOX_REFRESH_TOKEN`. The refresh token is long-lived, so once it's set the
app renews access automatically with nothing to regenerate. `dbox` writes
nothing to disk, so you can keep the credentials in an encrypted store.

### One-time setup

1. Create an app at https://www.dropbox.com/developers/apps.
2. Under **Settings**, note the **App key** and **App secret**, and add a
   **Redirect URI** of `http://localhost:53682/`.
3. Under **Permissions**, enable the scopes you need, then save:
   - Browse / download: `files.metadata.read`, `files.content.read`
   - Push: also `files.content.write`
   - Collaborators: also `sharing.read`, `sharing.write`
4. Run `dbox login` to obtain a refresh token. It opens your browser to
   authorize the app, then prints sourceable exports to stdout (status messages
   go to stderr, so stdout stays clean to pipe or capture):

   ```sh
   export DROPBOX_APP_KEY="..."     # from the app's Settings
   export DROPBOX_APP_SECRET="..."  # from the app's Settings
   dbox login
   # ->
   # export DROPBOX_APP_KEY='...'
   # export DROPBOX_APP_SECRET='...'
   # export DROPBOX_REFRESH_TOKEN='...'
   ```

### Running

Store those exports wherever you keep secrets and source them before running
`dbox`. For example, with [`pass`](https://www.passwordstore.org/):

```sh
dbox login | pass insert -m -e Dev/dropbox/export/odaacabeef-dbox   # save once
. <(pass Dev/dropbox/export/odaacabeef-dbox)                        # each session
dbox                                                                # or: dbox dbox.yaml
```

## Browse mode

Run `dbox` with no arguments to browse your account:

```sh
dbox
```

Move through folders, select items with `space`, and press `d` to download
them. Selecting a folder downloads it recursively. Downloads are written under
`~/.dbox/`, mirroring their Dropbox path; files that already exist locally are
skipped.

| Key | Action |
| --- | --- |
| `up` / `k` | Move up |
| `down` / `j` | Move down |
| `g` | Jump to top |
| `G` | Jump to bottom |
| `ctrl+u` | Move up 5 items |
| `ctrl+d` | Move down 5 items |
| `enter` | Open folder |
| `esc` | Go to parent folder |
| `space` | Toggle selection |
| `d` | Download selected files |
| `b` | Open current folder in browser |
| `R` | Refresh current folder |
| `C` | Clear folder cache |
| `?` | Toggle help |
| `q` / `ctrl+c` | Quit |

## Management mode

Passing a config file opens management mode, which pushes matching files from
the current directory up to a Dropbox folder:

```sh
cd ~/path/to/local/folder
dbox dbox.yaml
```

The config file describes where files go and which ones to include:

```yaml
# dbox.yaml
remote: /sequences/cool-song         # remote Dropbox folder (created if needed)
file_types: [wav]                    # only files with these extensions are pushed
collaborators:                       # optional; see "Collaborators" below
  - alice@example.com
  - bob@example.com
```

On launch, `dbox` lists the matching files in the current directory and its
subdirectories (hidden files and hidden directories are ignored). The directory
layout is preserved, so a local `drums/kick.wav` is uploaded to
`<remote>/drums/kick.wav`.

Each file is checked against the remote and shows its sync state: `✓ in sync`
(already uploaded, identical), `● modified` (on the remote but changed locally),
or `new` (not on the remote yet). Files of the configured types that exist on
the remote but not locally appear in the list in gray as `remote only`; they're
shown for awareness and are never part of a push. The list is sorted
alphabetically. Press `P` to push the local files. For each file:

- If the same content already exists at the remote path it is **skipped** —
  comparison uses Dropbox's content hash, so re-running only uploads what
  actually changed.
- Otherwise the file is uploaded, overwriting any existing remote copy. Large
  files are uploaded in chunks automatically.

The remote folder is created if it doesn't already exist.

| Key | Action |
| --- | --- |
| `up` / `k` | Move up |
| `down` / `j` | Move down |
| `g` | Jump to top |
| `G` | Jump to bottom |
| `P` | Push files to Dropbox |
| `C` | Reconcile collaborators (make remote match config) |
| `R` | Rescan the local folder |
| `?` | Toggle help |
| `q` / `ctrl+c` | Quit |

### Collaborators

If the config includes a `collaborators` list, management mode also manages who
the folder is shared with, treating the config as the **source of truth**. On
launch it shows the difference between the configured collaborators and the
folder's actual Dropbox members:

- `+ to add` — in the config but not yet on the folder
- `✓ in sync` — in both
- `− to remove` — on the folder but not in the config
- `👑 owner` — you; never changed

Pressing `C` reconciles the folder to match the config: it shares the folder if
needed, invites anyone missing as an **editor** (Dropbox emails them an invite),
and **removes anyone who isn't listed**. Because removals affect other people's
access, the diff is always shown before you press `C`, and reconciling happens
immediately when you do.

Notes:

- The folder owner is never removed, even if not in the list.
- An empty or omitted `collaborators` list disables this entirely — it is never
  interpreted as "remove everyone."
- Collaborators who have access via a Dropbox group are not managed here.
