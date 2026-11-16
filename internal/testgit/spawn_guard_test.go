package testgit_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// routedPrograms are the programs a test can reach Git through: git itself, a
// shell running a pasted command line, and the go tool, whose build stamps a
// binary by asking Git about the checkout.
var routedPrograms = []string{"git", "sh", "go"}

// TestTestsStartGitUnderTheFixtureEnvironment reads every Go test file outside
// internal/vcs and internal/testgit and fails when one starts git, sh, or go
// with the environment it inherited. A suite started from a Git hook or
// `git rebase --exec` inherits GIT_DIR and GIT_WORK_TREE, and a fixture command
// that keeps them acts on the developer's checkout instead of its temporary
// directory.
//
// The rule it holds: the command is assigned to a variable, and the same
// function sets that variable's Env from testgit.Env(), directly or through a
// function of the same package that calls it. testgit.Run already does this.
func TestTestsStartGitUnderTheFixtureEnvironment(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "go.mod"))
	exempt := map[string]bool{
		filepath.Join(root, "internal", "vcs"):     true,
		filepath.Join(root, "internal", "testgit"): true,
	}

	byDir := map[string][]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if exempt[path] || name == "node_modules" || name == "testdata" || (path != root && strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			byDir[filepath.Dir(path)] = append(byDir[filepath.Dir(path)], path)
		}
		return nil
	}))
	require.NotEmpty(t, byDir, "the walk found no test files; the guard is reading the wrong directory")

	var unguarded []string
	for _, paths := range byDir {
		fset := token.NewFileSet()
		var files []*ast.File
		for _, path := range paths {
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			require.NoError(t, err)
			files = append(files, file)
		}
		builders := environmentBuilders(files)
		for _, file := range files {
			for _, decl := range file.Decls {
				for _, spawn := range unguardedSpawns(decl, builders) {
					position := fset.Position(spawn.Pos())
					rel, err := filepath.Rel(root, position.Filename)
					require.NoError(t, err)
					unguarded = append(unguarded, fmt.Sprintf("%s:%d", filepath.ToSlash(rel), position.Line))
				}
			}
		}
	}
	slices.Sort(unguarded)
	require.Empty(t, unguarded, "these tests start git, sh, or go with the inherited environment; "+
		"use testgit.Run, or assign the command to a variable and set its Env from testgit.Env()")
}

// environmentBuilders names the package's functions that call testgit.Env(),
// so a helper that adds its own settings on top still counts.
func environmentBuilders(files []*ast.File) map[string]bool {
	builders := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Body != nil && usesFixtureEnvironment(fn.Body, nil) {
				builders[fn.Name.Name] = true
			}
		}
	}
	return builders
}

// unguardedSpawns returns the exec.Command and exec.CommandContext calls in
// one declaration that start a routed program without the fixture
// environment.
func unguardedSpawns(decl ast.Decl, builders map[string]bool) []*ast.CallExpr {
	guarded := map[string]bool{}
	assigned := map[*ast.CallExpr]string{}
	var spawns []*ast.CallExpr
	ast.Inspect(decl, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.AssignStmt:
			if len(node.Lhs) != 1 || len(node.Rhs) != 1 {
				return true
			}
			if call, ok := node.Rhs[0].(*ast.CallExpr); ok {
				if name, ok := node.Lhs[0].(*ast.Ident); ok {
					assigned[call] = name.Name
				}
			}
			if field, ok := node.Lhs[0].(*ast.SelectorExpr); ok && field.Sel.Name == "Env" {
				if name, ok := field.X.(*ast.Ident); ok && usesFixtureEnvironment(node.Rhs[0], builders) {
					guarded[name.Name] = true
				}
			}
		case *ast.CallExpr:
			if startsRoutedProgram(node) {
				spawns = append(spawns, node)
			}
		}
		return true
	})
	var unguarded []*ast.CallExpr
	for _, spawn := range spawns {
		if name, ok := assigned[spawn]; !ok || !guarded[name] {
			unguarded = append(unguarded, spawn)
		}
	}
	return unguarded
}

// startsRoutedProgram reports whether call is exec.Command or
// exec.CommandContext naming one of the routed programs literally.
func startsRoutedProgram(call *ast.CallExpr) bool {
	fn, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if pkg, ok := fn.X.(*ast.Ident); !ok || pkg.Name != "exec" {
		return false
	}
	program := 0
	switch fn.Sel.Name {
	case "Command":
	case "CommandContext":
		program = 1
	default:
		return false
	}
	if len(call.Args) <= program {
		return false
	}
	literal, ok := call.Args[program].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return false
	}
	name, err := strconv.Unquote(literal.Value)
	if err != nil {
		return false
	}
	return slices.Contains(routedPrograms, name)
}

// usesFixtureEnvironment reports whether node calls testgit.Env() or one of
// the package's environment builders.
func usesFixtureEnvironment(node ast.Node, builders map[string]bool) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return !found
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if pkg, ok := fn.X.(*ast.Ident); ok && pkg.Name == "testgit" && fn.Sel.Name == "Env" {
				found = true
			}
		case *ast.Ident:
			if builders[fn.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}
