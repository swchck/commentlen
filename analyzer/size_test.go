package analyzer

import "testing"

func TestMaxLines(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		settings KindSettings
		src      string
		want     []string
	}{
		{
			name:     "inline over budget",
			kind:     KindInline,
			settings: KindSettings{MaxLines: ptr(2)},
			src: `package p

func f() {
	// one
	// two
	// three
	do()
}
`,
			want: []string{"6:2: inline comment is 3 lines long, max 2"},
		},
		{
			name:     "inline at budget",
			kind:     KindInline,
			settings: KindSettings{MaxLines: ptr(2)},
			src: `package p

func f() {
	// one
	// two
	do()
}
`,
		},
		{
			name:     "blank comment lines do not count",
			kind:     KindExported,
			settings: KindSettings{MaxLines: ptr(2)},
			src: `package p

// One.
//
// Two.
func F() {}
`,
		},
		{
			name:     "block comment counts its physical lines",
			kind:     KindInline,
			settings: KindSettings{MaxLines: ptr(2)},
			src: `package p

func f() {
	/* one
	two
	three */
	do()
}
`,
			want: []string{"6:1: inline comment is 3 lines long, max 2"},
		},
		{
			// four prose-looking lines, but the example is a code block and the
			// separators are blank: two counted lines, inside the budget
			name:     "godoc code block is not counted",
			kind:     KindExported,
			settings: KindSettings{MaxLines: ptr(2)},
			src: `package p

// F does a thing.
//
// Usage:
//
//	F()
//	F()
func F() {}
`,
		},
		{
			name:     "zero means unlimited",
			kind:     KindPackage,
			settings: KindSettings{MaxLines: ptr(0)},
			src: `// Package p is long.
//
// One.
// Two.
// Three.
// Four.
package p
`,
		},
		{
			name:     "unexported doc kind",
			kind:     KindUnexported,
			settings: KindSettings{MaxLines: ptr(1)},
			src: `package p

// one
// two
func f() {}
`,
			want: []string{"4:1: unexported comment is 2 lines long, max 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runSrc(t, sizeOnly(tt.kind, tt.settings), "p.go", tt.src)
			assertDiags(t, got, tt.want)
		})
	}
}

func TestMaxWidth(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		settings KindSettings
		global   func(*Settings)
		src      string
		want     []string
	}{
		{
			name:     "width counts runes not bytes",
			kind:     KindInline,
			settings: KindSettings{MaxWidth: ptr(20)},
			src: `package p

func f() {
	// ` + "ααααααααααααααααααααααα" + `
	do()
}
`,
			want: []string{"4:2: inline comment line is 27 columns wide, max 20"},
		},
		{
			name:     "indentation counts towards width",
			kind:     KindInline,
			settings: KindSettings{MaxWidth: ptr(12)},
			src: `package p

func f() {
	if true {
		// twelve!!
		do()
	}
}
`,
			want: []string{"5:3: inline comment line is 13 columns wide, max 12"},
		},
		{
			name:     "indentation excluded on request",
			kind:     KindInline,
			settings: KindSettings{MaxWidth: ptr(12)},
			global:   func(s *Settings) { s.WidthIncludesIndent = ptr(false) },
			src: `package p

func f() {
	if true {
		// twelve!!
		do()
	}
}
`,
		},
		{
			name:     "url is allowed to overflow",
			kind:     KindExported,
			settings: KindSettings{MaxWidth: ptr(40)},
			src: `package p

// F implements https://www.rfc-editor.org/rfc/rfc9110.html#name-methods
func F() {}
`,
		},
		{
			name:     "prose around a url is not",
			kind:     KindExported,
			settings: KindSettings{MaxWidth: ptr(40)},
			src: `package p

// F implements the whole specification of https://example.com/a and more text
func F() {}
`,
			want: []string{"3:1: exported comment line is 78 columns wide, max 40"},
		},
		{
			name:     "each overlong line is reported",
			kind:     KindExported,
			settings: KindSettings{MaxWidth: ptr(10)},
			src: `package p

// aaaaaaaaaaaa
// bbbbbbbbbbbb
func F() {}
`,
			want: []string{
				"3:1: exported comment line is 15 columns wide, max 10",
				"4:1: exported comment line is 15 columns wide, max 10",
			},
		},
		{
			name:     "code block width is ignored",
			kind:     KindExported,
			settings: KindSettings{MaxWidth: ptr(20)},
			src: `package p

// F does a thing.
//
//	longCall(withAVeryLongArgument, andAnother)
func F() {}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := sizeOnly(tt.kind, tt.settings)
			if tt.global != nil {
				tt.global(&s)
			}
			got := runSrc(t, s, "p.go", tt.src)
			assertDiags(t, got, tt.want)
		})
	}
}

func TestForbiddenKind(t *testing.T) {
	s := sizeOnly(KindTrailing, KindSettings{Forbidden: ptr(true)})
	src := `package p

func f() {
	do() // why not
	// this one is fine
}
`
	assertDiags(t, runSrc(t, s, "p.go", src), []string{"4:7: trailing comments are not allowed"})
}
