package analyzer

import (
	"strings"
	"testing"
)

// longAndRussian trips both a size limit and a content pattern, so a test can
// tell which of the two an override kept.
const longAndRussian = `package p

// one
// two
// three
func f() {}

// F делает вещь.
func F() {}
`

func TestOverrideRelaxesSizeButKeepsStyle(t *testing.T) {
	// the shape the strict profile needs in a real project: test prose may be
	// long, but a comment in the wrong language is unwelcome anywhere
	s := Settings{
		Kinds: map[string]KindSettings{
			"unexported": {MaxLines: ptr(2)},
			"exported":   {MaxLines: ptr(2)},
		},
		Style: StyleSettings{
			TagsEnabled:        ptr(false),
			UseDefaultPatterns: ptr(false),
			Banners:            ptr(false),
			Metadata:           ptr(false),
			CommentedCode:      ptr(false),
			PatternsExtra:      []BannedPattern{{Pattern: `[А-Яа-яЁё]`, Message: "comments are in English, always"}},
		},
		Overrides: []Override{{
			Path: `_test\.go$`,
			Settings: Settings{
				Defaults: KindSettings{MaxLines: ptr(0)}, // 0 lifts every length cap
			},
		}},
	}

	prod := runSrc(t, s, "p.go", longAndRussian)
	assertDiags(t, prod, []string{
		"5:1: unexported comment is 3 lines long, max 2",
		"8:1: comments are in English, always",
	})

	// in the test file only the length rule is gone
	assertDiags(t, runSrc(t, s, "p_test.go", longAndRussian),
		[]string{"8:1: comments are in English, always"})
}

func TestOverrideCanTighten(t *testing.T) {
	s := Settings{
		Defaults: KindSettings{MaxLines: ptr(0), MaxWidth: ptr(0)},
		Style:    noStyle(),
		Overrides: []Override{{
			Path:     `^internal/`,
			Settings: Settings{Kinds: map[string]KindSettings{"unexported": {MaxLines: ptr(1)}}},
		}},
	}
	if got := runSrc(t, s, "cmd/app/main.go", longAndRussian); len(got) != 0 {
		t.Errorf("outside the override nothing applies, got %v", got)
	}
	assertDiags(t, runSrc(t, s, "internal/svc/p.go", longAndRussian),
		[]string{"4:1: unexported comment is 3 lines long, max 1"})
}

func TestOverrideFirstMatchWins(t *testing.T) {
	s := Settings{
		Defaults: KindSettings{MaxLines: ptr(9)},
		Style:    noStyle(),
		Overrides: []Override{
			{Path: `_test\.go$`, Settings: Settings{Defaults: KindSettings{MaxLines: ptr(0)}}},
			{Path: `^internal/`, Settings: Settings{Defaults: KindSettings{MaxLines: ptr(1)}}},
		},
	}
	// the file matches both; the first entry decides
	if got := runSrc(t, s, "internal/svc/p_test.go", longAndRussian); len(got) != 0 {
		t.Errorf("the first matching override should have lifted the cap, got %v", got)
	}
	if got := runSrc(t, s, "internal/svc/p.go", longAndRussian); len(got) == 0 {
		t.Error("the second override should still apply to non-test files")
	}
}

func TestOverrideInheritsWhatItDoesNotSet(t *testing.T) {
	s := Settings{
		Preset:    "strict",
		Overrides: []Override{{Path: `_test\.go$`, Settings: Settings{Ratio: RatioSettings{Enabled: ptr(false)}}}},
	}
	cfg, err := newConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.overrides) != 1 {
		t.Fatalf("got %d overrides, want 1", len(cfg.overrides))
	}
	child := cfg.overrides[0].cfg
	if child.ratio.enabled {
		t.Error("the override should have switched the ratio check off")
	}
	if !child.kinds[KindTrailing].forbidden {
		t.Error("the override should inherit the strict preset's forbidden trailing kind")
	}
	if child.needsStmts {
		t.Error("with the ratio off the override must not collect statement spans")
	}
	if !cfg.ratio.enabled || !cfg.needsStmts {
		t.Error("the base config must be left alone")
	}
	if len(child.style.patterns) != len(cfg.style.patterns) {
		t.Error("the override should inherit the base content patterns")
	}
}

func TestOverrideValidation(t *testing.T) {
	tests := []struct {
		name    string
		ov      Override
		wantErr string
	}{
		{
			name:    "missing path",
			ov:      Override{},
			wantErr: "overrides[0]: path is required",
		},
		{
			name:    "bad regexp",
			ov:      Override{Path: "("},
			wantErr: "overrides[0].path: bad regexp",
		},
		{
			name:    "nested overrides",
			ov:      Override{Path: "x", Settings: Settings{Overrides: []Override{{Path: "y"}}}},
			wantErr: "overrides cannot nest",
		},
		{
			name:    "preset inside an override",
			ov:      Override{Path: "x", Settings: Settings{Preset: "loose"}},
			wantErr: "preset belongs at the top level",
		},
		{
			name:    "file selection inside an override",
			ov:      Override{Path: "x", Settings: Settings{SkipTests: ptr(true)}},
			wantErr: "file selection",
		},
		{
			name:    "exclude-files inside an override",
			ov:      Override{Path: "x", Settings: Settings{ExcludeFiles: []string{"y"}}},
			wantErr: "file selection",
		},
		{
			name:    "errors name the override",
			ov:      Override{Path: "x", Settings: Settings{Kinds: map[string]KindSettings{"doc": {}}}},
			wantErr: `overrides[0].kinds: unknown kind "doc"`,
		},
		{
			name:    "bad pattern inside an override",
			ov:      Override{Path: "x", Settings: Settings{Style: StyleSettings{PatternsExtra: []BannedPattern{{Pattern: "("}}}}},
			wantErr: "overrides[0].style.patterns: bad regexp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(Settings{Overrides: []Override{tt.ov}})
			if err == nil {
				t.Fatalf("want an error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestGodocScopeValidation(t *testing.T) {
	if _, err := New(Settings{Godoc: GodocSettings{Scope: "everything"}}); err == nil {
		t.Error("an unknown godoc scope should be rejected")
	} else if !strings.Contains(err.Error(), "unknown scope") {
		t.Errorf("got %q, want it to mention the unknown scope", err)
	}
}
