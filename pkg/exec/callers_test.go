package exec

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const pkgExecPath = "github.com/stashapp/stash/pkg/exec"

// runMethods are the exec.Cmd methods that must not be called on a command
// made by this package, because on Windows they would bypass Start.
var runMethods = map[string]bool{"Start": true, "Run": true, "Output": true, "CombinedOutput": true}

// findDirectStarts reports calls such as cmd.Run() on commands made by this
// package, in one file. A command counts as made by this package when it
// comes from pkg/exec's Command or CommandContext, or from a Command method
// (FFMpeg.Command, Python.Command), which wrap them. It follows commands
// through local variables only, which is how Stash's code uses them.
func findDirectStarts(fset *token.FileSet, file *ast.File) []string {
	imports := map[string]string{} // local name -> import path
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imports[name] = path
	}

	isMadeHere := func(e ast.Expr) bool {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if pkg, ok := sel.X.(*ast.Ident); ok {
			if path, isPkg := imports[pkg.Name]; isPkg {
				return path == pkgExecPath && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext")
			}
		}
		return sel.Sel.Name == "Command"
	}

	var found []string
	checkBody := func(body *ast.BlockStmt) {
		made := map[string]bool{}
		ast.Inspect(body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				for i, rhs := range n.Rhs {
					if i < len(n.Lhs) && isMadeHere(rhs) {
						if id, ok := n.Lhs[i].(*ast.Ident); ok {
							made[id.Name] = true
						}
					}
				}
			case *ast.ValueSpec:
				for i, v := range n.Values {
					if i < len(n.Names) && isMadeHere(v) {
						made[n.Names[i].Name] = true
					}
				}
			case *ast.CallExpr:
				sel, ok := n.Fun.(*ast.SelectorExpr)
				if !ok || !runMethods[sel.Sel.Name] || len(n.Args) != 0 {
					return true
				}
				id, isIdent := sel.X.(*ast.Ident)
				if isMadeHere(sel.X) || (isIdent && made[id.Name]) {
					found = append(found, fmt.Sprintf("%s: .%s() on a pkg/exec command", fset.Position(n.Pos()), sel.Sel.Name))
				}
			}
			return true
		})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			// Closures are checked with the function they are in, so they see its
			// variables.
			if n.Body != nil {
				checkBody(n.Body)
			}
			return false
		case *ast.FuncLit:
			checkBody(n.Body) // a function literal outside any function
			return false
		}
		return true
	})
	return found
}

func TestFindDirectStartsCatchesDirectCalls(t *testing.T) {
	const src = `package p

import (
	"context"
	"os/exec"

	stashExec "github.com/stashapp/stash/pkg/exec"
)

func a(ctx context.Context, f *FFMpeg) {
	cmd := stashExec.Command("x")
	_ = cmd.Run()
	_, _ = stashExec.CommandContext(ctx, "x").Output()
	var c2 = f.Command(ctx, nil)
	_ = c2.Start()
	go func() {
		c3 := f.Command(ctx, nil)
		_, _ = c3.CombinedOutput()
	}()

	// allowed
	ok := stashExec.Command("x")
	_ = stashExec.Run(ok)
	_ = exec.Command("explorer").Run()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := findDirectStarts(fset, file)
	want := []string{
		"p.go:12:6: .Run()",
		"p.go:13:9: .Output()",
		"p.go:15:6: .Start()",
		"p.go:18:10: .CombinedOutput()",
	}
	if len(found) != len(want) {
		t.Fatalf("found %d calls, want %d:\n%s", len(found), len(want), strings.Join(found, "\n"))
	}
	for i := range want {
		if !strings.HasPrefix(found[i], want[i]) {
			t.Errorf("got %q, want prefix %q", found[i], want[i])
		}
	}
}

// TestNoDirectStarts checks that Stash's own code starts every pkg/exec
// command through Start, Run, Output or CombinedOutput.
func TestNoDirectStarts(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skipf("repository root not found: %v", err)
	}
	self, _ := filepath.Abs(".")

	var found []string
	files := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "ui", "vendor", "testdata":
				return filepath.SkipDir
			}
			if path == self {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files++
		found = append(found, findDirectStarts(fset, file)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 100 {
		t.Fatalf("only %d Go files checked; is the repository layout different?", files)
	}
	sort.Strings(found)
	for _, f := range found {
		t.Errorf("%s; use this package's Start, Run, Output or CombinedOutput", f)
	}
}
