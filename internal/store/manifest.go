package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExitCoder is implemented by store errors that map to a specific process exit
// code, so the CLI layer can translate a typed store error to an exit status
// without inspecting its concrete type.
type ExitCoder interface {
	error
	ExitCode() int
}

// EnvError is an environment-class store error (exit 3 by default): the repo is
// not set up the way a command needs it. Reason is a stable machine-readable
// discriminator the CLI can branch on for messaging.
type EnvError struct {
	Code   int    // process exit code; 3 when unset
	Reason string // one of the Reason* constants
	Msg    string
}

func (e *EnvError) Error() string { return e.Msg }

// ExitCode returns the process exit code this error maps to (3 by default).
func (e *EnvError) ExitCode() int {
	if e.Code == 0 {
		return 3
	}
	return e.Code
}

// Reason discriminators for EnvError.
const (
	ReasonNotARepo                 = "not_a_repo"
	ReasonProjectMarkerUnreadable  = "project_marker_unreadable"
	ReasonContributorUnset         = "contributor_unset"
	ReasonContributorUnknown       = "contributor_unknown"
	ReasonIdentityUnreadable       = "identity_unreadable"
	ReasonObsoleteIdentity         = "obsolete_contributor_identity"
	ReasonLocalSettingsUnreadable  = "local_settings_unreadable"
	ReasonMissingDesign            = "missing_design"
	ReasonMissingProjectKind       = "missing_project_kind"
	ReasonMissingContributors      = "missing_contributors"
	ReasonMissingCodeRoot          = "missing_code_root"
	ReasonInvalidCodeRootMode      = "invalid_code_root_mode"
	ReasonNoReferenceRoot          = "no_reference_root"
	ReasonSchemaVersionUnsupported = "schema_version_unsupported"
	ReasonMigrationWorkInFlight    = "migration_work_in_flight"
	ReasonMigrationReservedEpicKey = "migration_reserved_epic_key"
	ReasonOwnershipReviewRequired  = "ownership_review_required"
	ReasonOwnershipReviewInvalid   = "ownership_review_invalid"
)

// CurrentSchemaVersion is the on-disk schema_version this binary writes and
// the only one it reads. Three remedies apply depending on the stamp:
// below MinMigratableSchemaVersion, re-init as the move (artifact-schema.md's
// settled decision); from MinMigratableSchemaVersion through
// CurrentSchemaVersion-1, tiller upgrade --project-only (distribution.md's Versioning
// section); above CurrentSchemaVersion, upgrade the binary (since removing
// .tiller/ would discard data ahead of this binary).
const CurrentSchemaVersion = 12

// MinMigratableSchemaVersion is the oldest stamp tiller migrate can carry
// forward. Below it lies the 1-to-2 boundary, which shipped as a refusal with
// re-init as the move (no released binary and no surviving tree existed on its
// far side); from it upward, every boundary has a registered migration.
const MinMigratableSchemaVersion = 2

const (
	KindGreenfield      = "greenfield"
	KindBrownfield      = "brownfield"
	KindLegacyMigration = "legacy-migration"
	CodeModeSurface     = "surface"
	CodeModeReference   = "reference"
)

// ValidProjectKind reports whether kind names a supported project archetype.
func ValidProjectKind(kind string) bool {
	switch kind {
	case KindGreenfield, KindBrownfield, KindLegacyMigration:
		return true
	default:
		return false
	}
}

// Manifest is the root manifest, .tiller/tiller.yml (artifact-schema.md, "The
// root manifest"). It holds the gates, contributors, inputs, and conventions.
type Manifest struct {
	SchemaVersion int            `yaml:"schema_version"`
	SchemaEpoch   string         `yaml:"schema_epoch"`
	Project       ProjectBlock   `yaml:"project"`
	Contributors  []Contributor  `yaml:"contributors"`
	Inputs        InputsBlock    `yaml:"inputs,omitempty"`
	Gates         GatesConfig    `yaml:"gates,omitempty"`
	Git           GitBlock       `yaml:"git,omitempty"`
	Execution     ExecutionBlock `yaml:"execution,omitempty"`
	Drift         DriftBlock     `yaml:"drift,omitempty"`
	Usage         UsageBlock     `yaml:"usage,omitempty"`

	// root is the repo root the manifest was loaded from. It is not part of the
	// on-disk shape; LoadManifest sets it, and Validate's filesystem checks
	// resolve against it.
	root string
}

// UsageBlock is the opt-in agent usage setting. Capture is off unless the
// manifest says usage.capture: true; an absent block means off.
type UsageBlock struct {
	Capture bool `yaml:"capture"`
}

// ProjectBlock identifies the project and its archetype.
type ProjectBlock struct {
	Name           string         `yaml:"name"`
	Key            string         `yaml:"key"`
	Kind           string         `yaml:"kind"` // greenfield | brownfield | legacy-migration
	RequiresTiller string         `yaml:"requires_tiller,omitempty"`
	Adopted        *AdoptedAnchor `yaml:"adopted,omitempty"`
}

// AdoptedAnchor observes this repository's HEAD when Tiller first scaffolds it.
// It does not describe an external reference repository or assert a clean
// worktree. Old manifests and unavailable HEAD observations leave it absent.
type AdoptedAnchor struct {
	Commit string `yaml:"commit"`
	At     string `yaml:"at"` // UTC RFC3339
}

// Contributor is a human who holds work. A developer's agent acts as that developer.
type Contributor struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
}

// InputsBlock records where decomposition came from.
type InputsBlock struct {
	Docs         []string       `yaml:"docs,omitempty"`
	CurrentState []string       `yaml:"current_state,omitempty"`
	Code         []CodeRoot     `yaml:"code,omitempty"`
	ImportedFrom []ImportSource `yaml:"imported_from,omitempty"`
}

// ImportSource is one source folder a design layer was imported from at init
// (`tiller init --design`). It is provenance only: the canonical docs live under
// .tiller/design/ once imported. Path is the absolute source path read at
// import time, and At is the RFC3339 timestamp of the import, so source-document
// drift has a starting point to compare against later.
type ImportSource struct {
	Path string `yaml:"path"`
	At   string `yaml:"at,omitempty"`
}

// CodeRoot is one existing code root, tagged by how Tiller treats it.
type CodeRoot struct {
	Root string `yaml:"root"`
	Mode string `yaml:"mode"` // surface | reference
}

// ValidateMode checks the explicit mode without requiring other readiness inputs.
// Both readiness and surface recording use this rule.
func (c CodeRoot) ValidateMode(manifestPath string) *EnvError {
	if c.Mode == CodeModeSurface || c.Mode == CodeModeReference {
		return nil
	}
	return &EnvError{
		Reason: ReasonInvalidCodeRootMode,
		Msg: fmt.Sprintf("code root %q has invalid mode %q in %s; set mode to surface or reference",
			c.Root, c.Mode, manifestPath),
	}
}

// GatesConfig is the manifest's gate configuration: required (command-backed,
// blocking) and optional (attestation-backed, non-blocking) gate definitions.
// It is distinct from the per-node stored gate results.
type GatesConfig struct {
	Required map[string]GateSpec `yaml:"required,omitempty"`
	Optional map[string]GateSpec `yaml:"optional,omitempty"`
}

// GateSpec is one gate definition in the manifest: its kind, the command to run
// (for command gates), and the node grains it applies on.
type GateSpec struct {
	Kind  string     `yaml:"kind"` // command | attestation
	Run   string     `yaml:"run,omitempty"`
	On    StringList `yaml:"on,omitempty"`
	Reuse *bool      `yaml:"reuse,omitempty"` // false opts the gate out of reuse; absent means reuse
}

// Reusable reports whether a passing run of this gate may be reused on
// unchanged code (design 2026-10-21-gate-reuse). Reuse is on unless the
// manifest says `reuse: false`, so an existing manifest keeps working.
func (s GateSpec) Reusable() bool { return s.Reuse == nil || *s.Reuse }

// GitBlock holds the branch and PR conventions. IntegrationBranch is the branch
// merges land on, the one `sync` observes done crossings into; it is seeded at
// init from the branch the project was scaffolded on, and it is omitempty so a
// manifest written before the field existed still loads and round-trips
// unchanged (sync then falls back to the branch the checkout is standing on,
// which is what it observed before).
type GitBlock struct {
	BranchPattern     string `yaml:"branch_pattern,omitempty"`
	IntegrationBranch string `yaml:"integration_branch,omitempty"`
	PRCoherence       bool   `yaml:"pr_coherence,omitempty"`
}

// ExecutionBlock carries the project-level execution defaults. ReviewGrain is
// the project's default review grain, written by `tiller configure project`
// or by hand; the manifest stays human-owned either way.
type ExecutionBlock struct {
	ReviewGrain string `yaml:"review_grain,omitempty"`
}

// DriftBlock lists the enabled drift checks.
type DriftBlock struct {
	Checks []string `yaml:"checks,omitempty"`
}

// StringList is a list of strings that also accepts a bare scalar, so a gate's
// `on:` field reads whether it is written `[feature, task]` or `pr`.
type StringList []string

// UnmarshalYAML accepts either a scalar (one element) or a sequence.
func (s *StringList) UnmarshalYAML(node *yaml.Node) error {
	if node.Tag == "!!null" {
		return nil
	}
	if node.Kind == yaml.ScalarNode {
		*s = StringList{node.Value}
		return nil
	}
	var list []string
	if err := node.Decode(&list); err != nil {
		return err
	}
	*s = list
	return nil
}

// Includes reports whether the on: list names the noun.
func (s StringList) Includes(noun string) bool {
	for _, g := range s {
		if g == noun {
			return true
		}
	}
	return false
}

// LoadManifest reads .tiller/tiller.yml under root and returns the parsed
// manifest with its root recorded. If the manifest is absent, the directory is
// not a Tiller repo and the error is an *EnvError (exit 3).
func LoadManifest(root string) (*Manifest, error) {
	path := filepath.Join(root, ".tiller", "tiller.yml")
	data, err := readStateFile(root, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, &EnvError{
				Code:   3,
				Reason: ReasonNotARepo,
				Msg:    fmt.Sprintf("not a Tiller repo: %s not found", path),
			}
		}
		return nil, err
	}
	return ParseManifest(root, data)
}

// ParseManifest parses manifestData with the same schema checks as
// LoadManifest, recording root for later filesystem-aware validation.
func ParseManifest(root string, manifestData []byte) (*Manifest, error) {
	path := filepath.Join(root, ".tiller", "tiller.yml")

	// The schema_version node is probed before the full Manifest unmarshal below,
	// so a malformed or absent stamp is diagnosed on its own terms rather than as
	// a generic "cannot unmarshal into int" parse error (Manifest.SchemaVersion is
	// a plain int field, which a non-numeric stamp would otherwise fail on before
	// this function ever got to classify it). yaml.Node absorbs any node shape
	// (scalar, sequence, mapping, or absent) without erroring, so this unmarshal
	// fails only when the document does not even fit schemaVersionProbe's shape
	// (top-level YAML that is not a mapping) or carries a genuine syntax error,
	// either of which the full unmarshal below would reject identically.
	var probe schemaVersionProbe
	if err := yaml.Unmarshal(manifestData, &probe); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if refusal := refuseSchemaVersion(path, probe.SchemaVersion); refusal != nil {
		return nil, refusal
	}
	if err := rejectRetiredOwnership(path, manifestData, "claiming"); err != nil {
		return nil, err
	}
	if err := rejectLegacyContributorKey(path, manifestData, "owners"); err != nil {
		return nil, err
	}

	var m Manifest
	if err := yaml.Unmarshal(manifestData, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := validateStoredReviewGrain(path, "project", m.Execution.ReviewGrain); err != nil {
		return nil, err
	}

	m.root = root
	return &m, nil
}

// schemaVersionProbe is LoadManifest's pre-check target for the schema_version
// node. It is a named type, not an anonymous struct literal, because yaml.v3
// prints the Go target type into its own unmarshal-error text on a document
// that does not fit (a top-level sequence, say): an anonymous struct's
// registered type name is its full field-and-tag source, escaped quotes and
// all, where a named type prints as its plain package-qualified name.
type schemaVersionProbe struct {
	SchemaVersion yaml.Node `yaml:"schema_version"`
}

// missingStampRefusal builds the neutral *EnvError a missing schema_version key gets.
func missingStampRefusal(path string) *EnvError {
	return schemaVersionRefusal(fmt.Sprintf(
		"%s has no schema_version set; this tiller reads schema_version %d only: set schema_version by hand (%d, if this is a current-format project), since a missing stamp does not tell this binary whether the tree predates this version or simply lost the line",
		path, CurrentSchemaVersion, CurrentSchemaVersion,
	))
}

// invalidStampRefusal builds the neutral *EnvError a non-integer schema_version gets.
func invalidStampRefusal(path string, node yaml.Node) *EnvError {
	return schemaVersionRefusal(fmt.Sprintf(
		"%s's schema_version is not a valid integer (found %s); this tiller reads schema_version %d only: correct schema_version by hand to a plain integer, since this binary cannot tell from an unparseable value whether the tree predates or postdates what it reads",
		path, describeNode(node), CurrentSchemaVersion,
	))
}

// refuseSchemaVersion is artifact-schema.md's settled decision on schema_version
// and distribution.md's Versioning section, applied: this binary reads and
// writes the current schema only, and gives one of three remedies depending on
// what it can actually determine about the stamp in front of it. Only one of
// the three is destructive (it deletes .tiller/ entirely), and it is used
// only when an integer value actually places the tree on the old side of the
// boundary; every case this binary cannot place with that certainty gets a
// neutral, non-destructive remedy instead.
//
// A missing schema_version key, or one present with no value (YAML null,
// however written: bare, ~, or the word null), is NOT evidence of an old
// tree: a genuine version-1 tree always carried the key (the pre-refactor
// manifest struct declared SchemaVersion int, and every version-1 fixture in
// this codebase's history wrote schema_version: 1), so an absent or empty
// stamp reads instead as a dropped or truncated write on a tree of unknown
// vintage, current-format included. This gets the same neutral remedy as an
// unparseable value below: set schema_version by hand rather than delete
// anything.
//
// A value present but not decodable as an integer (a quoted string, a
// sequence, a mapping) gets the identical neutral treatment for the identical
// reason: this binary cannot tell which side of the boundary such a tree is
// on, or whether it is simply corrupted, and a destructive remedy needs to
// know the tree holds nothing worth keeping. The wording differs from the
// missing-key case, correcting a value that is there versus setting one that
// is not, so a reader knows which hand-fix applies.
//
// A value that does parse as an integer is the only case this binary can
// actually place on one side of the boundary. Below MinMigratableSchemaVersion
// is the 1-to-2 boundary distribution.md's Migration section describes: no
// released binary and no surviving tree exist on the version-1 side of it, so
// the refusal names the move that actually clears it, removing .tiller/ and
// re-running tiller init. Plain re-running tiller init without removing
// .tiller/ first does not work: init.go's scaffold validates an existing
// manifest before writing anything, so it reaches this same refusal a second
// time and exits 3 again with nothing rewritten. From MinMigratableSchemaVersion
// upward through CurrentSchemaVersion minus one, explicit project maintenance
// brings the tree forward. Newer than CurrentSchemaVersion
// is the opposite problem, and distribution.md says so directly: "a hard refuse,
// because a behind binary must not corrupt a tree that already migrated ahead of
// it." Removing .tiller/ there would discard real project data, so the remedy is
// to upgrade the binary, never to re-init.
func refuseSchemaVersion(path string, node yaml.Node) *EnvError {
	if node.Kind == 0 || node.Tag == "!!null" {
		return missingStampRefusal(path)
	}

	var version int
	if err := node.Decode(&version); err != nil {
		return invalidStampRefusal(path, node)
	}

	switch {
	case version == CurrentSchemaVersion:
		return nil
	case version < MinMigratableSchemaVersion:
		return schemaVersionRefusal(fmt.Sprintf(
			"%s is stamped schema_version %d; this tiller reads schema_version %d only: remove .tiller/ and run `tiller init` to start a current-format project, since there is no migration across this boundary",
			path, version, CurrentSchemaVersion,
		))
	case version < CurrentSchemaVersion:
		return schemaVersionRefusal(fmt.Sprintf(
			"%s is stamped schema_version %d; this tiller reads schema_version %d only: run `tiller upgrade --project-only` (or explicit `tiller migrate`) to bring the tree forward, then review and commit its changes",
			path, version, CurrentSchemaVersion,
		))
	default:
		return schemaVersionRefusal(fmt.Sprintf(
			"%s is stamped schema_version %d; this tiller reads schema_version %d only: this tree already moved past what this binary understands, so run `tiller upgrade` rather than re-init, which would discard project data ahead of this binary",
			path, version, CurrentSchemaVersion,
		))
	}
}

// schemaVersionRefusal builds the exit-3 EnvError every refuseSchemaVersion
// branch returns, differing only in msg.
func schemaVersionRefusal(msg string) *EnvError {
	return &EnvError{Code: 3, Reason: ReasonSchemaVersionUnsupported, Msg: msg}
}

// describeNode renders a YAML node's own text for a message, so a bad
// schema_version value is named rather than left implicit. A scalar shows its
// literal text (a quoted string is the common case: a hand-edit or a
// formatter that quotes scalars); a sequence or mapping shows its tag instead,
// since it has no single value to quote.
func describeNode(n yaml.Node) string {
	if n.Kind == yaml.ScalarNode {
		return fmt.Sprintf("%q", n.Value)
	}
	return n.ShortTag()
}

// SchemaVersionOf reports the integer stamp in manifestData. Missing and
// unparseable stamps get the same neutral refusals ParseManifest gives them;
// a parseable stamp is returned WITHOUT range classification, so maintenance
// can read a stamp ParseManifest would refuse.
func SchemaVersionOf(path string, manifestData []byte) (int, *EnvError) {
	var probe schemaVersionProbe
	if err := yaml.Unmarshal(manifestData, &probe); err != nil {
		return 0, schemaVersionRefusal(fmt.Sprintf("parse %s: %v", path, err))
	}
	if probe.SchemaVersion.Kind == 0 || probe.SchemaVersion.Tag == "!!null" {
		return 0, missingStampRefusal(path)
	}
	var version int
	if err := probe.SchemaVersion.Decode(&version); err != nil {
		return 0, invalidStampRefusal(path, probe.SchemaVersion)
	}
	return version, nil
}

// requiresTillerProbe is RequiresTillerOf's target: project.requires_tiller
// and nothing else, so the read cannot fail on any other field.
type requiresTillerProbe struct {
	Project struct {
		RequiresTiller string `yaml:"requires_tiller"`
	} `yaml:"project"`
}

// RequiresTillerOf reports the project.requires_tiller floor recorded in the
// manifest under root, trimmed, or "" when there is no manifest, the field is
// unset, or the document cannot be read as a YAML mapping at all. It is
// deliberately lenient and never errors: the advisory that reads it (the
// requires_tiller warning, distribution.md) must fire on a tree whose schema
// stamp ParseManifest refuses, because a binary behind the floor is exactly
// the binary most likely to be behind the schema too.
func RequiresTillerOf(root string) string {
	data, err := readStateFile(root, filepath.Join(root, ".tiller", "tiller.yml"))
	if err != nil {
		return ""
	}
	var probe requiresTillerProbe
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return ""
	}
	return strings.TrimSpace(probe.Project.RequiresTiller)
}

// Root returns the repo root the manifest was loaded from.
func (m *Manifest) Root() string { return m.root }

// SetRoot records the repo root for a manifest built in memory (for example by
// `tiller init` before the file is written), so Validate's filesystem checks
// have a base to resolve against.
func (m *Manifest) SetRoot(root string) { m.root = root }

// Validate is the readiness gate (cli-surface.md, `tiller check readiness`): it
// verifies design/ is non-empty, project.kind and contributors are set, and every
// declared code root exists on disk. It returns the first gap as an *EnvError,
// or nil when ready. Gaps reports the full set.
func (m *Manifest) Validate() error {
	gaps := m.Gaps()
	if len(gaps) == 0 {
		return nil
	}
	return gaps[0]
}

// Gaps returns every readiness gap, in a stable order, so check readiness can
// name them all. An empty result means ready.
func (m *Manifest) Gaps() []error {
	var gaps []error

	if strings.TrimSpace(m.Project.Kind) == "" {
		gaps = append(gaps, &EnvError{
			Code:   3,
			Reason: ReasonMissingProjectKind,
			Msg:    "project.kind is not set",
		})
	}

	if len(m.Contributors) == 0 {
		gaps = append(gaps, &EnvError{
			Code:   3,
			Reason: ReasonMissingContributors,
			Msg:    "no contributors yet: a human registers the first one with `tiller add contributor <id> --name \"<name>\"`",
		})
	}

	designDir := filepath.Join(m.root, ".tiller", "design")
	if !dirHasEntries(m.root, designDir) {
		gaps = append(gaps, &EnvError{
			Code:   3,
			Reason: ReasonMissingDesign,
			Msg:    "design/ is empty or absent",
		})
	}

	hasReference := false
	manifestPath := filepath.Join(m.root, ".tiller", "tiller.yml")
	for _, c := range m.Inputs.Code {
		if err := c.ValidateMode(manifestPath); err != nil {
			gaps = append(gaps, err)
		}
		if c.Mode == CodeModeReference {
			hasReference = true
		}
		p := c.Root
		if !filepath.IsAbs(p) {
			p = filepath.Join(m.root, p)
		}
		if _, err := os.Stat(p); err != nil {
			gaps = append(gaps, &EnvError{
				Code:   3,
				Reason: ReasonMissingCodeRoot,
				Msg:    fmt.Sprintf("declared code root %q does not exist", c.Root),
			})
		}
	}
	if m.Project.Kind == KindLegacyMigration && !hasReference {
		gaps = append(gaps, &EnvError{
			Reason: ReasonNoReferenceRoot,
			Msg:    fmt.Sprintf("legacy-migration needs an existing code root tagged mode: reference in %s; declare the old system under inputs.code", manifestPath),
		})
	}

	return gaps
}

// ResolveContributor looks up the acting contributor. envOrFlag is the already
// resolved identity (--contributor over TILLER_CONTRIBUTOR, decided by the
// caller). An unset or unknown contributor is an *EnvError (exit 3): Tiller
// refuses rather than guess. The unset refusal names why a checkout that
// another checkout of the same repository already joined can still have no
// identity: .tiller/identity.yml is gitignored, so every new clone or Git
// worktree starts without one until join runs there.
func ResolveContributor(m *Manifest, envOrFlag string) (Contributor, error) {
	id := strings.TrimSpace(envOrFlag)
	if id == "" {
		return Contributor{}, &EnvError{
			Code:   3,
			Reason: ReasonContributorUnset,
			Msg:    "no contributor on this checkout: identity is per checkout (.tiller/identity.yml is gitignored), so a new clone or worktree has none until `tiller join --as <you>` runs there (or pass --contributor / set TILLER_CONTRIBUTOR)",
		}
	}
	for _, o := range m.Contributors {
		if o.ID == id {
			return o, nil
		}
	}
	return Contributor{}, &EnvError{
		Code:   3,
		Reason: ReasonContributorUnknown,
		Msg:    fmt.Sprintf("contributor %q is not in the project contributors", id),
	}
}

// RenderSchemaVersion rewrites only the schema_version line of manifestData,
// leaving every other byte intact, the same surgical posture RenderContributors
// takes with the contributors block: the manifest is human-owned, and a migration
// owns exactly the stamp.
func RenderSchemaVersion(manifestData []byte, version int) ([]byte, error) {
	var probe schemaVersionProbe
	if err := yaml.Unmarshal(manifestData, &probe); err != nil {
		return nil, err
	}
	stamp := probe.SchemaVersion
	if stamp.Kind == 0 {
		return nil, fmt.Errorf("manifest has no schema_version line to update")
	}
	if stamp.Kind != yaml.ScalarNode || stamp.Tag != "!!int" {
		return nil, fmt.Errorf("manifest has no integer schema_version to update")
	}
	lines := strings.SplitAfter(string(manifestData), "\n")
	line := lines[stamp.Line-1]
	at := yamlColumnOffset(line, stamp.Column)
	if !strings.HasPrefix(line[at:], stamp.Value) {
		return nil, fmt.Errorf("cannot safely update schema_version at line %d", stamp.Line)
	}
	lines[stamp.Line-1] = line[:at] + fmt.Sprint(version) + line[at+len(stamp.Value):]
	return []byte(strings.Join(lines, "")), nil
}

// ParseManifestUnchecked parses manifestData with no schema_version refusal.
// Maintenance uses it to close a version gap; ordinary tree reads go through
// ParseManifest.
func ParseManifestUnchecked(root string, manifestData []byte) (*Manifest, error) {
	path := filepath.Join(root, ".tiller", "tiller.yml")
	var m Manifest
	if err := yaml.Unmarshal(manifestData, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.SchemaVersion < 9 {
		var legacy struct {
			Owners []Contributor `yaml:"owners"`
		}
		if err := yaml.Unmarshal(manifestData, &legacy); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		var fields map[string]yaml.Node
		if err := yaml.Unmarshal(manifestData, &fields); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if _, exists := fields["contributors"]; exists {
			return nil, fmt.Errorf("%s contains contributors before schema 9; restore the original owners roster before migration", path)
		}
		m.Contributors = legacy.Owners
	} else if err := rejectLegacyContributorKey(path, manifestData, "owners"); err != nil {
		return nil, err
	}
	if err := validateStoredReviewGrain(path, "project", m.Execution.ReviewGrain); err != nil {
		return nil, err
	}
	m.root = root
	return &m, nil
}

func dirHasEntries(root, dir string) bool {
	entries, err := readStateDirectory(root, dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Name() != "baseline.yml" {
			return true
		}
	}
	return false
}
