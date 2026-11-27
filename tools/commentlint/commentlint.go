package commentlint

import (
	"go/ast"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

const reasonPrefix = "why:"

var directivePrefixes = []string{"//go:", "//nolint", "//lint:", "//line ", "//export "}

func init() {
	register.Plugin("commentlint", New)
}

func New(_ any) (register.LinterPlugin, error) {
	return plugin{}, nil
}

type plugin struct{}

func (plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{Analyzer}, nil
}

func (plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

var Analyzer = &analysis.Analyzer{
	Name: "commentlint",
	Doc:  "reports comments that are not docblocks, tool directives, or marked with why:",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		docs := docGroups(file)
		for _, group := range file.Comments {
			if docs[group] || allowed(group) {
				continue
			}
			pass.Reportf(group.Pos(), "comment must be a docblock, a tool directive, or start with %q", reasonPrefix)
		}
	}
	return nil, nil
}

func docGroups(file *ast.File) map[*ast.CommentGroup]bool {
	docs := map[*ast.CommentGroup]bool{}
	add := func(g *ast.CommentGroup) {
		if g != nil {
			docs[g] = true
		}
	}
	add(file.Doc)
	ast.Inspect(file, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.GenDecl:
			add(d.Doc)
		case *ast.FuncDecl:
			add(d.Doc)
		case *ast.TypeSpec:
			add(d.Doc)
		case *ast.ValueSpec:
			add(d.Doc)
		case *ast.Field:
			add(d.Doc)
		}
		return true
	})
	return docs
}

func allowed(group *ast.CommentGroup) bool {
	for _, c := range group.List {
		if isDirective(c.Text) {
			continue
		}
		return hasReason(group)
	}
	return true
}

func hasReason(group *ast.CommentGroup) bool {
	for _, c := range group.List {
		if isDirective(c.Text) {
			continue
		}
		text := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(c.Text, "//"), "/*"))
		return strings.HasPrefix(text, reasonPrefix)
	}
	return false
}

func isDirective(text string) bool {
	for _, p := range directivePrefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}
