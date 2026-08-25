package analyzer

import "testing"

func godocOnly(scope string) Settings {
	return Settings{
		Defaults: KindSettings{MaxLines: ptr(0), MaxWidth: ptr(0)},
		Style:    noStyle(),
		Godoc:    GodocSettings{Enabled: ptr(true), Scope: scope},
	}
}

func TestGodoc(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		src   string
		want  []string
	}{
		{
			name: "conventional doc comments pass",
			src: `// Package p does things.
package p

// F returns nothing.
func F() {}

// Buffer holds bytes.
type Buffer struct {
	// Len is the length in bytes.
	Len int
}

// A Writer writes.
type Writer struct{}

// The Reader reads.
type Reader struct{}

// Enabled reports whether the feature is on.
func Enabled() bool { return true }

// Quote(s) returns a quoted string.
func Quote(s string) string { return s }
`,
		},
		{
			name: "wrong opening symbol",
			src: `package p

// This function returns nothing.
func F() {}
`,
			want: []string{`3:1: doc comment should start with "F"`},
		},
		{
			name: "missing package prefix",
			src: `// A tiny helper package.
package p
`,
			want: []string{`1:1: doc comment should start with "Package p"`},
		},
		{
			name: "missing final period",
			src: `package p

// F returns nothing
func F() {}
`,
			want: []string{"3:1: doc comment should end with a period"},
		},
		{
			name: "colon before a code block is a valid ending",
			src: `package p

// F does a thing. Usage:
//
//	F()
func F() {}
`,
		},
		{
			name: "lowercase start on an exported symbol",
			src: `package p

// the value of x.
var X = 1
`,
			want: []string{
				`3:1: doc comment should start with "X"`,
				"3:1: doc comment should start with a capital letter",
			},
		},
		{
			name: "unexported symbol keeps its lowercase name",
			src: `package p

// f returns nothing.
func f() {}
`,
			scope: "all",
		},
		{
			name:  "unexported checked only in all scope",
			scope: "all",
			src: `package p

// does nothing.
func f() {}
`,
			want: []string{`3:1: doc comment should start with "f"`},
		},
		{
			name: "unexported ignored in exported scope",
			src: `package p

// does nothing
func f() {}
`,
		},
		{
			name: "returns true if on a bool function",
			src: `package p

// Ready returns true if the thing is ready.
func Ready() bool { return true }
`,
			want: []string{`3:1: "returns true if" on a bool-returning function`},
		},
		{
			name: "group doc has no symbol to open with",
			src: `package p

// The knobs of the package.
var (
	A = 1
	B = 2
)
`,
		},
		{
			name: "trailing field comment is exempt from grammar",
			src: `package p

// Config holds the knobs.
type Config struct {
	Timeout int // milliseconds
}
`,
			scope: "all",
		},
		{
			name: "directive-only doc is exempt",
			src: `package p

//go:generate stringer -type=Kind
func F() {}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertDiags(t, runSrc(t, godocOnly(tt.scope), "p.go", tt.src), tt.want)
		})
	}
}

func TestGodocSelectiveRules(t *testing.T) {
	src := `package p

// this returns nothing
func F() {}
`
	s := godocOnly("")
	s.Godoc.StartsWithName = ptr(false)
	s.Godoc.Capitalized = ptr(false)
	assertDiags(t, runSrc(t, s, "p.go", src), []string{"3:1: doc comment should end with a period"})
}
