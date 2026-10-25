package store

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/rootpath"

	"gopkg.in/yaml.v3"
)

// This file loads the whole .tiller/ tree into memory: the manifest plus every
// node in the work tree (epics, their features, their tasks) and the
// open-question ledger (artifact-schema.md, Directory layout). It is read-only;
// mutations go through write.go.

// Tree is the in-memory view of a .tiller/ directory. The node maps are keyed by
// id, the one stable identifier every node carries, so a rename never moves a
// node between buckets. Lazy paths that have not been created yet (a feature's
// tasks/ subtree, the open-questions ledger) simply contribute nothing.
type Tree struct {
	Root     string
	Manifest *Manifest
	Epics    map[string]*Epic
	Features map[string]*Feature
	Tasks    map[string]*Task
	// Contributors is the snapshot's sorted, deduplicated started-task owners
	// for each feature and epic. Shelf attachments never contribute.
	Contributors map[string][]Contributor
	// FeatureTasks groups each feature's tasks under its feature id, in the
	// order loadTasks appended them, which is whatever order os.ReadDir
	// returned the task folders in. A folder carries no sequence of its own;
	// the recorded sequence is depends_on (artifact-schema.md, Tasks:
	// Ordering), which gen ranks. Tasks and FeatureTasks are two indexes over
	// one set of *Task values, so a mutation through either is visible in both.
	FeatureTasks map[string][]*Task
	Questions    map[string]*Question
	// Amendments holds every staged reshape under .tiller/amendments/, keyed by
	// id like every other node map. Unlike Epics/Features/Tasks it is not a
	// folder-backed subtree: each amendment is a flat file pair, so there is no
	// per-amendment children index to keep beside it.
	Amendments map[string]*Amendment
	// Bugs and Chores are the two shelves, keyed by id like every other
	// map (docs/plans/2026-08-28-bugs-and-chores-design.md, Where they live).
	Bugs   map[string]*Bug
	Chores map[string]*Chore
	Tweaks map[string]*Tweak
	// BugOrder and ChoreOrder are each shelf's recorded soft order.
	BugOrder   ShelfOrder
	ChoreOrder ShelfOrder
	TweakOrder ShelfOrder
	// ProjectOrder is the root soft order read from .tiller/order.yml
	// (order.go). It is machine-owned like every other state file Load reads,
	// and its absence is a zero value, not an error.
	ProjectOrder ProjectOrder
	// Baseline records design coverage independently of work-node lifecycle.
	Baseline BaselineLedger
	// ToBeDesign lists the design inputs under .tiller/design/ (regular
	// files, dotfiles excepted) that the baseline ledger neither registers
	// nor cites as a --from source: the to-be docs whose presence makes
	// decompose due on a tree with no epics. Sorted, slash-separated,
	// relative to .tiller/design/; nil when there are none. Gathered only
	// when the tree has no epics, the one case the route consults it; nil
	// otherwise.
	ToBeDesign []string
	generation string
	// reader is the load's confined multi-file reader. It is set only while
	// loadTreeWithManifest runs and is nil on every returned tree.
	reader *RepositoryReader
}

type nodeIDRegistry struct {
	paths map[string]string
}

func (r *nodeIDRegistry) claim(noun, nodeID, path string) error {
	if !id.Valid(nodeID) {
		return fmt.Errorf("parse %s: %s id %s is invalid", path, noun, nodeID)
	}
	prefix, err := id.PrefixFor(noun)
	if err != nil {
		return fmt.Errorf("parse %s: %s id %s: %w", path, noun, nodeID, err)
	}
	if !strings.HasPrefix(nodeID, prefix+"-") {
		return fmt.Errorf("parse %s: %s id %s must start with %s-", path, noun, nodeID, prefix)
	}
	if prior, duplicate := r.paths[nodeID]; duplicate {
		return fmt.Errorf("parse %s: %s id %s is already recorded in %s; one id names one %s",
			path, noun, nodeID, prior, noun)
	}
	r.paths[nodeID] = path
	return nil
}

// Load reads the .tiller/ tree under root. It fails with an *EnvError (exit 3)
// when root is not a Tiller repo (no manifest), and surfaces a parse error for
// any malformed node file.
func Load(root string) (*Tree, error) {
	return loadStable(root, loadTree)
}

func loadStable(root string, load func(string) (*Tree, error)) (*Tree, error) {
	before, err := readableGeneration(root)
	if err != nil {
		return nil, err
	}
	tree, loadErr := load(root)
	after, stateErr := readableGeneration(root)
	if stateErr != nil {
		return nil, stateErr
	}
	if err := stableGeneration(before, after); err != nil {
		return nil, err
	}
	if loadErr != nil {
		return nil, loadErr
	}
	tree.generation = after
	return tree, nil
}

// UncheckedTree loads the complete tree using an already parsed manifest,
// bypassing only the schema-version refusal. Migration needs the real node
// inventory and the same stable generation boundary as ordinary readers.
func UncheckedTree(root string, m *Manifest) (*Tree, error) {
	return loadStable(root, func(root string) (*Tree, error) {
		return loadTreeWithManifest(root, m)
	})
}

// CheckStable closes a loaded tree's read boundary after a downstream
// projection has read Tiller-owned files that are not part of loadTree.
func (t *Tree) CheckStable() error {
	after, err := readableGeneration(t.Root)
	if err != nil {
		return err
	}
	return stableGeneration(t.generation, after)
}

func loadTree(root string) (*Tree, error) {
	m, err := LoadManifest(root)
	if err != nil {
		return nil, err
	}
	return loadTreeWithManifest(root, m)
}

func loadTreeWithManifest(root string, m *Manifest) (*Tree, error) {
	reader, err := OpenRepositoryReader(root)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	t := &Tree{
		Root:         root,
		Manifest:     m,
		Epics:        map[string]*Epic{},
		Features:     map[string]*Feature{},
		Tasks:        map[string]*Task{},
		FeatureTasks: map[string][]*Task{},
		Questions:    map[string]*Question{},
		Amendments:   map[string]*Amendment{},
		Bugs:         map[string]*Bug{},
		Chores:       map[string]*Chore{},
		Tweaks:       map[string]*Tweak{},
		reader:       reader,
	}
	defer func() { t.reader = nil }()

	ids := &nodeIDRegistry{paths: map[string]string{}}
	if err := t.loadEpics(ids); err != nil {
		return nil, err
	}
	if err := t.loadQuestions(ids); err != nil {
		return nil, err
	}
	if err := t.loadAmendments(ids); err != nil {
		return nil, err
	}
	for _, noun := range ShelfNouns() {
		if err := t.loadShelf(noun, ids); err != nil {
			return nil, err
		}
	}
	po, err := LoadProjectOrder(root)
	if err != nil {
		return nil, err
	}
	t.ProjectOrder = po
	t.Baseline, err = LoadBaseline(root)
	if err != nil {
		return nil, err
	}
	if len(t.Epics) == 0 {
		t.loadToBeDesign()
	}
	if m.SchemaVersion >= 6 {
		if err := t.ValidateOwnership(); err != nil {
			return nil, err
		}
	}
	if err := t.validateRecoveredReviewGrains(); err != nil {
		return nil, err
	}
	t.ProjectContributors()
	return t, nil
}

func readGeneration(root string) (string, error) {
	path := filepath.Join(root, ".tiller", cacheDirName, generationFileName)
	data, err := readStateFile(root, path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// readableGeneration refuses reads while a mutation lock or recovery journal is
// present. The lock-free existence checks are not atomic: another writer can
// acquire the lock and checkpoint its journal between them, so the reader reports
// mutation_recovery_needed for a mutation still in flight (#106). After that
// writer finishes, a retry can succeed without recovery if it cleared the journal;
// a genuinely retained journal continues to refuse.
func readableGeneration(root string) (string, error) {
	generation, err := readGeneration(root)
	if errors.Is(err, rootpath.ErrChanged) {
		// A concurrent writer replaced the generation mid-read (#151).
		return "", mutationEnvError(ReasonMutationStale,
			"another Tiller mutation changed .tiller/.cache/generation while it was being read; retry")
	}
	if err != nil {
		return "", mutationEnvError("", "read mutation generation: %v", err)
	}

	lockPath := filepath.Join(root, ".tiller", cacheDirName, lockFileName)
	lockExists, err := PathExistsNoSymlinks(root, lockPath)
	if err != nil {
		return "", mutationEnvError("", "inspect mutation lock %s: %v", lockPath, err)
	}
	if lockExists {
		return "", lockedEnvError("another Tiller mutation is in progress (.tiller/.cache/lock is held)")
	}

	journalPath := filepath.Join(root, ".tiller", cacheDirName, journalName)
	journalExists, err := PathExistsNoSymlinks(root, journalPath)
	if err != nil {
		return "", mutationEnvError("", "inspect mutation journal %s: %v", journalPath, err)
	}
	if journalExists {
		return "", mutationEnvError(
			ReasonMutationRecovery,
			"unfinished Tiller mutation requires recovery (.tiller/.cache/journal.yml is present)",
		)
	}
	return generation, nil
}

func stableGeneration(before, after string) error {
	if before != after {
		return mutationEnvError(
			ReasonMutationStale,
			"Tiller state changed while it was being read; retry",
		)
	}
	return nil
}

// loadEpics walks .tiller/epics/<epic-key>/, loading each epic.yml, then its
// features under features/<feature-key>/, then each feature's task folders.
func (t *Tree) loadEpics(ids *nodeIDRegistry) error {
	epicsDir := filepath.Join(t.Root, ".tiller", "epics")
	entries, err := t.stateDirectory(epicsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		epicKey := e.Name()

		epicYML := filepath.Join(epicsDir, epicKey, "epic.yml")
		var epic Epic
		if err := t.readNodeYAML(epicYML, &epic); err != nil {
			return err
		}
		if err := validateStoredReviewGrain(epicYML, NounEpic, epic.ReviewGrain); err != nil {
			return err
		}
		if err := ids.claim(NounEpic, epic.ID, epicYML); err != nil {
			return err
		}
		if err := checkKeyNamesItsFolder(epicYML, epicKey, epic.Key); err != nil {
			return err
		}
		t.Epics[epic.ID] = &epic

		if err := t.loadFeatures(epicsDir, epicKey, ids); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tree) loadFeatures(epicsDir, epicKey string, ids *nodeIDRegistry) error {
	featuresDir := filepath.Join(epicsDir, epicKey, "features")
	entries, err := t.stateDirectory(featuresDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, fe := range entries {
		if !fe.IsDir() {
			continue
		}
		featureKey := fe.Name()
		featureDir := filepath.Join(featuresDir, featureKey)

		featureYML := filepath.Join(featureDir, "feature.yml")
		var feat Feature
		if err := t.readNodeYAML(featureYML, &feat); err != nil {
			return err
		}
		if err := validateStoredReviewGrain(featureYML, NounFeature, feat.ReviewGrain); err != nil {
			return err
		}
		if err := ids.claim(NounFeature, feat.ID, featureYML); err != nil {
			return err
		}
		if err := checkKeyNamesItsFolder(featureYML, featureKey, feat.Key); err != nil {
			return err
		}
		feat.EpicKey = epicKey
		feat.HasEval = t.stateFileExists(filepath.Join(featureDir, "feature.eval.md"))
		t.Features[feat.ID] = &feat

		if err := t.loadTasks(featureDir, &feat, ids); err != nil {
			return err
		}
	}
	return nil
}

// loadTasks walks a feature's tasks/<task-key>/ folders, reading each task.yml
// (artifact-schema.md, Directory layout). The subtree is lazy: it appears with
// the feature's specification, so its absence is not an error.
//
// A directory here is a claim that a node lives in it, so a missing task.yml is
// a load error naming the path, the same refusal loadEpics and loadFeatures make
// for a folder with no epic.yml or feature.yml. Skipping it instead would drop
// the node from every surface with nothing to say it had been dropped. task.md
// is not read: the loader reads state, and no grain's prose file is loaded here.
func (t *Tree) loadTasks(featureDir string, feat *Feature, ids *nodeIDRegistry) error {
	tasksDir := filepath.Join(featureDir, TaskDirName)
	entries, err := t.stateDirectory(tasksDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		taskYML := filepath.Join(tasksDir, e.Name(), TaskStateFile)
		var task Task
		if err := t.stateYAML(taskYML, &task); err != nil {
			return err
		}
		if err := ids.claim(NounTask, task.ID, taskYML); err != nil {
			return err
		}
		if err := checkKeyNamesItsFolder(taskYML, e.Name(), task.Key); err != nil {
			return err
		}
		if err := validateStoredStartedGrain(taskYML, task.StartedGrain); err != nil {
			return err
		}
		task.FeatureID = feat.ID
		t.Tasks[task.ID] = &task
		t.FeatureTasks[feat.ID] = append(t.FeatureTasks[feat.ID], &task)
	}
	return nil
}

// loadQuestions reads .tiller/decisions/open-questions.yml if present.
func (t *Tree) loadQuestions(ids *nodeIDRegistry) error {
	path := filepath.Join(t.Root, ".tiller", "decisions", "open-questions.yml")
	data, err := t.stateFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	var qf QuestionsFile
	if err := yaml.Unmarshal(data, &qf); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	// Validate every entry before claiming or storing any, so a null entry
	// anywhere leaves no partial question list behind.
	for i, q := range qf.Questions {
		source := fmt.Sprintf("%s[%d]", path, i)
		if q == nil {
			return fmt.Errorf("parse %s: question entry is null; remove it or give it an id and statement", source)
		}
	}
	for i, q := range qf.Questions {
		source := fmt.Sprintf("%s[%d]", path, i)
		if err := ids.claim(NounQuestion, q.ID, source); err != nil {
			return err
		}
		t.Questions[q.ID] = q
	}
	return nil
}

// loadAmendments reads .tiller/amendments/*.yml if the directory exists. Each
// file is one amendment (artifact-schema.md, Amendments); the companion .md
// files carry prose and are not read here, the same split the loader keeps
// everywhere else. The directory is lazy like open-questions.yml: a project
// with no amendments yet has no amendments/ directory, and that absence is an
// empty map, never an error.
func (t *Tree) loadAmendments(ids *nodeIDRegistry) error {
	dir := filepath.Join(t.Root, ".tiller", "amendments")
	entries, err := t.stateDirectory(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		var amendment Amendment
		if err := t.stateYAML(path, &amendment); err != nil {
			return err
		}
		if err := ids.claim(NounAmendment, amendment.ID, path); err != nil {
			return err
		}
		t.Amendments[amendment.ID] = &amendment
	}
	return nil
}

// checkKeyNamesItsFolder refuses a folder-backed node whose stored key does not
// end in the folder the loader found it in (artifact-schema.md, Keys and paths:
// the directory segment is the LOCAL key, and the canonical key ends in it).
//
// The loader identifies a node by its folder. Every path derived for that node
// afterwards runs off its key instead: gen.ProsePath and the tree map's file
// footprint, search's state path, and mutate's write paths all rebuild the
// folder from the key. A disagreement is therefore not cosmetic, it points the
// reads and the writes at two different files, and neither side can notice
// alone. Refusing the load names both halves at the one place that can see both.
//
// A node with no key at all is a different fact, not a disagreement: it simply
// does not resolve by key, and nothing derives a path for it (see
// gen.ProsePath's ok result). It loads.
func checkKeyNamesItsFolder(path, folder string, key *string) error {
	if key == nil {
		return nil
	}
	local := *key
	if i := strings.LastIndex(local, "/"); i >= 0 {
		local = local[i+1:]
	}
	if local != folder {
		return fmt.Errorf("parse %s: recorded key %q names folder %q, but the node is in %q",
			path, *key, local, folder)
	}
	return nil
}

// fileExists reports whether path names an existing regular file, the stat the
// loader uses to record optional-file presence (a node's *.eval.md) without
// reading the file.
func fileExists(root, path string) bool {
	relative, err := RepositoryRelativePath(root, path)
	if err != nil {
		return false
	}
	info, err := RepositoryPathInfo(root, relative)
	return err == nil && info.Mode().IsRegular()
}

// readYAMLFile reads and unmarshals a YAML file into out, wrapping a parse error
// with the path for a legible message.
func readYAMLFile(root, path string, out any) error {
	data, err := readStateFile(root, path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
