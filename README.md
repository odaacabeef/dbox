# dbox

Terminal application for working with Dropbox.

## Usage

The primary function is browsing files and selectively downloading them.

Download path is hardcoded to `~/.dbox/`.

Use ' ' (space) to select files and/or folders and press 'd' to download them.

## Authentication

Expects `DROPBOX_ACCESS_TOKEN` environment variable. You obtain one by first
creating an application: https://www.dropbox.com/developers/apps. Go to settings
--> generated access token --> generate.
