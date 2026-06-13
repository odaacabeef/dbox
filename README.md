# dbox

Terminal application for working with Dropbox.

## Usage

The primary function is browsing files and selectively downloading them.

Download path is hardcoded to `~/.dbox/`.

Use ' ' (space) to select files and/or folders and press 'd' to download them.

Press '?' at any time to see the available key bindings.

## Management mode

Passing a config file (`dbox dbox.yaml`) opens management mode instead, which
pushes matching files from the current directory up to a Dropbox folder. Files
whose contents already match what's on Dropbox are skipped, so re-running only
uploads what changed.

```yaml
# dbox.yaml
remote: /sequences/airy-dissonance   # remote Dropbox folder (created if needed)
file_types: [wav]                    # only these extensions are uploaded
# collaborators: []                  # reserved; not acted on yet
```

`cd` into the folder you want to push, then run `dbox /path/to/dbox.yaml`.
Only top-level files are considered (no recursion into subdirectories). Press
'u' to push, 'R' to rescan, '?' for help.

## Authentication

Expects `DROPBOX_ACCESS_TOKEN` environment variable. You obtain one by first
creating an application: https://www.dropbox.com/developers/apps. Go to settings
--> generated access token --> generate.

Browsing and downloading need read scopes. Management mode also uploads, so the
token must include the `files.content.write` scope (and `files.metadata.read`);
a read-only token will fail when pushing.
