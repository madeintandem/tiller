// Typed client for the tiller serve read API. The types mirror the Go structs
// field for field: internal/views (summary, streams, node, nodepage, list,
// queue, recap), internal/gen (tree), internal/search, and the serve-facing
// wrappers in internal/serve (the list rows that lift ref into the JSON, the
// event feed). The Go json tags ARE the contract: a field tagged omitempty may
// be absent and is optional here; a pointer field without omitempty arrives as
// null and is typed nullable; a slice the Go side always allocates is typed as
// a plain array. Machine payloads carry id and key structurally plus the
// precomputed ref string, and the UI renders ref as the human reference.

// NodeStatus names the seven lifecycle/display states the chart-table
// primitives know how to paint (StateDot, StateBar, Chip; see stateColor in
// lib/states.ts): draft, specced (the spec is inked and awaiting approval;
// the Go lifecycle emits this value), ready, active, in_review, done, and
// blocked. The store's full status vocabulary is wider (cancelled, archived,
// proposed, open, answered, in_progress, check, withdrawn, approved,
// abandoned among them), so the status fields below stay typed as plain
// string, which is the vocabulary they actually carry. NodeStatus is
// additive: it exists for callers that specifically compose the seven-state
// chart legend (StateBar mixes). A page holding a wire status paints it
// through lib/states.ts's total statusColor(string) and never casts to
// NodeStatus.
export type NodeStatus =
  | "draft"
  | "specced"
  | "ready"
  | "active"
  | "in_review"
  | "done"
  | "blocked";

export interface ProjectInfo {
  name: string;
  key: string;
  kind: string;
}

export interface ContributorInfo {
  id: string;
  name: string;
}

export interface TreeCounts {
  epics: number;
  features: number;
  tasks: number;
  questions: number;
}

// The project-wide tally of task A2's NextOwner projection (views.
// WhoseMoveCounts): how many non-terminal nodes currently wait on a
// human-only gate and how many the agent can act on right now. Carries no
// omitempty on the Go side, so it is always present, zeroes included.
export interface WhoseMoveCounts {
  human: number;
  agent: number;
}

// GET /api/status (views.ProjectSummary).
export interface ProjectSummary {
  project: ProjectInfo;
  contributors: ContributorInfo[];
  counts: TreeCounts;
  active_epics?: string[];
  whose_move: WhoseMoveCounts;
  // order is the recorded root soft order (store.ProjectOrder.Epics,
  // .tiller/order.yml): carriage only, the epic sequence `tiller order
  // epic` last wrote, verbatim and possibly partial. Absent until a root
  // order is first recorded. order_refs stays off the wire, the same
  // CLI-only posture active_epic_refs already keeps (views.ProjectSummary's
  // own doc comment).
  order?: string[];
}

export interface ExecutionRecord {
  id: string;
  key: string;
  branch: string;
  source: "working_tree" | "local_branch" | "remote_tracking";
  revision: string;
  observed_at: string;
  uncommitted: boolean;
  freshness?: "local_branch_behind" | "remote_tracking_behind";
  status: string;
  owner: string;
  integration_status: string;
}

export interface ExecutionDiagnostic {
  id?: string;
  branch?: string;
  code: string;
  message: string;
}

export interface ExecutionSnapshot {
  observations: ExecutionRecord[];
  diagnostics: ExecutionDiagnostic[];
}

export const executionPath = "/api/execution";

export function fetchExecution(): Promise<ExecutionSnapshot> {
  return fetchJSON<ExecutionSnapshot>(executionPath);
}

// One recorded gate result (views.GateView).
export interface GateView {
  required: boolean;
  kind?: string;
  status?: string;
  run_by?: string;
  run_at?: string;
  against?: string;
  evidence?: EvidenceView[];
  criteria?: Record<string, CriterionStatusView>;
}

export interface CriterionStatusView {
  status?: string;
  evidence?: EvidenceView[];
}

export interface EvidenceView {
  kind?: string;
  ref?: string;
  url?: string;
  unavailable?: string;
}

// One unfinished assignment on the current-work lens (views.StreamItem).
export interface StreamItem {
  noun: string;
  id: string;
  key?: string;
  ref: string;
  status: string;
  display: string;
}

// One question handle on a blocked or unclaimed row. The matched canonical
// target identifies where the structural hold was found; full statements stay
// on question detail surfaces.
export interface QuestionHoldItem {
  question: string;
  title: string;
  target_id: string;
  target_noun: string;
}

// One condition row on the blocked lens. Manual dependencies, a retired
// ancestor, and operative questions are separate rows for the same node.
export interface BlockedItem {
  noun: string;
  id: string;
  key?: string;
  ref: string;
  depends_on?: string[];
  depends_on_refs?: string[];
  held?: HeldView;
  question_holds?: QuestionHoldItem[];
}

// One in-review node with its recorded gate results (views.GatesItem).
export interface GatesItem {
  noun: string;
  id: string;
  key?: string;
  ref: string;
  gates?: Record<string, GateView>;
}

// One open question with its bounded scan title, complete statement, and due
// hint (views.QuestionItem).
export interface QuestionItem {
  id: string;
  title?: string;
  statement?: string;
  due?: string;
  blocks?: string[];
}

// One registered contributor's four-part rollup (views.ContributorStream).
export interface ContributorStream {
  contributor: ContributorInfo;
  active: StreamItem[];
  blocked: BlockedItem[];
  gates_in_review: GatesItem[];
  open_questions: QuestionItem[];
}

// One eligible, unclaimed ready task and its feature context (views.UnclaimedItem).
export interface UnclaimedItem {
  noun: string;
  id: string;
  key?: string;
  title?: string;
  ref: string;
  status: string;
  feature_id: string;
  feature_key?: string;
}

// GET /api/board (views.Streams). unclaimed is additive and omitted from the
// wire entirely when nothing is claimable (Go's own omitempty), so it reads
// as optional here too.
export interface Streams {
  contributors: ContributorStream[];
  unclaimed?: UnclaimedItem[];
}

// The topbar's asof affordance (console v2 phase 2, JC's ruling: the
// freshness line lives in the topbar, not a reserved slot in Home's header
// band - ui-surface.md's amended header-band sentence) tracks the last
// successful completion of ANY read through fetchJSON below, which is every
// read's one choke point (useFetch and every imperative fetchXxx helper here
// funnel through it). markFetchSucceeded fires only on the success path, so
// a failed read never advances the freshness line - "synced" is a claim about
// data actually in hand, never about an attempt.
export type LastFetchListener = (at: Date) => void;
let lastFetchAt: Date | null = null;
const lastFetchListeners = new Set<LastFetchListener>();

export function getLastFetchAt(): Date | null {
  return lastFetchAt;
}

// subscribeLastFetch returns its own unsubscribe, the same disposable-
// listener shape every other subscription in this codebase follows (useFetch's
// own cleanup returns an inline function rather than a named "unsubscribe").
export function subscribeLastFetch(listener: LastFetchListener): () => void {
  lastFetchListeners.add(listener);
  return () => {
    lastFetchListeners.delete(listener);
  };
}

function markFetchSucceeded(): void {
  lastFetchAt = new Date();
  for (const listener of lastFetchListeners) {
    listener(lastFetchAt);
  }
}

// The topbar's manual refresh (round-1 review, Finding 4: the console has no
// polling, so without this the asof line was a staleness signal with no
// remedy - the mock draws the control, and JC's ruling is that it ships).
// One counter, bumped by the topbar's click handler; hooks.ts's useFetch
// subscribes and adds the current value to its own fetch effect's
// dependency array alongside path, so a bump re-issues every CURRENTLY
// MOUNTED request in place, through the exact same effect a path change
// already runs - one mechanism, not a second refetch path invented beside
// it. Plain module state, not React state: this file has no framework
// dependency, and the subscription below is what turns a bump into a
// re-render for whichever components are actually mounted.
export type FetchGenerationListener = () => void;
let fetchGeneration = 0;
const fetchGenerationListeners = new Set<FetchGenerationListener>();

export function getFetchGeneration(): number {
  return fetchGeneration;
}

export function subscribeFetchGeneration(listener: FetchGenerationListener): () => void {
  fetchGenerationListeners.add(listener);
  return () => {
    fetchGenerationListeners.delete(listener);
  };
}

export function bumpFetchGeneration(): void {
  fetchGeneration += 1;
  for (const listener of fetchGenerationListeners) {
    listener();
  }
}

// init carries a read's optional headers and the abort signal useFetch uses
// to give a superseded request's connection back to the browser.
export async function fetchJSON<T>(path: string, init?: Pick<RequestInit, "headers" | "signal">): Promise<T> {
  const res = await (init === undefined ? fetch(path) : fetch(path, init));
  if (!res.ok) {
    let detail = "";
    try {
      detail = await res.text();
      // The API wraps refusals as {"error": msg}; surface the message itself
      // rather than the JSON envelope. A non-JSON body reads as-is.
      const parsed: unknown = JSON.parse(detail);
      if (
        typeof parsed === "object" &&
        parsed !== null &&
        "error" in parsed &&
        typeof (parsed as { error: unknown }).error === "string"
      ) {
        detail = (parsed as { error: string }).error;
      }
    } catch {
      // The status line is enough.
    }
    const suffix = detail.trim() === "" ? "" : `: ${detail.trim()}`;
    throw new Error(`GET ${path} failed (${res.status} ${res.statusText})${suffix}`);
  }
  const data = (await res.json()) as T;
  markFetchSucceeded();
  return data;
}

// One recorded human sign-off (views.AttestationView).
export interface AttestationView {
  id: string;
  target_kind?: string;
  target_id?: string;
  status?: string;
  by?: string;
  at?: string;
  evidence_ref?: string;
}

// One merge-time policy violation (views.PolicyViolationView).
export interface PolicyViolationView {
  gate?: string;
  observed?: string;
  status?: string;
  evidence_ref?: string;
  source_revision?: string;
  unresolved_criteria?: string[];
  evidence_refs?: string[];
}

// The spec/eval revision pair a feature was accepted against (views.BasisView).
export interface BasisView {
  spec?: string;
  eval?: string;
  // HEAD at approval (views.BasisView.Commit), the one revision here a
  // hosting page can show; spec and eval are blob fingerprints. commit_url
  // is serve-filled for a GitHub origin only, like pr_url.
  commit?: string;
  commit_url?: string;
}

// One verified surface fingerprint (views.SurfaceView).
export interface SurfaceView {
  path?: string;
  kind?: string;
  value?: string;
}

// The completion record set at done (views.VerifiedAgainstView). Surfaces is
// always present, the empty array for a create feature that touched no surface.
export interface VerifiedAgainstView {
  spec?: string;
  eval?: string;
  // HEAD at the done crossing; see BasisView.commit.
  commit?: string;
  commit_url?: string;
  surfaces: SurfaceView[];
}

// The existing code a node touches (views.SurfaceInfoView): empty for a
// greenfield create, populated for a brownfield modify/replace/migrate.
export interface SurfaceInfo {
  paths?: string[];
}

// A recorded branch and submission-time head (views.ReviewRefView). This is
// either an offline review target or a task riding a coarse review, including
// with a remote. It does not establish that the coarse PR exists or is published.
export interface ReviewRefView {
  branch?: string;
  head?: string;
}

export interface RecoveredReviewView {
  pr?: number;
  branch: string;
  head: string;
  merge_commit: string;
  integration_branch: string;
  basis: string;
  recovered_by: string;
  recovered_at: string;
  reason: string;
  tasks: { id: string; path: string; state: string }[];
  policy_snapshots: {
    node_id?: string;
    path: string;
    source: string;
    observations: {
      gate: string;
      status: string;
      unresolved_criteria?: string[];
      evidence_refs?: string[];
    }[];
  }[];
}

// The full-node payload of show and the node page (views.NodeView): one stable
// shape across every noun, fields that do not apply omitted. A question's
// answer is a Go *string under omitempty, so it is absent until answered and a
// plain string after.
export interface NodeView {
  noun: string;
  id: string;
  key?: string;
  title?: string;
  status: string;
  // activity is the node's derived phase: a feature's rollup phase
  // (awaiting_check / awaiting_sync) or an epic's active/idle working status
  // (views.activity, matching the status oracle); absent for a task, a
  // question, and for a feature or epic with no phase.
  activity?: string;
  blocked?: boolean;
  owner?: string;
  owner_name?: string;
  // Present on epics/features only, including [] before anyone starts work.
  contributors?: ContributorInfo[];
  started?: boolean;
  supersedes?: string[];
  superseded_by?: string;
  change_type?: string;
  epic?: string;
  feature?: string;
  intent?: string;
  statement?: string;
  answer?: string;
  // Set only on a cancelled question.
  cancel_reason?: string;
  attached_to?: string;
  resolved_by?: string;
  resolution?: string;
  duplicate_of?: string;
  branch?: string;
  pr?: number;
  // review_grain is the review grain in force for work under this node
  // (views.NodeView.ReviewGrain): "task", "feature", or "epic", present for
  // an epic and a feature, the two review-grain homes besides the project
  // default (docs/spec/ui-surface.md).
  // review_grain_source names which of the three homes it actually came
  // from: "feature", "epic", "project" (the manifest's own default), or
  // "default" when none of the three is set and the value is the hardcoded
  // task fallback. Absent for every other noun.
  review_grain?: string;
  review_grain_source?: string;
  depends_on?: string[];
  gates?: Record<string, GateView>;
  attestations?: AttestationView[];
  policy_violations?: PolicyViolationView[];
  accepted_against?: BasisView;
  verified_against?: VerifiedAgainstView;
  source?: string;
  surface?: SurfaceInfo;
  review_ref?: ReviewRefView;
  recovered_review?: RecoveredReviewView;
  review_missing?: boolean;
  pr_url?: string;
  // The node header pass's three wire additions (2026-08-30; views.NodeView):
  // what the done crossing rested on, who captured a task or a shelf node,
  // and the branch's page on a GitHub origin (serve-filled, like pr_url).
  done_basis?: string;
  proposed_by?: string;
  branch_url?: string;
  // NextOwner is whose move this node is next, "human" or "agent", empty at a
  // terminal state with no legal next verb (views.NextOwner, task A2). A
  // node in review always reports "agent" here (resume/sync are the legal
  // agent moves), even though the human's own PR merge is what it is really
  // waiting on: NextOwner sees Tiller verbs only, and merge is not one
  // (views.NextOwner's own doc comment). A caller naming "what's left" must
  // enumerate in-review nodes from their plain state, never take this field
  // as "nobody needs to look" for them.
  next_owner?: string;
  // held names the nearest retired ancestor this node sits under (a
  // cancelled feature, or a cancelled-or-archived epic), absent when none
  // (views.NodeView.Held, gen.RetiredAncestor). It is additive, orthogonal
  // to status and next_owner above, which stay pure lifecycle projections
  // either way (the "annotate, don't subtract" decision, issue #36): the
  // doing verbs already refuse under a retired ancestor
  // (mutate.guardParentState); this field says why, before anyone runs one.
  held?: HeldView;
  // order is the recorded soft order of this node's own children: an
  // epic's feature sequence or a feature's task sequence (store.Epic.Order,
  // store.Feature.Order). Carriage only, verbatim and possibly partial,
  // never the full computed sibling order relationships.features/tasks
  // already carries. Absent for a task, a question, an amendment, and for
  // an epic or feature that never ran `tiller order`. order_refs carries
  // the same targets as human references, index-aligned to order and in
  // its recorded sequence (views.NodeView.OrderRefs).
  order?: string[];
  order_refs?: string[];
}

// The nearest retired ancestor a node sits under (views.HeldView): named the
// same noun-id-ref way RelatedNode names an adjacent node, plus that
// ancestor's own retired state ("cancelled" or "archived").
export interface HeldView {
  noun: string;
  id: string;
  ref: string;
  state: string;
}

// A node's prose file (views.ProseRef): the repo-relative path and its content.
// An epic's epic.md, a feature's feature.md, a task's task.md; only a question,
// whose prose handle rides on the node itself, carries none. Content is empty
// when the file is missing or unreadable; the path stays an honest pointer
// either way. stub says the content is still exactly what `tiller add`
// generated and nobody has written over it, answered server-side by
// mutate.IsProseStub (the predicate `tiller spec feature` itself refuses on),
// so the console never has to match the boilerplate text here. Omitted on the
// wire when false.
export interface ProseRef {
  path: string;
  content: string;
  stub?: boolean;
}

// The safe Markdown reader includes baseline facts only for a registered
// design doc. Drift detail is a sibling of that existing ledger projection.
export interface ProseFile extends ProseRef {
  line: number;
  line_text: string;
  baseline?: BaselineDocView;
  drift_detail?: NodeDriftView;
}

// A registered as-is document is path-addressed, never a work node.
export interface BaselineDocView {
  path: string;
  describes: string[];
  derived_from?: RevisionRefView[];
  confirmed?: BaselineConfirmationView;
}

export interface BaselineConfirmationView {
  commit: RevisionRefView;
  surfaces: BaselineSurfaceView[];
  by: string;
}

export interface BaselineSurfaceView extends RevisionRefView {
  inventory: string[];
}

// One adjacent node on the relationship hub (views.RelatedNode). Questions
// carry a bounded title for the row and may also carry their complete
// statement for detail consumers; row renderers never promote statement into
// a missing title. A dangling edge degrades to a bare-id row: noun and status
// arrive as empty strings and ref repeats the id, so guard before linking.
export interface RelatedNode {
  noun: string;
  id: string;
  key?: string;
  ref: string;
  status: string;
  title?: string;
  statement?: string;
}

export interface AmendmentEffect {
  operation_index?: number;
  operation: string;
  role: "target" | "replacement" | "created" | "addition_parent" | "question_block" | "move_source_parent" | "move_destination_parent";
  scope: "direct" | "descendant" | "historical_parent";
  node?: RelatedNode;
  reference?: string;
  note?: string;
}

export interface AmendmentRelation {
  amendment: RelatedNode;
  effects: AmendmentEffect[];
  proposed_at?: string;
  approved_at?: string;
  withdrawn_at?: string;
}

// The relationship hub (views.Relationships). Every field is omitted when it
// does not apply to the noun, and a present list is never empty.
export interface Relationships {
  epic?: RelatedNode;
  feature?: RelatedNode;
  attached_to?: RelatedNode;
  resolved_by?: RelatedNode;
  duplicate_of?: RelatedNode;
  attached_work?: RelatedNode[];
  features?: RelatedNode[];
  tasks?: RelatedNode[];
  depends_on?: RelatedNode[];
  depended_on_by?: RelatedNode[];
  supersedes?: RelatedNode[];
  superseded_by?: RelatedNode[];
  blocked_by_questions?: RelatedNode[];
  blocks?: RelatedNode[];
  amendments?: AmendmentRelation[];
  amendment_history_warning?: string;
}

// One event on a node's timeline (views.HistoryEntry), newest first.
export interface HistoryEntry {
  at: string;
  type: string;
  contributor: string;
  command: string;
}

// One node's active-basis drift report (views.ActiveBasisView): the pinned
// files whose current working-tree content no longer matches what the human
// approved, which node's pin moved, and the move that clears it.
//
// The node named here is not always the page's own. A task's guard compares its
// parent feature's spec and contract before its own task.md, so a task page can
// carry a report whose noun and ref are the FEATURE's, which is the grain the
// command addresses; a renderer that drops that distinction hands the reader a
// feature's command with nothing saying why. missing names pins that are gone
// rather than changed, which have to be restored before command can succeed.
export interface ActiveBasisView {
  drifted: string[];
  missing?: string[];
  noun: string;
  id: string;
  ref: string;
  command: string;
}

// One marker's presence on a node's Drifted chip (views.DriftBadgeView): the
// marker name and whether it currently refuses a verb, which is what decides
// the chip's palette register (ui-surface.md, the node-page chip sentence:
// "takes the blocked palette register only where the condition actually
// refuses verbs ... and the neutral register the stale gates use
// otherwise"). The rule behind refusing is computed server-side, in
// internal/views/nodepage.go's driftRefusing, so the console reads the bit
// rather than re-deriving it. An uncovered node (a marker's sweep could not
// compare it) never becomes a badge, so there is no reason field here the
// way a drift report's UncoveredView carries one.
export interface DriftBadgeView {
  marker: string;
  refusing: boolean;
}

// One staged operation's own precondition, structural half (views.
// AmendmentPreconditionPageView): the name and the staged expected value,
// always present. live is the current-tree read, filled by the serve
// handler the same way active_basis and drift are (nil until it runs, never
// computed by the console itself).
export interface AmendmentPreconditionPageView {
  name: string;
  expected: string;
  live?: AmendmentPreconditionLiveView;
}

// A staged precondition's LIVE read against the current tree (views.
// AmendmentPreconditionLiveView), mutate.PreconditionValidity projected for
// the console: status is one of "valid", "stale", or "unknown" (an
// add-feature/add-task operation's own staged key, which has no live node to
// compare against until approve materializes it). actual and note are both
// optional and never both meaningful at once in practice, mirroring the CLI's
// own show amendment render.
export interface AmendmentPreconditionLiveView {
  status?: string;
  actual?: string;
  note?: string;
}

// One staged operation on an amendment's page (views.
// AmendmentOperationPageView), the identical shape the CLI's own show
// amendment renders reprojected for the console: target, by, and
// superseded_by resolve to a RelatedNode where the live tree can (an
// existing feature or task in full, a staged key naming an add op on this
// same amendment degrading to a bare-id row, RelatedNode's own dangling-edge
// convention), so a renderer just hands each one to RefLink. An add-question
// carries both its bounded title and complete statement: the title is the
// operation heading and statement remains subordinate detail. source cites
// the feedback this operation traces to, legal on every op kind. blocks is
// legal only on add-question, resolved through the identical RelatedNode
// convention as target/by/superseded_by, the same rule the question NODE
// page's own blocks_refs already uses (below). source_missing and
// shape_error are this operation's own #57 pre-flight, computed server-side
// and always present (never gated on whether the batch resolves): true only
// when this op's own citation is blank on an intake-minted amendment, and
// the reason token (mutate.ReasonDuplicateStagedKey "duplicate_staged_key"
// or mutate.ReasonStagedEpicReplacement "staged_epic_replacement") when
// resolveAmendment would refuse on this op's own shape, blank otherwise.
export interface AmendmentOperationPageView {
  index: number;
  op: string;
  target?: RelatedNode;
  by?: RelatedNode;
  key?: string;
  title?: string;
  statement?: string;
  change_type?: string;
  reason?: string;
  superseded_by?: RelatedNode;
  blocks?: RelatedNode[];
  // A correct-eval's deliberately removed criterion ids.
  retire?: string[];
  source?: string;
  source_missing?: boolean;
  shape_error?: string;
  preconditions?: AmendmentPreconditionPageView[];
  created?: RelatedNode;
  parent?: RelatedNode;
  identity_notes?: string[];
  move?: FeatureMovePageView;
}

export interface FeatureMovePageView {
  source_key: string;
  review_grain: string;
  materialize_review_grain?: boolean;
  order_effect: string;
  reference_warnings?: string[];
}

// An intake-minted amendment's own provenance artifact (views.
// AmendmentSourcePageView), additive on the amendment page (task-5-brief.md):
// path and revision are the structural fingerprint intake recorded; live is
// the current-tree read, filled by the serve handler the same way a
// precondition's own live is (nil until it runs). Absent whole for a
// hand-opened amendment, the same convention AmendmentPageView.source itself
// keeps.
export interface AmendmentSourcePageView {
  path: string;
  revision: string;
  live?: AmendmentPreconditionLiveView;
}

// A proposed reshape's own node-page content (views.AmendmentPageView),
// filling NodePage.amendment for an amendment page only: every staged
// operation in staged order, the two design-doc lists that name why the
// amendment exists (triggered_by) and what its approval keeps current
// (amends_design), and source, the amendment-level fingerprinted feedback
// artifact an intake-minted amendment carries. Absent for a hand-opened
// amendment. validity_error names the reason(s) live validity could not be
// computed against the current tree (#57), the identical vocabulary the
// CLI's own `show amendment --json` envelope already carries: present when
// EITHER of the server's two fallible live reads fails — the batch cannot
// even resolve (a vanished target, a hand-edited epic key), OR the
// amendment-level source artifact itself cannot be read (permission denied,
// a symlink loop) — joined into one string, deduped, when both fail on the
// same underlying cause.
//
// The two reads are independent, and so is what goes absent: a
// precondition's own `live` slot is absent only when the PRECONDITIONS read
// itself failed, and the amendment-level source's own `live` slot is absent
// only when the SOURCE read itself failed. validity_error present is not a
// claim that every `live` slot on the page is absent — it names only that
// at least one of the two reads failed; the other read's `live` slots fill
// normally beside it (internal/serve's own
// TestNodePageAmendmentSourceErrorAloneStillFillsPreconditionLive pins the
// case that makes this concrete: validity_error set from the source read
// alone, every precondition's `live` still filled).
export interface AmendmentPageView {
  operations: AmendmentOperationPageView[];
  triggered_by?: string[];
  amends_design?: string[];
  source?: AmendmentSourcePageView;
  validity_error?: string;
}

// GET /api/node/{noun}/{addr} (views.NodePage). Prose is null only for a
// question, whose prose handle rides on the node itself; every folder-backed
// noun, task included, carries its own file. History is always an array, empty
// rather than null. Eval is the node's authored acceptance contract: null, a
// well-formed EvalContractView, or an EvalContractError naming why the file
// failed to load; narrow it with isEvalError. Decomposition is filled for an
// epic only: every child feature projected through the full NodeView, so an
// epic's approval review has each feature's source, change type, surface, and
// gates without a second fetch. active_basis is absent unless a pin drifted.
// drift is absent unless drift.ForNode found something for this node: one
// badge per distinct marker, in drift.KnownMarkers' fixed schema order.
// amendment is filled for an amendment page only, absent for every other noun.
export interface WorkflowWork {
  kind: string;
  id: string;
  status: string;
  actor: string;
  node_id?: string;
}

export interface CurrentAcceptanceView {
  node_id: string;
  contract_status: string;
  acceptance_ready: boolean;
  outstanding?: WorkflowWork[];
}

export interface WorkflowAdvice {
  phase: string;
  actor: string;
  action: string;
  command?: string;
  reason: string;
  review_id: string;
  feature_id?: string;
  contract_status: string;
  acceptance_ready: boolean;
  submission_recorded: boolean;
  outstanding?: WorkflowWork[];
  historical_violations?: number;
  crossing_violations?: number;
  recovered_review?: RecoveredReviewView;
  review_missing?: boolean;
  review_guidance?: string;
  recovered_crossings_pending?: boolean;
  current_acceptance?: CurrentAcceptanceView[];
}

export interface NodePage {
  node: NodeView;
  ref: string;
  prose: ProseRef | null;
  relationships: Relationships;
  backlinks: BacklinksView;
  history: HistoryEntry[];
  eval: EvalPayload;
  decomposition?: { features: NodeView[] };
  amendment?: AmendmentPageView;
  workflow?: WorkflowAdvice;
  execution?: ExecutionRecord;
  execution_history_error?: string;
  active_basis?: ActiveBasisView;
  drift?: DriftBadgeView[];
  // The panel behind the chip: the same ForNode call's findings at candidate
  // grain (views.NodePage.DriftDetail). Fills and degrades together with
  // drift above, so the chip and its panel cannot disagree.
  drift_detail?: NodeDriftView;
  // unpushed_review names the review-state warning (issue #42, Task 15,
  // internal/views/nodepage.go's UnpushedReview) for a task in review whose
  // recorded feature branch holds a commit its PR does not yet show: the
  // node page's own warning-register attention box (task-17-brief.md item
  // 5). Absent for every noun but a task, and for a task that is not in
  // review, records no PR, has no recorded feature branch, or whose branch
  // already sits pushed even with its upstream.
  unpushed_review?: UnpushedReviewView;
}

// The additive unpushed_review object (views.UnpushedReviewView): the short
// sha sitting on the recorded feature branch's local tip, the branch itself,
// and its remote-tracking counterpart (an @{upstream} abbrev-ref, always
// "<remote>/<branch>" shaped - never a bare branch name), which is what lets
// a renderer name the actual remote a push command should target rather than
// assuming "origin".
export interface UnpushedReviewView {
  commit: string;
  branch: string;
  upstream: string;
}

// A stored revision reference (views.RevisionRefView): what kind and value
// address (a git blob, commit, tree, or content hash), the path the ref was
// taken against, and observed_at rendered RFC3339 in UTC.
export interface RevisionRefView {
  kind: string;
  value: string;
  path?: string;
  observed_at?: string;
}

// Connects a criterion to a gate (views.EvalBindingView): a test file or id, a
// command to run, a bare manual human verification, or an AI-graded rubric.
// Ref and run are omitted together for a manual binding, which needs neither.
// A test binding that names two or more files sets refs and leaves ref empty.
export interface EvalBindingView {
  type: string;
  ref?: string;
  refs?: string[];
  run?: string;
}

// A required preserve criterion's pre-change baseline (views.EvalBaselineView):
// the source surface, the revision it was captured at, and the artifact that
// embodies the old behavior.
export interface EvalBaselineView {
  surface?: string;
  captured_at?: RevisionRefView;
  artifact?: string;
}

// One acceptance criterion in the authored contract (views.EvalCriterionView):
// its polarity, whether it is required, the gate it binds to, and its
// pre-change baseline when it carries one (a required preserve criterion
// always does; every other criterion carries none).
export interface EvalCriterionView {
  id: string;
  statement: string;
  polarity: string;
  required: boolean;
  binding: EvalBindingView;
  baseline?: EvalBaselineView;
}

// A node's authored acceptance contract (views.EvalContractView): the
// repo-relative path it was read from, the node it grounds, and its criteria
// in file order.
export interface EvalContractView {
  path: string;
  node: string;
  criteria: EvalCriterionView[];
}

// Reports that a node's eval file failed to load, parse, or validate as a
// well-formed acceptance contract (views.EvalContractErrorView): the path it
// was read from and the error naming why, standing in for the criteria a
// well-formed contract would carry.
export interface EvalContractError {
  path: string;
  error: string;
}

// NodePage's eval field: null when the node carries none, a well-formed
// contract, or a contract that failed to load.
export type EvalPayload = EvalContractView | EvalContractError | null;

// Narrows EvalPayload to the failure case. Anything else, null or a
// well-formed EvalContractView, carries no error field and is not one.
export function isEvalError(e: EvalPayload): e is EvalContractError {
  return e !== null && "error" in e;
}

// One list row (serve's listNode: views.ListItem plus the ref the CLI keeps
// out of its JSON). activity is the row's derived phase: a feature's rollup
// phase (awaiting_check / awaiting_sync) or an epic's active/idle working
// status (views.activity, matching the status oracle); absent for a task, a
// question, and for a feature or epic with no phase.
export interface ListRow {
  id: string;
  key?: string;
  title?: string;
  intent?: string;
  statement?: string;
  status: string;
  recovered_review?: RecoveredReviewView;
  review_missing?: boolean;
  gates?: Record<string, GateView>;
  current_acceptance?: CurrentAcceptanceView;
  participating_acceptance?: CurrentAcceptanceView[];
  recovered_crossings_pending?: boolean;
  historical_violations?: number;
  crossing_violations?: number;
  activity?: string;
  owner?: string;
  owner_name?: string;
  contributors?: ContributorInfo[];
  ref: string;
  // A feature row's Build board column (serve/list.go's listNode): status,
  // except a ready feature with a task in flight reads "active". Absent for
  // every other noun.
  column?: string;
  // The three fields a question row alone carries (internal/views/list.go's
  // ListItem, task-15): answer is nil until answered, so its absence IS the
  // open/answered signal rather than a second status check; due is
  // gen.QuestionDue's date-or-hint answer, the same string the queue and
  // streams rows already carry; blocks_refs resolves the question's Blocks
  // edges the same way every other edge on the node page resolves one - a
  // dangling target degrades to a bare-id RelatedNode (empty noun, ref
  // repeating the id) rather than disappearing. Empty for every other noun.
  answer?: string;
  // A cancelled question's recorded reason; absent otherwise.
  cancel_reason?: string;
  due?: string;
  blocks_refs?: RelatedNode[];
}

// GET /api/ls (serve's listPayload, the CLI's ls envelope with refs on the
// rows). Noun echoes back normalized to the singular; nodes is always an array.
export interface ListResult {
  noun: string;
  count: number;
  nodes: ListRow[];
  facets?: Facet[];
  axes?: Record<string, FilterAxis[]>;
  unresolved?: Partial<Record<FilterAxis, string>>;
}

// The optional narrowing axes of /api/ls, each the CLI filter of the same
// name. An absent or empty field is "no filter on this axis".
export interface ListFilters {
  status?: string;
  owner?: string;
  contributor?: string;
  feature?: string;
  epic?: string;
}

// Explore's server-owned axes, spelled as /api/ls query parameters.
export type FilterAxis = "status" | "owner" | "contributor" | "feature" | "epic";

// Labels are display only; picks carry status words, person IDs or node IDs.
export interface FacetOption {
  value: string;
  label: string;
  key?: string;
  parent?: string;
  count: number;
  selected?: boolean;
  you?: boolean;
  cancelled?: boolean;
}

export interface Facet {
  axis: FilterAxis;
  options: FacetOption[];
}

// One board card (views.BuildFeature, GET /api/build): a feature row in
// ListItem's own shape (id/key/title/status/activity/contributors) plus the board's
// own additions - ref rendered explicitly (the shadow-the-embedded-render-
// only-Ref idiom serve/list.go's listNode already uses), the task rollup
// (task_done/task_total, both excluding a cancelled task, matching the
// "awaiting check" phase Home already renders for the same feature),
// open_work_items (all nonterminal bugs, chores, and tweaks attached directly or
// through one of the feature's tasks), the task list itself in the shared
// task order (always an array, never null, even for a feature with no tasks
// yet), and the board's two distinct blocked-register conditions (issue #39:
// never folded into one flag) -
// blocked, the manual flag (store.Feature.Blocked, read through
// views.Blocked; false omits the key rather than emitting "blocked": false,
// commit 9b4c778), and held, the nearest retired ancestor this feature sits
// under (absent for a live feature). A card can carry both at once: a
// manually blocked feature under a retired ancestor is not a contradiction,
// it is two independent facts. This is narrower than ListRow on purpose: a
// card is always a feature, never a question, so it carries no
// statement/intent field a feature can never have.
export interface BuildFeature {
  id: string;
  key?: string;
  title?: string;
  status: string;
  // The board column the card lands in: status, except a ready feature with
  // a task in flight reads "active" (gen.FeatureBoardStatus).
  column: string;
  activity?: string;
  contributors: ContributorInfo[];
  ref: string;
  task_done: number;
  task_total: number;
  open_work_items: number;
  tasks: RelatedNode[];
  blocked?: boolean;
  held?: HeldView;
}

// One epic panel row (views.BuildEpic): enough identity to link and select
// it, its own features' column composition for the panel's count badge and
// mix bar (feature_counts, keyed by BuildFeature.column - draft/specced/
// ready/active/in_review/done, the board's own six columns - absent, never an empty object, for an epic with no features),
// and its own features in the shared feature order.
export interface BuildEpic {
  noun: string;
  id: string;
  key?: string;
  title?: string;
  ref: string;
  status: string;
  contributors: ContributorInfo[];
  feature_counts?: Record<string, number>;
  features: BuildFeature[];
}

// GET /api/build (views.BuildBoard): non-cancelled epics in shared dependency
// order, panel-and-columns ready. Epic selection rides the URL. A column is
// a status filter over this served sequence, never a re-sort.
export interface BuildBoard {
  epics: BuildEpic[];
}

export const buildPath = "/api/build";

export function fetchBuild(): Promise<BuildBoard> {
  return fetchJSON<BuildBoard>(buildPath);
}

export interface WorkItemCard extends ListRow {
  noun: "bug" | "chore" | "tweak";
  ref: string;
  attached_to?: string;
  attached?: RelatedNode;
  attachment_ids?: string[];
  closed_at?: string;
  resolution?: string;
  supersedes?: string[];
  superseded_by?: string;
}

export interface WorkItemsBoard {
  open: WorkItemCard[];
  closed: WorkItemCard[];
  attachments: RelatedNode[];
}

export const workItemsPath = "/api/work-items";

export function fetchWorkItems(): Promise<WorkItemsBoard> {
  return fetchJSON<WorkItemsBoard>(workItemsPath);
}

// One node in the work tree (gen.TreeNode). Activity is derived for
// epics only; files and children are always arrays. The tree payload carries
// no precomputed ref, so a renderer composes the reference from key and id.
export interface TreeNode {
  noun: string;
  id: string;
  key?: string;
  title?: string;
  status: string;
  owner?: string;
  owner_name?: string;
  contributors?: ContributorInfo[];
  activity?: string;
  path: string;
  files: string[];
  children: TreeNode[];
}

// Shared relationship index (internal/relationships). Only an explicit
// destination grants navigation; unresolved IDs and recorded coordinates do
// not acquire a node route, a lifecycle status, or a repository file link.
export interface RelationshipDestination {
  kind: "node" | "file";
  noun?: string;
  id?: string;
  path?: string;
  line?: number;
}

export interface RelationshipVertex {
  id: string;
  kind: string;
  key?: string;
  title?: string;
  status?: string;
  owner?: string;
  contributors?: ContributorInfo[];
  path?: string;
  raw?: string;
  document_owner?: string;
  generated?: boolean;
  unresolved?: boolean;
  destination?: RelationshipDestination;
}

export interface RelationshipSource {
  file: string;
  field?: string;
  line?: number;
  citation_path?: string;
  citation_line?: number;
  criterion?: string;
  basis?: string;
  binding_type?: string;
  classification?: string;
  raw?: string;
  revision?: RevisionRefView;
  operation?: number;
  role?: string;
  state?: string;
  scope?: string;
  evidence?: string[];
}

export interface RelationshipEdge {
  id: string;
  kind: string;
  from: string;
  to: string;
  sources: RelationshipSource[];
  derived?: boolean;
  declared_scope?: string;
}

export interface RelationshipDiagnostic {
  code: string;
  message: string;
  facet: string;
  source?: RelationshipSource;
  subjects?: string[];
  incomplete?: boolean;
}

export interface RelationshipKind {
  name: string;
  inverse: string;
  from_kinds: string[];
  to_kinds: string[];
}

export interface RelationshipGraph {
  vertices: RelationshipVertex[];
  edges: RelationshipEdge[];
  catalog: RelationshipKind[];
  diagnostics: RelationshipDiagnostic[];
  complete: boolean;
}

export interface RelationshipQueryResult {
  starts: string[];
  follow: string[];
  depth: number;
  where: { field: string; value: string }[];
  vertices: (RelationshipVertex & { depth: number; seed: boolean })[];
  matched: string[];
  edges: RelationshipEdge[];
  diagnostics: RelationshipDiagnostic[];
  complete: boolean;
}

export interface BacklinkSource {
  vertex: RelationshipVertex;
  owner_vertex?: RelationshipVertex;
  edges: RelationshipEdge[];
}

export interface BacklinkGroup {
  kind: string;
  sources: BacklinkSource[];
}

export interface BacklinksView {
  groups: BacklinkGroup[];
  count: number;
  diagnostics: RelationshipDiagnostic[];
  complete: boolean;
}

// GET /api/map keeps its ordered work display tree separate from the full
// relationship index. Only these original edges drive affinity and chains.
// A depends_on layout arrow is drawn from the dependency to its dependent.
export type MapEdgeKind = "depends_on" | "attached_to" | "resolved_by";

export interface MapNodeView {
  noun: string;
  id: string;
  key?: string;
  title?: string;
  status: string;
  owner?: string;
  owner_name?: string;
  contributors?: ContributorInfo[];
  parent?: string;
  holds?: number;
}

export interface MapEdgeView {
  from: string;
  to: string;
  kind: MapEdgeKind;
}

export interface MapTreeView {
  nodes: MapNodeView[];
  edges: MapEdgeView[];
}

export interface MapView extends MapTreeView {
  relationships: RelationshipGraph;
}

export const mapPath = "/api/map";

// GET /api/tree (gen.TreeModel): epics, features, and tasks in shared dependency
// order, excluding cancelled epic subtrees from console browsing.
export interface TreeModel {
  project: string;
  nodes: TreeNode[];
}

// One event in the raw feed (serve's eventFeedEntry): identity, actor, the
// literal command, target node ids, and touched file paths. Targets and files
// are always arrays.
export interface EventEntry {
  id: string;
  at: string;
  type: string;
  contributor: string;
  command: string;
  targets: string[];
  files: string[];
}

// GET /api/events (serve's eventFeed), newest first. Count is the number of
// events in this response.
export interface EventFeed {
  count: number;
  events: EventEntry[];
}

// One thing waiting on the human (views.QueueEntry). Kind names the gate:
// approve_amendment, approve_epic, approve_feature, approve_tasks,
// answer_question, attest_gate, attest_criterion, feature_check (an active
// feature with every task done and a required feature-grain gate unrecorded;
// the rollup is parked until `tiller check feature` runs). There is no
// acceptance kind, because epic done is a rollup `sync` records rather than a
// gate anyone waits on (artifact-schema.md, Epics States). Prose is the
// entry's one line of human meaning; command is the exact CLI invocation, the
// copy-command affordance; due carries a question's due hint. Gate and
// gate_status ride only on attest_gate, the gate's name and its current
// stored status; narrow with isAttestGate rather than checking kind by hand.
// approve_amendment reaches Home's queue card the same as any other kind, but
// the node page's attention stack filters it out (NodeDetail.tsx's
// asksFrom): the amendment's own copyable command stays queue-card-only.
export interface QueueEntry {
  kind: string;
  noun: string;
  id: string;
  key?: string;
  ref: string;
  prose?: string;
  command: string;
  due?: string;
  gate?: string;
  gate_status?: string;
  actor?: string;
  phase?: string;
}

// GET /api/queue (views.Queue). Entries is always an array, empty when nothing
// needs the human.
export interface Queue {
  entries: QueueEntry[];
}

// A well-formed attest_gate entry (views.QueueEntry narrowed): gate and
// gate_status are always present together, rather than the optional fields
// QueueEntry types them as generally.
export interface AttestGateEntry extends QueueEntry {
  kind: "attest_gate";
  gate: string;
  gate_status: string;
}

// Narrows QueueEntry to the attest_gate case, where gate and gate_status are
// always present; every other kind may omit them.
export function isAttestGate(e: QueueEntry): e is AttestGateEntry {
  return e.kind === "attest_gate" && typeof e.gate === "string" && typeof e.gate_status === "string";
}

// A recap group's owning epic (views.RecapEpic). Title is absent when the epic
// has none, which is the group head's cue to lead with the key instead: a
// rendered row leads with a name and lets the key follow as muted mono, and a
// blank is not a name.
export interface RecapEpic {
  key: string;
  id: string;
  title?: string;
  ref: string;
}

// One event target resolved against the tree (views.TargetRef). An id the tree
// no longer knows keeps the bare id as its ref with an empty noun and no
// owner, so guard before linking. Key is absent for nouns that carry none,
// including questions; title is the bounded prose handle those rows lead with.
export interface TargetRef {
  noun: string;
  id: string;
  key?: string;
  title?: string;
  ref: string;
  owner?: string;
  contributors?: ContributorInfo[];
}

// One recorded event in the digest (views.RecapEntry). Ask flags an entry that
// created an ask for the reader; targets is always an array.
export interface RecapEntry {
  at: string;
  type: string;
  contributor: string;
  command: string;
  ask: boolean;
  targets: TargetRef[];
}

// One (epic, stream) cell of the digest (views.RecapGroup). Epic is null for
// the project-level groups, which sort last. Entries is ONE newest-first
// timeline, asks in their real place, which is what lets the view collapse
// consecutive same-type runs and still call the run a moment in time. Omitted
// is how many routine entries the API's per-group cap dropped, 0 when it
// dropped none; asks are never capped, so an omitted count is always routine
// activity.
export interface RecapGroup {
  epic: RecapEpic | null;
  stream: string;
  entries: RecapEntry[];
  omitted: number;
}

// GET /api/recap (views.Recap). Since is the horizon as RFC3339 UTC, empty on
// a first visit (which recaps nothing); window and reader echo the request;
// total is the TRUE count of digested events before any group's cap, not the
// number served; groups is always an array.
export interface Recap {
  since: string;
  window: string;
  reader: string;
  total: number;
  groups: RecapGroup[];
}

// The windows /api/recap accepts; anything else is a 400.
//
// Kept "Recap" on 2026-08-10 when the view that renders it renamed to
// History (task-6-brief.md item 3): the view's own name is a presentation
// choice, this type names the wire's request shape, and the two are free to
// disagree.
export type RecapWindow = "visit" | "24h" | "7d";

// GET /api/recap/peek (serve.recapPeek). The counts-only read behind Home's
// recap signal: the same horizon and totals the visit-window recap would
// answer, served WITHOUT advancing the visit, so glancing at Home never spends
// a recap nobody opened. Total is the same true pre-cap count Recap carries;
// asks is how many of those events ask something of the reader.
export interface RecapPeek {
  since: string;
  reader: string;
  total: number;
  asks: number;
}

// One search match (search.Hit): a line in a .tiller/ markdown file (line is
// 1-based), or a question's statement or recorded answer, which live only in
// the ledger yaml and carry line 0 and the field that matched.
export interface SearchHit {
  path: string;
  line: number;
  snippet: string;
  field?: string;
}

// How a node matched at its strongest (search.NodeResult.Match), strongest
// first: the query named its id or reference, its whole key, its title, a
// fragment of its key, or only its prose and ledger fields.
export type SearchMatch = "id" | "key" | "title" | "key_fragment" | "text";

// One node and everything in it that matched (search.NodeResult). count is
// every matching line and ledger field; hits is the first few of them. A
// matching title is neither, since the title is already the row's label.
export interface SearchNodeResult {
  noun: string;
  id: string;
  key?: string;
  title: string;
  status: string;
  ref: string;
  match: SearchMatch;
  count: number;
  hits: SearchHit[];
}

// One file no node owns and its matching lines (search.DocumentResult).
export interface SearchDocumentResult {
  path: string;
  count: number;
  hits: SearchHit[];
}

// How many results of one kind matched: a node noun, or "document".
export interface SearchKindCount {
  kind: string;
  count: number;
}

// GET /api/search (search.Result). Nodes arrive ranked and documents after
// them. count is how many results matched within kind (all kinds when kind is
// absent), so truncated says the page dropped some; kinds tallies every kind
// whatever the filter. Every list is an array.
export interface SearchResult {
  query: string;
  kind?: string;
  count: number;
  truncated: boolean;
  kinds: SearchKindCount[];
  nodes: SearchNodeResult[];
  documents: SearchDocumentResult[];
}

// A drift finding addresses either a work node or a design doc. Design rows
// have noun="design", id="" and a canonical design-relative key, with optional
// upper history anchor and matching features. Files remain plain text:
// active_basis uses basenames, while design describes may name external code.
// Neither is a safe Markdown destination. Only feature specs and design doc
// keys carry the explicit coordinates required for file links.
export interface CandidateView {
  marker: string;
  noun: string;
  id: string;
  key?: string;
  detail?: string;
  files?: string[];
  since?: RevisionRefView;
  until?: RevisionRefView;
  features?: DesignFeatureView[];
  context?: string[];
  // The clearing move as one runnable command (views.candidateRemedy),
  // absent where no command exists (stale_spec, orphan_eval): the panel
  // renders what exists and invents nothing.
  remedy?: string;
}

export interface DesignFeatureView {
  id: string;
  key: string;
  spec: string;
  commit: RevisionRefView;
}

// The drift panel's payload (views.NodeDriftView): one node page's findings
// at candidate grain, uncovered rows included, beside the one-per-marker
// badge list the chip renders from. Absent on a clean node.
export interface NodeDriftView {
  candidates?: CandidateView[];
  uncovered?: UncoveredView[];
}

// One node a marker's sweep could not compare (views.UncoveredView): flat
// across every marker (a node can land here under more than one (marker,
// reason) pair, drift.Uncovered's own doc comment), so every row carries its
// own marker rather than relying on section context the way a candidate row
// can. Same Ref carve-out as CandidateView above.
export interface UncoveredView {
  marker: string;
  noun: string;
  id: string;
  key?: string;
  reason: string;
  detail?: string;
}

// The exact projection `tiller drift --json` renders. Coverage is per marker:
// criteria for orphan_eval, docs for baseline markers, nodes otherwise.
// compared_design is always emitted by current binaries; optional here so an
// older node-only response keeps its existing rendering.
export interface DriftReportView {
  candidates?: Record<string, CandidateView[]>;
  uncovered?: UncoveredView[];
  disabled?: string[];
  exempted?: Record<string, number>;
  coverage?: Record<string, Record<string, number>>;
  compared: number;
  compared_design?: number;
}

// The Report page's payload (internal/views/report.go): ready-to-draw weekly
// series over the whole project, counts and durations only. Every weekly
// series array has one entry per element of weeks; the last week is the current,
// partial one. week_boundaries also includes the next Monday, so instant
// positions use the server's timezone and actual week lengths across DST.
export interface Report {
  project: string;
  as_of: string;
  off_branch?: ReportBranchNotice;
  weeks: string[]; // YYYY-MM-DD labels
  week_boundaries: string[]; // RFC3339; length == weeks.length + 1
  kpis: ReportKPIs;
  burn_up: ReportBurnUpWeek[];
  throughput: ReportThroughputWeek[];
  bugs: ReportBugWeek[];
  feature_times: ReportFeatureTime[];
  epics: ReportEpic[];
}

export interface ReportBranchNotice {
  branch: string;
  integration: string;
}

export interface ReportKPIs {
  features_done: number;
  features_total: number;
  shipped_last_7_days: number;
  median_cycle_days: number | null;
  median_lead_days: number | null;
  completed_features: number;
  open_bugs: number;
  open_questions: number;
  answered_questions: number;
  open_questions_by_week: number[];
}

export interface ReportBurnUpWeek {
  scope: number;
  done: number;
}

export interface ReportThroughputWeek {
  feature_work: number;
  fixes: number;
  upkeep: number;
}

export interface ReportBugWeek {
  reported: number;
  fixed: number;
  open: number;
}

export interface ReportFeatureTime {
  title: string;
  completed_at: string;
  cycle_days: number;
  lead_days: number;
  cycle_median_days: number;
  lead_median_days: number;
}

// A planned epic is a draft the client has not approved yet: a title and its
// planned_features count with zero progress counts, drawn as a ghost row and
// never counted as scope.
export interface ReportEpic {
  title: string;
  done: number;
  in_progress: number;
  not_started: number;
  planned: boolean;
  planned_features: number;
}

// Paths come in two forms on purpose: constants and builders for useFetch,
// which refetches when its path string changes, and fetch functions for
// imperative callers. Both go through the same strings so a view cannot drift
// from the endpoint it names.

export const versionPath = "/api/version";
export const statusPath = "/api/status";
export const boardPath = "/api/board";
export const treePath = "/api/tree";
export const queuePath = "/api/queue";
export const driftPath = "/api/drift";
export const nextPath = "/api/next";

// nodePagePath addresses a node page. The address is usually an id, but a
// canonical key's slashes are real path segments to the API, so each segment
// is encoded on its own and the slashes survive.
export function nodePagePath(noun: string, addr: string): string {
  const encoded = addr.split("/").map(encodeURIComponent).join("/");
  return `/api/node/${encodeURIComponent(noun)}/${encoded}`;
}

// prosePath addresses one exact line in a repo-relative Markdown file. The
// whole path is a query value here (unlike nodePagePath's path segments), so
// encodeURIComponent deliberately escapes its slashes as well as spaces and
// fragment characters.
export function prosePath(path: string, line: number): string {
  return `/api/prose?path=${encodeURIComponent(path)}&line=${String(line)}`;
}

// listPath builds /api/ls with only the active filters, in a fixed order so
// the same query yields the same string (useFetch keys its refetch on it).
export function listPath(noun: string, filters: ListFilters = {}): string {
  const params = new URLSearchParams({ noun });
  for (const axis of ["status", "owner", "contributor", "feature", "epic"] as const) {
    const value = filters[axis];
    if (value !== undefined && value !== "") params.set(axis, value);
  }
  return `/api/ls?${params.toString()}`;
}

export function eventsPath(limit?: number): string {
  return limit === undefined ? "/api/events" : `/api/events?limit=${String(limit)}`;
}

// Kept "recap" on 2026-08-10 when the view that calls this renamed to
// History (task-6-brief.md item 3, views/History.tsx): the route and this
// function name the wire, which did not rename with it.
export function recapPath(window?: RecapWindow): string {
  return window === undefined ? "/api/recap" : `/api/recap?window=${window}`;
}

// The marker History's visit-window read carries, the only request the server
// lets advance the visit horizon (internal/serve/home.go's visitHeader). A
// cross-site page cannot attach a custom header without a CORS preflight the
// server never answers, so a plain GET from anywhere else reads the digest
// without spending it.
export const visitMarker: HeadersInit = { "X-Tiller-Visit": "1" };

// The recap's non-consuming twin. It is a separate route rather than a
// parameter on recapPath because the difference is not the payload's shape but
// whether the request moves the visit horizon, and only the recap view is
// allowed to move it.
export const recapPeekPath = "/api/recap/peek";

// The Report page's metrics payload (internal/serve, GET /api/report).
export const reportPath = "/api/report";

// searchPath addresses the search, narrowed to one kind when the reader picks
// a kind tab. It sends no limit and takes the API's own page (MaxResults),
// the same page `tiller search` takes, so the console and the terminal never
// disagree about what the corpus holds. A kind tab asks the server rather than
// filtering the all-kinds page, so a kind is never cut short by the others'
// share of that page.
export function searchPath(query: string, kind?: string): string {
  const params = new URLSearchParams({ q: query });
  if (kind !== undefined && kind !== "") params.set("kind", kind);
  return `/api/search?${params.toString()}`;
}

export function relationshipQueryPath(from: string, follow: string): string {
  return `/api/query?${new URLSearchParams({ from, follow }).toString()}`;
}

export function fetchStatus(): Promise<ProjectSummary> {
  return fetchJSON<ProjectSummary>(statusPath);
}

// Tenant restored 2026-08-10 (ui-surface.md, "Home gains the pulse"): Home's
// In flight column renders these owner streams.
export function fetchBoard(): Promise<Streams> {
  return fetchJSON<Streams>(boardPath);
}

export function fetchNodePage(noun: string, addr: string): Promise<NodePage> {
  return fetchJSON<NodePage>(nodePagePath(noun, addr));
}

export function fetchList(noun: string, filters: ListFilters = {}): Promise<ListResult> {
  return fetchJSON<ListResult>(listPath(noun, filters));
}

export function fetchTree(): Promise<TreeModel> {
  return fetchJSON<TreeModel>(treePath);
}

export function fetchEvents(limit?: number): Promise<EventFeed> {
  return fetchJSON<EventFeed>(eventsPath(limit));
}

export function fetchQueue(): Promise<Queue> {
  return fetchJSON<Queue>(queuePath);
}

export function fetchRecap(window?: RecapWindow): Promise<Recap> {
  return fetchJSON<Recap>(recapPath(window));
}

export function fetchRecapPeek(): Promise<RecapPeek> {
  return fetchJSON<RecapPeek>(recapPeekPath);
}

export function fetchSearch(query: string, kind?: string): Promise<SearchResult> {
  return fetchJSON<SearchResult>(searchPath(query, kind));
}

export function fetchDrift(): Promise<DriftReportView> {
  return fetchJSON<DriftReportView>(driftPath);
}

// One node a ranked next move acts on (views.NextNode): the noun/id/key/ref
// shape every other addressable row on the console carries, plus the node's
// one-line title where the server has one. Key and title ride omitempty, so
// an absent key reads through addrOf's fallback to the id.
export interface NextNode {
  noun: string;
  id: string;
  key?: string;
  ref: string;
  title?: string;
}

// One legal next move (views.NextCandidate): who makes it, what it does, to
// which node, and the command or skill that does it, with the one-line reason
// for its rank. reason is a lowercase-led phrase that lib/sentence.ts words
// as a sentence, the way `tiller next` prints it.
export interface NextCandidate {
  actor: "human" | "agent";
  kind: string;
  action: string;
  node?: NextNode;
  command?: string;
  skill?: string;
  reason: string;
  tier: number;
  branch?: string;
  workflow?: WorkflowAdvice;
}

// The tree's dependency gaps and advisories (readiness.Report), present on
// GET /api/next only when there is at least one.
export interface DependencyDiagnostics {
  ready: boolean;
  gaps: { reason: string; message: string }[];
  advisories: { reason: string; message: string }[];
}

// GET /api/next (views.Next, the same payload `tiller next --json` carries):
// the ranked list for the served checkout's contributor, already cut to the
// top five, best first, so the console never ranks or recomputes it. reader
// is "" when no identity resolved and the list is unpersonalized. hint
// explains a caught-up list's held work, or how work enters an unplanned
// tree.
export interface NextAnswer {
  reader: string;
  candidates: NextCandidate[];
  caught_up: boolean;
  unplanned?: boolean;
  hint?: string;
  dependency_diagnostics?: DependencyDiagnostics;
}
