package cli

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

type gradeView struct {
	Noun        string `json:"noun"`
	ID          string `json:"id"`
	Key         string `json:"key"`
	OldSeverity string `json:"old_severity"`
	Severity    string `json:"severity"`
	Changed     bool   `json:"changed"`
}

func newGradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grade bug <node> <blocker|important|medium|minor>",
		Short: "Set a bug's severity (blocker, important, medium, minor).",
		Long: "grade sets or changes a non-terminal bug's severity. Any contributor may\n" +
			"run it. Severity is a label for people: it never changes routing, tiller\n" +
			"next, or any queue's order. A done or cancelled bug refuses, since its\n" +
			"grade is part of the closed record.",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != store.NounBug {
				return emitFailure(cmd, "grade", &usageError{
					reason: "bad_args",
					msg:    fmt.Sprintf("grade applies to bugs; got %q (use: grade bug <node> <level>)", args[0]),
				})
			}
			tree, err := loadTree(cmd)
			if err != nil {
				return emitFailure(cmd, "grade", err)
			}
			d, err := deps(cmd, tree.Root)
			if err != nil {
				return emitFailure(cmd, "grade", err)
			}
			res, err := mutate.Grade(tree, d, mutate.GradeInput{Noun: store.NounBug, Node: args[1], Severity: args[2]})
			if err != nil {
				return emitFailure(cmd, "grade", err)
			}
			view := gradeView{
				Noun: store.NounBug, ID: res.ID, Key: res.Key,
				OldSeverity: res.OldSeverity, Severity: res.Severity, Changed: res.Changed,
			}
			ref := gen.RefParts(res.Key, res.ID)
			var human string
			switch {
			case !res.Changed:
				human = fmt.Sprintf("Bug %s: severity is already %s; unchanged.", ref, res.Severity)
			case res.OldSeverity == "":
				human = fmt.Sprintf("Bug %s: severity unrated → %s.", ref, res.Severity)
			default:
				human = fmt.Sprintf("Bug %s: severity %s → %s.", ref, res.OldSeverity, res.Severity)
			}
			return emitSuccess(cmd, "grade", view, human)
		},
	}
	addContributorFlag(cmd)
	return cmd
}
