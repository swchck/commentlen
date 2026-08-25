package analyzer

import (
	"strings"
	"testing"
)

// TestEdgeCases feeds the analyzer the shapes that make comment linters panic:
// no trailing newline, CRLF, empty block comments, comments in odd syntactic
// positions. Nothing here should crash, and the diagnostics must stay sane.
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "no trailing newline",
			src:  "package p\n\nfunc f() {\n\t// a note\n}\n\n// dangling comment",
		},
		{
			name: "crlf line endings",
			src:  "package p\r\n\r\n// F is exported.\r\nfunc F() {\r\n\t// a note\r\n\tdo() // trailing\r\n}\r\n",
		},
		{
			name: "empty block comment",
			src:  "package p\n\n/**/\nfunc f() { /**/ }\n",
		},
		{
			name: "single line block comment",
			src:  "package p\n\n/* one liner */\nfunc f() { /* inner */ }\n",
		},
		{
			name: "star continuation style",
			src:  "package p\n\n/*\n * F is exported.\n * Second line.\n */\nfunc F() {}\n",
		},
		{
			name: "comment as the whole file",
			src:  "// Package p is only a doc.\npackage p\n",
		},
		{
			name: "comments in every odd position",
			src: `package p

import (
	// grouped import doc
	"fmt" // trailing
)

func f(
	a int, // parameter note
	b int,
) (
	// result note
	int,
) {
	m := map[string]int{
		// inside a composite literal
		"a": 1, // element note
	}
	switch a {
	// before a case
	case 1:
		// inside a case
		fmt.Println(m)
	default:
		// inside default
	}
	select {
	// inside select
	default:
	}
	go func() {
		// inside a literal
	}()
	return b
}

type (
	// grouped type doc
	a struct{}
	b interface {
		// interface method doc
		M() // trailing
	}
)

const (
	// grouped const doc
	c = 1 // trailing
)
`,
		},
		{
			name: "generic receiver and type parameters",
			src: `package p

// Box holds a value.
type Box[T any] struct {
	// v is the value.
	v T
}

// Get returns the value.
func (b Box[T]) Get() T { return b.v }

// unexportedMethod is on an exported generic type.
func (b *Box[T]) unexportedMethod() {}
`,
		},
		{
			name: "unterminated disable region",
			src:  "package p\n\n//commentlen:disable\n\n// anything goes from here to the end of the file\nfunc F() {}\n",
		},
		{
			name: "nested disable directives",
			src: `package p

//commentlen:disable
//commentlen:disable
// still off
//commentlen:enable
// on again
func F() {}
`,
		},
		{
			name: "unicode and emoji",
			src:  "package p\n\n// Ф возвращает результат 🎉 очень длинный комментарий на русском языке.\nfunc Ф() {}\n",
		},
		{
			name: "tabs inside the comment",
			src:  "package p\n\nfunc f() {\n\t//\t\tdeeply\tindented\tnote\n\tdo()\n}\n",
		},
		{
			name: "only a build directive",
			src:  "//go:build linux\n\npackage p\n",
		},
	}

	for _, tt := range tests {
		for _, preset := range []string{"balanced", "strict", "loose"} {
			t.Run(tt.name+"/"+preset, func(t *testing.T) {
				s := Settings{Preset: preset, Godoc: GodocSettings{Enabled: ptr(true), Scope: "all"}}
				// the assertion is "it returns"; a panic or an error fails the test
				for _, d := range runSrc(t, s, "p.go", tt.src) {
					if strings.Contains(d, "0:0") {
						t.Errorf("diagnostic without a position: %s", d)
					}
				}
			})
		}
	}
}

func TestEmptyAndCommentlessFiles(t *testing.T) {
	for _, src := range []string{"package p\n", "package p\n\nfunc f() {}\n"} {
		if got := runSrc(t, Settings{Preset: "strict"}, "p.go", src); len(got) != 0 {
			t.Errorf("expected no diagnostics for %q, got %v", src, got)
		}
	}
}

// TestUnreadableFileIsSkipped covers the runner that cannot hand us the source:
// the linter must stay quiet rather than report nonsense positions.
func TestUnreadableFileIsSkipped(t *testing.T) {
	a, err := New(Settings{Preset: "strict"})
	if err != nil {
		t.Fatal(err)
	}
	got := runAnalyzerWithoutSource(t, a, "missing.go", "package p\n\n// a very long comment line that would normally trip the width limit\nfunc F() {}\n")
	if len(got) != 0 {
		t.Errorf("expected no diagnostics when the source cannot be read, got %v", got)
	}
}
