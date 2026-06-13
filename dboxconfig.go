package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DboxConfig describes a folder to manage: which local files to push and where
// to put them in Dropbox. It is loaded from the YAML file passed on the command
// line (e.g. `dbox dbox.yaml`).
type DboxConfig struct {
	// Remote is the full Dropbox path the local files are pushed to, e.g.
	// "/sequences/airy-dissonance". Normalized to a leading slash and no
	// trailing slash.
	Remote string `yaml:"remote"`

	// FileTypes is the list of file extensions to include (e.g. ["wav"]).
	// Extensions are normalized to lowercase without a leading dot.
	FileTypes []string `yaml:"file_types"`

	// Collaborators is reserved for future authoritative collaborator
	// management. It is parsed but not acted on yet.
	Collaborators []string `yaml:"collaborators"`
}

// LoadDboxConfig reads, parses, validates and normalizes a management-mode
// config file.
func LoadDboxConfig(path string) (*DboxConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not read config %q: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	// Reject unknown keys so typos surface as errors instead of being silently
	// ignored.
	dec.KnownFields(true)

	var cfg DboxConfig
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("could not parse config %q: %w", path, err)
	}

	if err := cfg.normalizeAndValidate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// normalizeAndValidate cleans up user-provided values and enforces the
// requirements (a remote path and at least one file type).
func (c *DboxConfig) normalizeAndValidate() error {
	c.Remote = normalizeRemotePath(c.Remote)
	if c.Remote == "" {
		return fmt.Errorf("config: %q is required", "remote")
	}

	var types []string
	for _, t := range c.FileTypes {
		t = strings.ToLower(strings.TrimSpace(t))
		t = strings.TrimPrefix(t, ".")
		if t != "" {
			types = append(types, t)
		}
	}
	if len(types) == 0 {
		return fmt.Errorf("config: %q must list at least one extension", "file_types")
	}
	c.FileTypes = types

	// Collaborators are optional. Normalize emails (lowercase, trimmed) and
	// drop blanks/duplicates so the remote/local diff is case-insensitive.
	var collaborators []string
	seen := make(map[string]bool)
	for _, email := range c.Collaborators {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		collaborators = append(collaborators, email)
	}
	c.Collaborators = collaborators

	return nil
}

// normalizeRemotePath ensures a Dropbox path has a single leading slash and no
// trailing slash. An empty (or slash-only) path normalizes to "".
func normalizeRemotePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	return "/" + p
}

// matchesFileType reports whether name has one of the configured extensions.
func (c *DboxConfig) matchesFileType(name string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepathExt(name), "."))
	for _, t := range c.FileTypes {
		if ext == t {
			return true
		}
	}
	return false
}

// filepathExt returns the extension of name (the suffix after the final dot),
// without the dot. It mirrors filepath.Ext but trims the leading dot so it can
// be compared directly against the normalized FileTypes.
func filepathExt(name string) string {
	for i := len(name) - 1; i >= 0 && name[i] != '/'; i-- {
		if name[i] == '.' {
			return name[i+1:]
		}
	}
	return ""
}
