package store

import "time"

// This file holds the typed structs for every addressable artifact in the work
// tree (artifact-schema.md: Epics, Features, Tasks, and the open-question
// ledger). The eval contract lives in eval.go and the stored gate results live
// in gates.go.

// Noun values: the validated noun each command takes (cli-surface.md, Grammar).
const (
	NounEpic      = "epic"
	NounFeature   = "feature"
	NounTask      = "task"
	NounQuestion  = "question"
	NounAmendment = "amendment"
	// Shelf nouns carry the task lifecycle on independent shelves under .tiller/.
	NounBug   = "bug"
	NounChore = "chore"
	NounTweak = "tweak"
)

// Node is the common read view over any addressable artifact, so the resolver
// and the read commands can hold a value without switching on its concrete
// type. NodeKey is a *string because the key is class-dependent: folder-backed
// nodes (epic, feature, task) carry one, while the question ledger item returns
// nil (artifact-schema.md, References and IDs).
type Node interface {
	NodeID() string
	NodeKey() *string
	Noun() string
	NodeStatus() string
}

// Surface is the existing code a node touches: empty for a greenfield create,
// populated for a brownfield modify/replace/migrate (artifact-schema.md,
// change_type and surface). Paths is the only axis: modules was cut (drift
// design decision 9) for having no writer, no defined domain, and no resolver
// short of reasoning about code.
type Surface struct {
	Paths []string `yaml:"paths,omitempty"`
}

// BasisRefs pins a spec and an eval revision. It is the shape of
// accepted_against, the active-work contract a human accepts before work begins
// (artifact-schema.md, Accepted basis).
type BasisRefs struct {
	Spec RevisionRef `yaml:"spec,omitempty"`
	Eval RevisionRef `yaml:"eval,omitempty"`
	// Commit pins HEAD at the moment of approval: not the state files, which are
	// usually uncommitted then, but the code baseline the plan was approved
	// against, the anchor a later drift candidate's since-when and an
	// amendment's diff range read from (artifact-schema.md, Accepted basis).
	Commit RevisionRef `yaml:"commit,omitempty"`
}

// VerifiedAgainst is the completion record set at done: the spec and eval
// revisions verified, plus the code-surface fingerprints (artifact-schema.md,
// Drift control hooks, verified_against). Surfaces carries no omitempty: a
// materialized verified_against always records surfaces as a list, the empty
// list `[]` for a create feature that touched no surface, so the completion
// record never silently drops the key. A node that never reached done has a zero
// verified_against and is omitted whole by the parent's omitempty.
type VerifiedAgainst struct {
	Spec RevisionRef `yaml:"spec,omitempty"`
	Eval RevisionRef `yaml:"eval,omitempty"`
	// Commit pins the integration commit sync observed at the crossing: the
	// since-when anchor a later drift
	// candidate reads from, same leg as BasisRefs.Commit one state earlier
	// (artifact-schema.md, Accepted basis; Epics, verified_against).
	Commit   RevisionRef          `yaml:"commit,omitempty"`
	Surfaces []SurfaceFingerprint `yaml:"surfaces"`
}

// SurfaceFingerprint is one git_tree or git_blob RevisionRef plus the inventory
// of paths it covered when verified (a blob's inventory is the file itself), so
// a later tree comparison can tell which files moved out from under the
// completion record (artifact-schema.md, Epics).
type SurfaceFingerprint struct {
	Path       string    `yaml:"path,omitempty"`
	Kind       string    `yaml:"kind,omitempty"`
	Value      string    `yaml:"value,omitempty"`
	Inventory  []string  `yaml:"inventory,omitempty"`
	ObservedAt time.Time `yaml:"observed_at,omitempty"`
}

// Assumption records a node proceeding under an open question: the human reason
// for proceeding and, once the question is answered and reviewed in, the answer
// revision (artifact-schema.md, Assumption basis). The on-disk inner shape is
// not pinned by the schema; this captures the two fields the schema and
// cli-surface describe.
type Assumption struct {
	Reason         string      `yaml:"reason,omitempty"`
	AnswerRevision RevisionRef `yaml:"answer_revision,omitempty"`
}

// AssumptionBasis maps a question id to the assumption the node rests on.
type AssumptionBasis map[string]Assumption

// ReviewRef records a submitted branch and its head commit. At task grain it
// is the remote-less review surface; tasks riding an aggregate review retain it
// as the immutable submission anchor the aggregate must contain.
type ReviewRef struct {
	Branch string      `yaml:"branch,omitempty"`
	Head   RevisionRef `yaml:"head,omitempty"`
}

// Epic is a coherent body of work owned over weeks, the top of the work tree
// (artifact-schema.md, Epics). Its on-disk state is epic.yml.
//
// An epic carries no gates block and no eval file of its own. Its acceptance bar
// is the sum of its features' contracts, each enforced at feature grain, so
// `eval`, `check`, and `attest` all stop at the feature (artifact-schema.md,
// Epics; Gates). VerifiedAgainst is the one completion record it does keep, and
// its Eval leg stays zero for the same reason: there is no epic contract to have
// verified against.
type Epic struct {
	ID        string   `yaml:"id"`
	Key       *string  `yaml:"key,omitempty"`
	Title     string   `yaml:"title,omitempty"`
	Status    string   `yaml:"status,omitempty"`
	Blocked   bool     `yaml:"blocked"`
	Source    string   `yaml:"source,omitempty"`
	DependsOn []string `yaml:"depends_on,omitempty"`

	// Order is the recorded soft order of this epic's features: intent, not
	// constraint. It arranges rendering and steers routing recommendations
	// wherever depends_on leaves the choice open, and it never gates
	// (docs/plans/2026-08-08-soft-order-design.md). Written only by
	// mutate.SetOrder; ids, a partial list legal.
	Order []string `yaml:"order,omitempty"`

	// ReviewGrain is this node's explicit review-grain override: the grain at
	// which the review PR opens for work under it (task | feature at feature
	// grain; task | feature | epic at epic grain). Absent means inherit; the
	// resolver in reviewgrain.go owns precedence
	// (docs/plans/2026-08-26-review-grain-design.md, The setting).
	ReviewGrain string `yaml:"review_grain,omitempty"`

	// Branch is the epic aggregate review branch, recorded at the first
	// epic-grain task start. Task implementation branches remain separate.
	Branch string `yaml:"branch,omitempty"`

	// PR and ReviewRef are the epic's review record, set only when the epic
	// itself is the reviewed node: a PR represents exactly one node, and at
	// epic grain that node is the epic (docs/plans/2026-08-26-review-grain-design.md,
	// Schema and the PR record, softening "a PR always represents a task").
	// PR is the number when the repository has a remote; ReviewRef is the
	// branch-plus-head fallback when it does not. Both stay nil at task and
	// feature grain.
	//
	// The console projects both through NodeView.PR/ReviewRef, the identical
	// fields a task's own page has always carried, since a PR represents
	// exactly one node whichever grain it rides at.
	PR              *int             `yaml:"pr,omitempty"`
	ReviewRef       *ReviewRef       `yaml:"review_ref,omitempty"`
	RecoveredReview *RecoveredReview `yaml:"recovered_review,omitempty"`

	Surface         Surface         `yaml:"surface,omitempty"`
	VerifiedAgainst VerifiedAgainst `yaml:"verified_against,omitempty"`

	// DoneBasis records what the epic's done crossing rested on. An epic has
	// exactly one value, "rollup": epic done is a computed fact sync records
	// when the last non-cancelled feature crosses, never a verb
	// (artifact-schema.md, Epics States and Done basis). The field carries the
	// same token the crossing's event does, so a reader of epic.yml never has
	// to infer the basis from an absence.
	DoneBasis string `yaml:"done_basis,omitempty"`
}

func (e *Epic) NodeID() string     { return e.ID }
func (e *Epic) NodeKey() *string   { return e.Key }
func (e *Epic) Noun() string       { return NounEpic }
func (e *Epic) NodeStatus() string { return e.Status }

// Feature is the grain where acceptance lives (artifact-schema.md, Features).
// Its on-disk state is feature.yml. The embedded GatesBlock carries the stored
// gates, attestations, and policy_violations as the top-level siblings the
// schema shows.
//
// Branch is the aggregate review branch at feature or epic grain. Task-grain
// features do not carry a branch; every task records its own working branch.
// PR and ReviewRef belong here only when the feature is the reviewed node.
type Feature struct {
	ID         string   `yaml:"id"`
	Key        *string  `yaml:"key,omitempty"`
	Title      string   `yaml:"title,omitempty"`
	Status     string   `yaml:"status,omitempty"`
	Blocked    bool     `yaml:"blocked"`
	Source     string   `yaml:"source,omitempty"`
	ChangeType string   `yaml:"change_type,omitempty"`
	Surface    Surface  `yaml:"surface,omitempty"`
	DependsOn  []string `yaml:"depends_on,omitempty"`

	// Order is the recorded soft order of this feature's tasks: intent, not
	// constraint. It arranges rendering and steers routing recommendations
	// wherever depends_on leaves the choice open, and it never gates
	// (docs/plans/2026-08-08-soft-order-design.md). Written only by
	// mutate.SetOrder; ids, a partial list legal.
	Order []string `yaml:"order,omitempty"`

	// ReviewGrain is this node's explicit task, feature, or epic review-grain
	// override. Reviewed moves materialize inherited grain when placement would
	// otherwise change it. Absence inherits through reviewgrain.go.
	ReviewGrain string `yaml:"review_grain,omitempty"`

	// Supersedes and SupersededBy are the two directions of one recorded
	// decision; Task carries the same pair, and its comment holds the rule for
	// both grains.
	Supersedes      []string         `yaml:"supersedes,omitempty"`
	SupersededBy    *string          `yaml:"superseded_by,omitempty"`
	Branch          string           `yaml:"branch,omitempty"`
	PR              *int             `yaml:"pr,omitempty"`
	ReviewRef       *ReviewRef       `yaml:"review_ref,omitempty"`
	RecoveredReview *RecoveredReview `yaml:"recovered_review,omitempty"`
	AcceptedAgainst BasisRefs        `yaml:"accepted_against,omitempty"`
	AssumptionBasis AssumptionBasis  `yaml:"assumption_basis,omitempty"`
	VerifiedAgainst VerifiedAgainst  `yaml:"verified_against,omitempty"`

	// DoneBasis records what the done crossing rested on, written by sync on
	// every crossing it records. A feature has two possible values, "rollup" and
	// "human_attested" (artifact-schema.md, Done basis; mutate/sync.go's
	// featureBasis decides between them). It is the node's durable record of
	// which of the two, so a reader never has to infer it from an absent value;
	// a node whose done predates the field carries none.
	DoneBasis  string `yaml:"done_basis,omitempty"`
	GatesBlock `yaml:",inline"`

	// EpicKey is the owning epic's key, set from the directory layout on Load.
	// It is not part of the on-disk feature.yml shape (the link is the folder).
	EpicKey string `yaml:"-"`

	// HasEval reports whether the feature's feature.eval.md acceptance contract
	// exists on disk, set from the directory layout on Load like EpicKey, so a
	// pure reader can name the eval file without re-statting.
	HasEval bool `yaml:"-"`
}

func (f *Feature) NodeID() string     { return f.ID }
func (f *Feature) NodeKey() *string   { return f.Key }
func (f *Feature) Noun() string       { return NounFeature }
func (f *Feature) NodeStatus() string { return f.Status }

// Task is the level where code gets written and the terminus of the prose
// chain, laid down just in time when its feature is specified
// (artifact-schema.md, Tasks). It is a folder node like the epic and the feature
// above it: task.md holds the implementation spec and task.yml, this shape,
// holds the state. Its canonical Key is the three-segment
// <epic-key>/<feature-key>/<task-key> (artifact-schema.md, Keys and paths).
type Task struct {
	ID         string   `yaml:"id"`
	Key        *string  `yaml:"key,omitempty"`
	Title      string   `yaml:"title,omitempty"`
	Status     string   `yaml:"status,omitempty"`
	Blocked    bool     `yaml:"blocked"`
	Started    bool     `yaml:"started"`
	Owner      string   `yaml:"owner,omitempty"`
	ProposedBy string   `yaml:"proposed_by,omitempty"`
	DependsOn  []string `yaml:"depends_on,omitempty"`

	// Replacement lineage is written atomically by amendment application or
	// transfer. Both ends retain the immutable IDs of the same-noun nodes.
	Supersedes   []string `yaml:"supersedes,omitempty"`
	SupersededBy *string  `yaml:"superseded_by,omitempty"`

	// AcceptedAgainst pins the task.md blob the human approved, the same
	// active-work contract Feature.AcceptedAgainst holds one grain up
	// (artifact-schema.md, Tasks: States). It is a pointer because a task before
	// its approve has nothing pinned at all, which is a different fact from a
	// pin whose legs are empty. The schema shows a spec leg alone here: the
	// acceptance contract enters the tree at feature grain, so there is no task
	// eval file for an Eval leg to name (artifact-schema.md, Gates).
	AcceptedAgainst *BasisRefs `yaml:"accepted_against,omitempty"`

	// Branch is the task's working branch, recorded at start from its full
	// canonical key and retained unchanged on resume.
	Branch string `yaml:"branch,omitempty"`

	// StartedGrain is the review grain this task resolved at its first start
	// (issue #146). A task keeps it through later grain changes, so one started
	// at task grain still takes its own PR under a coarse feature. Tasks
	// started before the field existed leave it empty and resolve through
	// their parents (TaskReviewGrain).
	StartedGrain string `yaml:"started_grain,omitempty"`

	Evidence  []Evidence `yaml:"evidence,omitempty"`
	PR        *int       `yaml:"pr,omitempty"`
	ReviewRef *ReviewRef `yaml:"review_ref,omitempty"`

	// DoneBasis records what the task's done crossing rested on. A task has three
	// possible values, "ancestry", "content", and "human_attested"
	// (artifact-schema.md, Done basis; mutate/sync.go's syncTasks writes it). It
	// serves the same purpose Feature.DoneBasis does one grain up, from a
	// different set.
	DoneBasis  string `yaml:"done_basis,omitempty"`
	GatesBlock `yaml:",inline"`

	// FeatureID is the owning feature's id, set from the directory layout on
	// Load. It is not part of the on-disk task.yml shape (the link is the folder).
	FeatureID string `yaml:"-"`
}

func (t *Task) NodeID() string     { return t.ID }
func (t *Task) NodeKey() *string   { return t.Key }
func (t *Task) Noun() string       { return NounTask }
func (t *Task) NodeStatus() string { return t.Status }

// Question is an owned, date-gated open question, a ledger entry in
// open-questions.yml (artifact-schema.md, Decisions and open questions). It is
// the one node class that carries an id and no key.
type Question struct {
	ID             string      `yaml:"id"`
	Key            *string     `yaml:"-"` // ledger item: never keyed, always nil
	Title          string      `yaml:"title,omitempty"`
	Statement      string      `yaml:"statement,omitempty"`
	Owner          string      `yaml:"owner,omitempty"`
	Due            QuestionDue `yaml:"due,omitempty"`
	Blocks         []string    `yaml:"blocks,omitempty"`
	Status         string      `yaml:"status,omitempty"`
	Answer         *string     `yaml:"answer,omitempty"`
	AnswerRevision RevisionRef `yaml:"answer_revision,omitempty"`
	CancelReason   string      `yaml:"cancel_reason,omitempty"`
	Source         string      `yaml:"source,omitempty"`
}

func (q *Question) NodeID() string     { return q.ID }
func (q *Question) NodeKey() *string   { return q.Key }
func (q *Question) Noun() string       { return NounQuestion }
func (q *Question) NodeStatus() string { return q.Status }

// QuestionDue is when a question is due, by date or as a hint
// (artifact-schema.md, Decisions and open questions).
type QuestionDue struct {
	Date string `yaml:"date,omitempty"`
	Hint string `yaml:"hint,omitempty"`
}

// QuestionsFile is the open-questions.yml ledger.
type QuestionsFile struct {
	Questions []*Question `yaml:"questions,omitempty"`
}
