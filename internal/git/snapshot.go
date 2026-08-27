package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"gopkg.in/yaml.v3"
)

// IdentifierKey uniquely identifies one asset across a Snapshot — kind plus
// its upsert identifier (id or origin, depending on the kind; see
// asset.ExtractIdentifier). Keying by kind as well as identifier means two
// different asset kinds can never collide even if their identifier strings
// happen to match.
type IdentifierKey struct {
	Kind       string
	Identifier string
}

// NoIdentifierDoc records a document that carries no stable identifier: its
// kind, and the file path it came from (without any multi-document suffix).
type NoIdentifierDoc struct {
	Kind     string
	FilePath string
}

// Snapshot is the set of asset identifiers found across a scanned scope (a
// directory or a single file) at one point in time — either a git ref or the
// current disk contents. --since's deletion detection is a diff between two
// Snapshots, never a commit-by-commit history scan.
type Snapshot struct {
	// Identifiers maps every document's (kind, identifier) to the
	// repo-relative (or scope-relative) path it was found at, for every
	// document that carries a stable identifier.
	Identifiers map[IdentifierKey]string

	// NoIdentifier maps the doc path (the file path, plus a "#<index>"
	// suffix for the second and later documents in a multi-document file)
	// of every document with no stable identifier to its details. Diff
	// checks FilePath (not the doc path itself) against the other
	// snapshot's Paths, since a no-identifier document can only be
	// identified as "deleted" by its underlying file disappearing.
	NoIdentifier map[string]NoIdentifierDoc

	// PrometheusAlertsByIdentifier maps a PrometheusRule CRD's identifier to
	// the (group, alert) pairs it contains, for detecting an individual
	// alerting rule removed from a CRD that otherwise still exists.
	PrometheusAlertsByIdentifier map[string][]asset.PrometheusAlertName

	// PrometheusRecordingRoleByIdentifier maps a PrometheusRule CRD's
	// identifier to whether it has at least one recording rule. Recorded for
	// every PrometheusRule CRD identifier found, even when false, so Diff
	// can tell "this CRD never had a recording role" apart from "this CRD
	// doesn't exist in this snapshot at all" -- the same map-presence
	// pattern PrometheusAlertsByIdentifier already relies on. Diff uses this
	// to detect a CRD that survives but whose recording-rule role
	// disappeared entirely (its last `record:` entry removed), a case a
	// per-alert-name diff can't catch: Dash0 models a CRD's recording rules
	// as one server-side resource, not one per record.
	PrometheusRecordingRoleByIdentifier map[string]bool

	// SpamFilterUsesOriginByIdentifier maps a spam filter's identifier to
	// whether it carries a dash0.com/origin label (per
	// asset.SpamFilterUsesOrigin). Diff carries this into Deletion so --since
	// can warn when deleting an ID-only spam filter, whose id may have been
	// reassigned server-side since this identifier was recorded.
	SpamFilterUsesOriginByIdentifier map[string]bool

	// Paths is the set of every file path scanned, regardless of whether it
	// parsed into a recognized kind. Used to check whether a NoIdentifier
	// document's file still exists at all in the other snapshot.
	Paths map[string]bool

	// RawContent maps every file path scanned to its raw content, as read at
	// that point in time. Populated only by BuildSnapshotFromRef (nil/empty
	// from BuildSnapshotFromDisk, which has no caller that needs it): every
	// file --since's deletion detection ever names came from the "before"
	// (ref) snapshot, so retaining the content already read while building
	// it lets a caller resolve a deletion's display name (see
	// resolveDeletionNames in internal/apply/since.go) from memory instead
	// of shelling out to `git cat-file` a second time for the same blob.
	RawContent map[string][]byte
}

func newSnapshot() Snapshot {
	return Snapshot{
		Identifiers:                         map[IdentifierKey]string{},
		NoIdentifier:                        map[string]NoIdentifierDoc{},
		PrometheusAlertsByIdentifier:        map[string][]asset.PrometheusAlertName{},
		PrometheusRecordingRoleByIdentifier: map[string]bool{},
		SpamFilterUsesOriginByIdentifier:    map[string]bool{},
		Paths:                               map[string]bool{},
		RawContent:                          map[string][]byte{},
	}
}

// BuildSnapshotFromRef builds a Snapshot from the contents of scope (a
// repo-relative directory or file path; "" scans the whole repo) as they
// existed at ref.
func BuildSnapshotFromRef(ctx context.Context, repo Repo, ref, scope string) (Snapshot, error) {
	files, err := repo.ListYAMLFilesAtRef(ctx, ref, scope)
	if err != nil {
		return Snapshot{}, err
	}

	contents, err := repo.readFilesAtRef(ctx, ref, files)
	if err != nil {
		return Snapshot{}, err
	}

	snap := newSnapshot()
	for i, path := range files {
		snap.Paths[path] = true
		snap.RawContent[path] = contents[i]
		if err := ingestDocuments(&snap, path, contents[i]); err != nil {
			return Snapshot{}, fmt.Errorf("%s at %s: %w", path, ref, err)
		}
	}
	return snap, nil
}

// BuildSnapshotFromDisk builds a Snapshot from the current contents of scope
// on disk (an absolute or working-directory-relative directory or file
// path). Hidden files and directories are skipped, matching the git-ref side
// (ListYAMLFilesAtRef) and apply's own discoverFiles behavior.
//
// repoRoot anchors the relative paths recorded in the returned Snapshot: it
// must be the same repository root used to resolve the ref passed to
// BuildSnapshotFromRef, so the two Snapshots' paths line up for Diff's
// NoIdentifier check (git ls-tree always prints paths relative to the repo
// root, regardless of any pathspec scope, so the disk side must match that
// convention rather than being relative to scope itself).
//
// ctx is honored for cancellation between files (checked once per visited
// entry) — this function does no I/O that itself accepts a context today,
// but taking one keeps the signature consistent with the rest of this
// package's public API and forward-compatible with future callers that need
// to bound how long a large directory scan can run.
//
// scope not existing on disk at all is not an error: every asset definition
// under it may have been deleted, taking the directory itself with them (or,
// for a single-file scope, the one file it named). That carries the same
// meaning as an existing-but-empty directory -- the "after" state has
// nothing -- so every identifier the "before" snapshot (BuildSnapshotFromRef)
// found becomes a deletion candidate, the same as it would for a survived,
// merely-emptied directory.
func BuildSnapshotFromDisk(ctx context.Context, scope, repoRoot string) (Snapshot, error) {
	info, err := os.Stat(scope)
	if err != nil {
		if os.IsNotExist(err) {
			return newSnapshot(), nil
		}
		return Snapshot{}, fmt.Errorf("failed to stat %s: %w", scope, err)
	}

	snap := newSnapshot()

	ingest := func(path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(repoRoot, absPath)
		if err != nil {
			return fmt.Errorf("failed to compute path relative to repo root %s: %w", repoRoot, err)
		}
		relPath = filepath.ToSlash(relPath)
		snap.Paths[relPath] = true
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}
		if err := ingestDocuments(&snap, relPath, data); err != nil {
			return fmt.Errorf("%s: %w", relPath, err)
		}
		return nil
	}

	if !info.IsDir() {
		// No extension check here: scope is a single file the caller (or the
		// user, via -f) named explicitly, and apply's own single-file
		// create/update path (readMultiDocumentYAML) has no extension check
		// either — a -f config.json target must be scanned by --since the
		// same way it's read by every other apply path, not silently
		// excluded from the snapshot because of its extension. Matches
		// ListYAMLFilesAtRef's equivalent exemption for a single-file scope
		// on the git-ref side.
		if err := ingest(scope); err != nil {
			return Snapshot{}, err
		}
		return snap, nil
	}

	var paths []string
	if err := filepath.WalkDir(scope, asset.FindNonHiddenYAMLFiles(scope, &paths, nil)); err != nil {
		return Snapshot{}, err
	}
	for _, path := range paths {
		if err := ingest(path); err != nil {
			return Snapshot{}, err
		}
	}
	return snap, nil
}

// ingestDocuments splits data (which may be a multi-document YAML stream)
// and records each document's identifier (or lack thereof) into snap under
// path. Multiple documents in one file are distinguished in NoIdentifier by
// appending a "#<index>" suffix to path for the second and later documents.
func ingestDocuments(snap *Snapshot, path string, data []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	index := 0
	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to parse YAML: %w", err)
		}
		// Skip empty documents (e.g. a trailing "---" with nothing after
		// it), matching apply's readMultiDocumentYAML.
		if node.Kind == 0 {
			continue
		}

		docBytes, err := yaml.Marshal(&node)
		if err != nil {
			return fmt.Errorf("failed to re-marshal document: %w", err)
		}

		docPath := path
		if index > 0 {
			docPath = fmt.Sprintf("%s#%d", path, index)
		}
		index++

		kind, err := dash0yaml.DetectKind(docBytes)
		if err != nil {
			return fmt.Errorf("failed to detect kind: %w", err)
		}
		if kind == "" || !asset.IsValidKind(kind) {
			// Either no recognizable kind at all, or a kind Dash0 doesn't
			// know about (e.g. a stray Kubernetes ConfigMap sitting in a
			// scanned scope's git history). apply's own document validation
			// already hard-fails on an unsupported kind for the *current*
			// contents of -f; --since additionally scans historical content
			// the live apply path never looks at, so a since-deleted,
			// unrelated document must not abort the whole deletion
			// computation just because it isn't a Dash0 asset.
			continue
		}

		identifier, err := asset.ExtractIdentifier(docBytes)
		if err != nil {
			return fmt.Errorf("failed to extract identifier: %w", err)
		}

		normalizedKind := asset.NormalizeKind(kind)
		if identifier == "" {
			snap.NoIdentifier[docPath] = NoIdentifierDoc{Kind: normalizedKind, FilePath: path}
			continue
		}
		snap.Identifiers[IdentifierKey{Kind: normalizedKind, Identifier: identifier}] = docPath

		if normalizedKind == "prometheusrule" {
			// asset.ExtractPrometheusAlertNames, not
			// dash0yaml.ExtractPrometheusAlertNames: the dash0yaml version
			// unmarshals into a *string struct field via sigs.k8s.io/yaml,
			// which silently corrupts an alert name that happens to be a
			// YAML boolean literal (e.g. "Y", "no") into "true"/"false".
			// asset's version reads the raw YAML node value instead. See
			// asset.ExtractPrometheusAlertNames's doc comment.
			alerts, err := asset.ExtractPrometheusAlertNames(docBytes)
			if err != nil {
				return fmt.Errorf("failed to extract alert names: %w", err)
			}
			snap.PrometheusAlertsByIdentifier[identifier] = alerts

			hasRecordingRule, err := asset.PrometheusRuleHasRecordingRule(docBytes)
			if err != nil {
				return fmt.Errorf("failed to determine recording rule presence: %w", err)
			}
			snap.PrometheusRecordingRoleByIdentifier[identifier] = hasRecordingRule
		}

		if normalizedKind == "spamfilter" {
			usesOrigin, err := asset.SpamFilterUsesOrigin(docBytes)
			if err != nil {
				return fmt.Errorf("failed to determine spam filter identifier source: %w", err)
			}
			snap.SpamFilterUsesOriginByIdentifier[identifier] = usesOrigin
		}
	}
	return nil
}
