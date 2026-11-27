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
	Doc:  "reports comments that are not directives, why: comments, or name-first docblocks on exported declarations of public packages",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	public := !isInternal(pass.Pkg.Path())
	for _, file := range pass.Files {
		docs := map[*ast.CommentGroup]bool{}
		if public && !strings.HasSuffix(pass.Fset.File(file.Pos()).Name(), "_test.go") {
			docs = docGroups(file)
		}
		for _, group := range file.Comments {
			if docs[group] || allowed(group) {
				continue
			}
			pass.Reportf(group.Pos(), "comment must be a directive, start with %q, or be a docblock on an exported declaration of a public package that starts with its name", reasonPrefix)
		}
	}
	return nil, nil
}

func isInternal(path string) bool {
	return path == "internal" || strings.HasPrefix(path, "internal/") ||
		strings.HasSuffix(path, "/internal") || strings.Contains(path, "/internal/")
}

func docGroups(file *ast.File) map[*ast.CommentGroup]bool {
	docs := map[*ast.CommentGroup]bool{}
	add := func(g *ast.CommentGroup, names ...*ast.Ident) {
		if g == nil {
			return
		}
		first := firstWord(g)
		for _, n := range names {
			if n != nil && n.IsExported() && n.Name == first {
				docs[g] = true
			}
		}
	}
	if file.Doc != nil && firstWord(file.Doc) == "Package" {
		docs[file.Doc] = true
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.GenDecl:
			var names []*ast.Ident
			for _, spec := range d.Specs {
				switch sp := spec.(type) {
				case *ast.TypeSpec:
					names = append(names, sp.Name)
				case *ast.ValueSpec:
					names = append(names, sp.Names...)
				}
			}
			add(d.Doc, names...)
		case *ast.FuncDecl:
			add(d.Doc, d.Name)
		case *ast.TypeSpec:
			add(d.Doc, d.Name)
		case *ast.ValueSpec:
			add(d.Doc, d.Names...)
		case *ast.Field:
			add(d.Doc, d.Names...)
		}
		return true
	})
	return docs
}

func firstWord(g *ast.CommentGroup) string {
	words := strings.Fields(g.Text())
	if len(words) == 0 {
		return ""
	}
	return words[0]
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
