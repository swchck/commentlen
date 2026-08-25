package analyzer

import "testing"

func ratioSettings() Settings {
	return Settings{
		Defaults: KindSettings{Disabled: ptr(true)},
		Kinds:    map[string]KindSettings{"inline": {Disabled: ptr(false)}},
		Ratio:    RatioSettings{Enabled: ptr(true), Kinds: []string{"inline"}},
		Style:    noStyle(),
	}
}

func TestRatio(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "comment longer than the block it describes",
			src: `package p

func f() {
	// one
	// two
	// three
	a := 1
	b := 2
}
`,
			want: []string{"4:2: inline comment is 3 lines for 2 lines of code"},
		},
		{
			name: "equal length is allowed",
			src: `package p

func f() {
	// one
	// two
	// three
	a := 1
	b := 2
	c := 3
}
`,
		},
		{
			name: "code long enough carries the comment",
			src: `package p

func f() {
	// one
	// two
	a := 1
	b := 2
	c := 3
	_, _, _ = a, b, c
}
`,
		},
		{
			name: "short code is exempt",
			src: `package p

func f() {
	// one
	// two
	a := 1
	_ = a
}
`,
		},
		{
			name: "a blank line ends the described block",
			src: `package p

func f() {
	// one
	// two
	// three
	a := 1
	b := 2

	c := 3
	d := 4
	_, _, _, _ = a, b, c, d
}
`,
			want: []string{"4:2: inline comment is 3 lines for 2 lines of code"},
		},
		{
			name: "the next comment ends the described block",
			src: `package p

func f() {
	// one
	// two
	// three
	a := 1
	b := 2
	// another note
	c := 3
	d := 4
}
`,
			want: []string{"4:2: inline comment is 3 lines for 2 lines of code"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertDiags(t, runSrc(t, ratioSettings(), "p.go", tt.src), tt.want)
		})
	}
}

func TestRatioOnDocComments(t *testing.T) {
	s := Settings{
		Defaults: KindSettings{Disabled: ptr(true)},
		Kinds:    map[string]KindSettings{"unexported": {Disabled: ptr(false)}},
		Ratio:    RatioSettings{Enabled: ptr(true), Kinds: []string{"unexported"}, MinCodeLines: ptr(1)},
		Style:    noStyle(),
	}
	src := `package p

// one
// two
// three
func f() { do() }
`
	assertDiags(t, runSrc(t, s, "p.go", src),
		[]string{"3:1: unexported comment is 3 lines for 1 line of code"})
}

func TestInlineBudget(t *testing.T) {
	s := Settings{
		Defaults:         KindSettings{Disabled: ptr(true)},
		Kinds:            map[string]KindSettings{"inline": {Disabled: ptr(false)}},
		MaxInlinePerFunc: ptr(2),
		Style:            noStyle(),
	}
	src := `package p

func tooMany() {
	// one
	a := 1
	// two
	b := 2
	// three
	_, _ = a, b
}

func fine() {
	// one
	a := 1
	// two
	_ = a
}

func withLiteral() {
	// one
	f := func() {
		// two
		a := 1
		// three
		b := 2
		// four
		_, _ = a, b
	}
	f()
}
`
	// consecutive comment lines are one comment; the budget counts comments,
	// not lines
	assertDiags(t, runSrc(t, s, "p.go", src), []string{
		"3:16: func tooMany has 3 inline comments, max 2",
		"21:14: function literal has 3 inline comments, max 2",
	})
}

func TestStyleChecks(t *testing.T) {
	tests := []struct {
		name     string
		settings func(*Settings)
		src      string
		want     []string
	}{
		{
			name: "banned tags",
			src: `package p

// TODO: fix this later
func F() {}
`,
			want: []string{"3:1: TODO marker left in a comment"},
		},
		{
			name: "tag word inside prose is not a tag",
			src: `package p

// F updates the todos of a user.
func F() {}
`,
		},
		{
			name: "hedging preamble",
			src: `package p

// F does a thing. Note that it may block.
func F() {}
`,
			want: []string{"3:1: hedging preamble adds no information"},
		},
		{
			name: "not just x but y",
			src: `package p

// F handles not just the header, but the body too.
func F() {}
`,
			want: []string{`the "not just X, but Y" construction says nothing concrete`},
		},
		{
			name: "vague abstraction",
			src: `package p

func f() {
	// cached for performance reasons
	do()
}
`,
			want: []string{"vague abstraction: name the actual number, error or system"},
		},
		{
			name: "decorative separator",
			src: `package p

// ==============================
// F does a thing.
func F() {}
`,
			want: []string{"3:1: decorative separator in a comment"},
		},
		{
			name: "authorship metadata",
			src: `package p

// F does a thing.
// @author someone
func F() {}
`,
			want: []string{"3:1: authorship or change-history metadata in a comment"},
		},
		{
			name: "copyright is not metadata",
			src: `// Copyright (c) 2026 Someone. All rights reserved.

package p
`,
		},
		{
			name: "commented-out code",
			src: `package p

func f() {
	// if err != nil {
	// 	return err
	// }
	do()
}
`,
			want: []string{"4:2: commented-out code (3 lines)"},
		},
		{
			name: "prose mentioning code is not commented-out code",
			src: `package p

// F returns the value of x (see RFC 9110) and never blocks.
func F() {}
`,
		},
		{
			name: "one code line stays quiet by default",
			src: `package p

func f() {
	// do(x)
	do(y)
}
`,
		},
		{
			name:     "custom banned pattern",
			settings: func(s *Settings) { s.Style.PatternsExtra = []BannedPattern{{Pattern: `(?i)obviously`, Message: "no"}} },
			src: `package p

// F obviously works.
func F() {}
`,
			want: []string{"3:1: no"},
		},
		{
			name:     "custom tag list",
			settings: func(s *Settings) { s.Style.Tags = []string{"WIP"} },
			src: `package p

// TODO: still fine
// WIP: not fine
func F() {}
`,
			want: []string{"3:1: WIP marker left in a comment"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Settings{Defaults: KindSettings{MaxLines: ptr(0), MaxWidth: ptr(0)}}
			if tt.settings != nil {
				tt.settings(&s)
			}
			assertDiags(t, runSrc(t, s, "p.go", tt.src), tt.want)
		})
	}
}
