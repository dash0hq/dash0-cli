package asset

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// IsYAMLFile reports whether path has a .yaml or .yml extension
// (case-insensitive) — the file types apply's directory scan and --since's
// git-ref/disk scans consider.
func IsYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// IsHiddenPath reports whether any slash-separated component of path starts
// with "." — used to skip hidden files and directories consistently across
// a directory walk (a single entry name), a git ls-tree listing (a full
// repo-relative path), and any other path-filtering scan.
func IsHiddenPath(path string) bool {
	for part := range strings.SplitSeq(path, "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// FindNonHiddenYAMLFiles returns a fs.WalkDirFunc for passing directly to
// filepath.WalkDir(root, ...): it appends every non-hidden .yaml/.yml file
// visited to *files and skips hidden files and directories (any path
// component starting with "."), via fs.SkipDir for directories. This is the
// one walk callback shared by apply's directory-scan and --since's
// disk-side scan, so both agree on what counts as an asset-definition file
// without hand-rolling the same callback twice; the filepath.WalkDir call
// itself stays at each call site for readability.
//
// root itself is exempt from the hidden-name check even if it starts with
// "." — an -f target the user named explicitly (e.g. -f .dash0-assets/) is
// a deliberate choice, not something to skip. Only path components *inside*
// root are checked. ListYAMLFilesAtRef (internal/git/plumbing.go) mirrors
// this same exemption for --since's git-ref-side scan, so a dot-prefixed
// -f target means the same thing on both sides of the diff.
//
// sawNestedDir, if non-nil, is set to true the first time the walk visits a
// non-hidden subdirectory of root — letting a caller tailor a "nothing
// found" message (e.g. "in <dir> and nested directories" vs a flat "in
// <dir>"). Pass nil when the caller doesn't need this.
func FindNonHiddenYAMLFiles(root string, files *[]string, sawNestedDir *bool) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			if IsHiddenPath(name) {
				return filepath.SkipDir
			}
			if sawNestedDir != nil {
				*sawNestedDir = true
			}
			return nil
		}
		if IsHiddenPath(name) || !IsYAMLFile(name) {
			return nil
		}
		*files = append(*files, path)
		return nil
	}
}
