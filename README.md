# dbox

Terminal application for working with Dropbox.

Functionality is geared toward musical collaboration.

It's intended to be used in conjunction with the Dropbox web UI as opposed to
being a replacement for it.

## Usage

The primary function is browsing files and selectively synchronizing local files
with Dropbox.

The local synchronization path is `~/.dbox/`.

When browsing, you only see remote files.

Pressing 'u' inside a folder will upload any local files in that directory.

Pressing 'd' will download remote files.

Pressing 's' will synchronize.

## Authentication

Expects `DROPBOX_ACCESS_TOKEN` environment variable. You obtain one by first
creating an application: https://www.dropbox.com/developers/apps. Go to settings
--> generated access token --> generate.
