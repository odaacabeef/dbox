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

`dbox` reads your Dropbox access token from the `DROPBOX_ACCESS_TOKEN`
environment variable:

```sh
export DROPBOX_ACCESS_TOKEN="..."
```

To obtain a token, create an app at
https://www.dropbox.com/developers/apps, then under the app's **Settings** use
**Generated access token → Generate**.

Make sure the token has the scopes the mode you're using needs (set these under
the app's **Permissions** tab, then regenerate the token):

| Mode | Required scopes |
| --- | --- |
| Browse / download | `files.metadata.read`, `files.content.read` |
| Management (push) | the above plus `files.content.write` |

A read-only token works for browsing but will fail when pushing.

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
remote: /sequences/airy-dissonance   # remote Dropbox folder (created if needed)
file_types: [wav]                    # only files with these extensions are pushed
```

On launch, `dbox` lists the matching files in the current directory and its
subdirectories (hidden files and hidden directories are ignored). The directory
layout is preserved, so a local `drums/kick.wav` is uploaded to
`<remote>/drums/kick.wav`. Press `u` to push them. For each file:

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
| `u` / `enter` | Push files to Dropbox |
| `R` | Rescan the local folder |
| `?` | Toggle help |
| `q` / `ctrl+c` | Quit |
