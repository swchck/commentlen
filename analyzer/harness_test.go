package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// runSrc analyzes one in-memory file and returns the diagnostics as
// "line:col: message" strings.
//
// analysistest is deliberately not used: its `// want "…"` annotations live in
// comments, and a linter that measures comments would measure the annotations
// too.
func runSrc(t *testing.T, s Settings, filename, src string) []string {
	t.Helper()

	a, err := New(s)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return runAnalyzer(t, a, filename, src)
}

func runAnalyzer(t *testing.T, a *analysis.Analyzer, filename, src string) []string {
	t.Helper()
	return runAnalyzerWith(t, a, filename, src, true)
}

// runAnalyzerWithoutSource simulates a runner whose ReadFile fails.
func runAnalyzerWithoutSource(t *testing.T, a *analysis.Analyzer, filename, src string) []string {
	t.Helper()
	return runAnalyzerWith(t, a, filename, src, false)
}

func runAnalyzerWith(t *testing.T, a *analysis.Analyzer, filename, src string, readable bool) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}

	var got []string
	pass := &analysis.Pass{
		Analyzer: a,
		Fset:     fset,
		Files:    []*ast.File{file},
		ReadFile: func(name string) ([]byte, error) {
			if !readable {
				return nil, fmt.Errorf("simulated read failure for %s", name)
			}
			if name != filename {
				return nil, fmt.Errorf("unexpected read of %s", name)
			}
			return []byte(src), nil
		},
		ResultOf: map[*analysis.Analyzer]any{},
		Report: func(d analysis.Diagnostic) {
			p := fset.Position(d.Pos)
			got = append(got, fmt.Sprintf("%d:%d: %s", p.Line, p.Column, d.Message))
		},
	}
	if _, err := a.Run(pass); err != nil {
		t.Fatalf("run: %v", err)
	}
	return got
}

// assertDiags compares diagnostics against substrings, in order. An empty want
// asserts a clean file.
func assertDiags(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d diagnostics, want %d:\ngot:  %s\nwant: %s",
			len(got), len(want), strings.Join(got, "\n      "), strings.Join(want, "\n      "))
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("diagnostic %d:\n got %q\nwant substring %q", i, got[i], w)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// noStyle turns off the content checks so that a size test is not polluted by
// them, and vice versa.
func noStyle() StyleSettings {
	return StyleSettings{
		TagsEnabled:        ptr(false),
		UseDefaultPatterns: ptr(false),
		Banners:            ptr(false),
		Metadata:           ptr(false),
		CommentedCode:      ptr(false),
	}
}

// sizeOnly is a config with one measured kind and nothing else enabled.
func sizeOnly(kind Kind, ks KindSettings) Settings {
	return Settings{
		Defaults: KindSettings{Disabled: ptr(true)},
		Kinds:    map[string]KindSettings{kind.String(): withEnabled(ks)},
		Style:    noStyle(),
	}
}

func withEnabled(ks KindSettings) KindSettings {
	ks.Disabled = ptr(false)
	return ks
}
