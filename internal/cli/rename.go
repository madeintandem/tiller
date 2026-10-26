package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

type renameTitleView struct {
	Noun     string `json:"noun"`
	ID       string `json:"id"`
	Key      string `json:"key"`
	OldTitle string `json:"old_title"`
	Title    string `json:"title"`
	Changed  bool   `json:"changed"`
}

type renameKeyView struct {
	Axis    string   `json:"axis"`
	Noun    string   `json:"noun"`
	ID      string   `json:"id"`
	OldKey  string   `json:"old_key"`
	Key     string   `json:"key"`
	Moved   []string `json:"moved"`
	Changed bool     `json:"changed"`
}

func newRenameCmd() *cobra.Command {
	var title, key string
	cmd := &cobra.Command{
		Use:   "rename <epic|feature|task|bug|chore|tweak> <node> (--title <display-title> | --key <canonical-key>)",
		Short: "Change a node's display title or proposal key.",
		Long: "rename changes exactly one axis of an epic, feature, task, bug,\n" +
			"chore, or tweak. --title changes display metadata without editing Markdown, at\n" +
			"every lifecycle state. --key preserves identity and is limited to:\n" +
			"  - a draft epic containing only draft, task-free features;\n" +
			"  - a draft, task-free feature; changing parents requires both epics draft;\n" +
			"  - a proposed task staying under its current feature;\n" +
			"  - a proposed bug, chore, or tweak, using one local key segment.\n" +
			"For eligible unstarted features outside that window, use an amendment's\n" +
			"--move-feature <feature> --to <epic>/<key> --reason <text>, then human\n" +
			"amendment approval. Started descendants cannot move. See amend --help.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			titleSet := cmd.Flags().Changed("title")
			keySet := cmd.Flags().Changed("key")
			if titleSet == keySet {
				reason := "missing_rename_axis"
				if titleSet {
					reason = "multiple_rename_axes"
				}
				return emitFailure(cmd, "rename", &usageError{
					reason: reason,
					msg:    "rename requires exactly one of --title or --key",
				})
			}
			if keySet {
				return runRenameKey(cmd, args[0], args[1], key)
			}
			return runRenameTitle(cmd, args[0], args[1], title)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "the new display title")
	cmd.Flags().StringVar(&key, "key", "", "the new canonical key")
	addContributorFlag(cmd)
	return cmd
}

func runRenameTitle(cmd *cobra.Command, noun, node, title string) error {
	noun, err := validNoun(cmd, "rename", noun, []string{store.NounEpic, store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak})
	if err != nil {
		return err
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "rename", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "rename", err)
	}
	res, err := mutate.RenameTitle(tree, d, mutate.RenameTitleInput{Noun: noun, Node: node, Title: title})
	if err != nil {
		return emitFailure(cmd, "rename", err)
	}

	view := renameTitleView{
		Noun: res.Noun, ID: res.ID, Key: res.Key, OldTitle: res.OldTitle,
		Title: res.Title, Changed: res.Changed,
	}
	ref := gen.RefParts(res.Key, res.ID)
	var human string
	if res.Changed {
		human = fmt.Sprintf("Renamed %s %s display title from %q to %q.", res.Noun, ref, res.OldTitle, res.Title)
	} else {
		human = fmt.Sprintf("%s %s display title is already %q; unchanged.", titleNoun(res.Noun), ref, res.Title)
	}
	return emitSuccess(cmd, "rename", view, human)
}

func runRenameKey(cmd *cobra.Command, noun, node, key string) error {
	noun, err := validNoun(cmd, "rename", noun, []string{store.NounEpic, store.NounFeature, store.NounTask, store.NounBug, store.NounChore, store.NounTweak})
	if err != nil {
		return err
	}
	tree, err := loadTree(cmd)
	if err != nil {
		return emitFailure(cmd, "rename", err)
	}
	d, err := deps(cmd, tree.Root)
	if err != nil {
		return emitFailure(cmd, "rename", err)
	}
	res, err := mutate.RenameKey(tree, d, mutate.RenameKeyInput{Noun: noun, Node: node, Key: key})
	if err != nil {
		return emitFailure(cmd, "rename", err)
	}

	view := renameKeyView{
		Axis: "key", Noun: res.Noun, ID: res.ID, OldKey: res.OldKey,
		Key: res.Key, Moved: nonNilStrings(res.Moved), Changed: res.Changed,
	}
	var human string
	if !res.Changed {
		human = fmt.Sprintf("%s %s already has key %s; unchanged.",
			titleNoun(res.Noun), gen.RefParts(res.Key, res.ID), res.Key)
	} else {
		var b strings.Builder
		fmt.Fprintf(&b, "Renamed %s key from %s to %s; identity preserved.",
			res.Noun, gen.RefParts(res.OldKey, res.ID), gen.RefParts(res.Key, res.ID))
		if len(res.Moved) != 0 {
			b.WriteString("\nMoved files:")
			for _, path := range res.Moved {
				fmt.Fprintf(&b, "\n  %s", filepath.ToSlash(path))
			}
		}
		human = b.String()
	}
	return emitSuccess(cmd, "rename", view, human)
}

func titleNoun(noun string) string {
	if noun == "" {
		return noun
	}
	return strings.ToUpper(noun[:1]) + noun[1:]
}
