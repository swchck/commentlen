package analyzer

import (
	"strings"
	"testing"
)

func TestSettingsValidation(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		wantErr  string
	}{
		{
			name:     "unknown preset",
			settings: Settings{Preset: "pedantic"},
			wantErr:  `unknown preset "pedantic"`,
		},
		{
			name:     "unknown kind",
			settings: Settings{Kinds: map[string]KindSettings{"doc": {}}},
			wantErr:  `unknown kind "doc"`,
		},
		{
			name:     "negative limit",
			settings: Settings{Kinds: map[string]KindSettings{"inline": {MaxLines: ptr(-1)}}},
			wantErr:  "must not be negative",
		},
		{
			name:     "bad exclude regexp",
			settings: Settings{ExcludeFiles: []string{"("}},
			wantErr:  "exclude-files: bad regexp",
		},
		{
			name:     "bad comment pattern",
			settings: Settings{ExcludePatterns: []string{"[a-"}},
			wantErr:  "exclude-patterns: bad regexp",
		},
		{
			name:     "bad style pattern",
			settings: Settings{Style: StyleSettings{PatternsExtra: []BannedPattern{{Pattern: "*"}}}},
			wantErr:  "style.patterns: bad regexp",
		},
		{
			name:     "empty style pattern",
			settings: Settings{Style: StyleSettings{PatternsExtra: []BannedPattern{{Message: "x"}}}},
			wantErr:  "style.patterns: empty pattern",
		},
		{
			name:     "unknown ratio kind",
			settings: Settings{Ratio: RatioSettings{Kinds: []string{"docs"}}},
			wantErr:  `ratio.kinds: unknown kind "docs"`,
		},
		{
			name:     "non-positive ratio",
			settings: Settings{Ratio: RatioSettings{Max: ptr(0.0)}},
			wantErr:  "ratio.max must be positive",
		},
		{
			name:     "negative inline budget",
			settings: Settings{MaxInlinePerFunc: ptr(-2)},
			wantErr:  "max-inline-per-func must not be negative",
		},
		{
			name:     "bad generated pattern",
			settings: Settings{GeneratedExtra: []string{"(?P<"}},
			wantErr:  "generated-extra: bad regexp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.settings)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestPresetLayering(t *testing.T) {
	strict, err := newConfig(Settings{Preset: "strict"})
	if err != nil {
		t.Fatal(err)
	}
	if !strict.kinds[KindTrailing].forbidden {
		t.Error("strict preset should forbid trailing comments")
	}
	if got := strict.kinds[KindUnexported].maxLines; got != 2 {
		t.Errorf("strict unexported max-lines = %d, want 2", got)
	}
	if !strict.ratio.enabled || !strict.godoc.enabled {
		t.Error("strict preset should enable the ratio and godoc checks")
	}

	// an explicit field wins over the preset, the rest of the preset stays
	custom, err := newConfig(Settings{
		Preset: "strict",
		Kinds:  map[string]KindSettings{"unexported": {MaxLines: ptr(4)}},
		Ratio:  RatioSettings{Enabled: ptr(false)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := custom.kinds[KindUnexported].maxLines; got != 4 {
		t.Errorf("override max-lines = %d, want 4", got)
	}
	if custom.ratio.enabled {
		t.Error("ratio.enabled:false should switch the check off")
	}
	if !custom.kinds[KindTrailing].forbidden {
		t.Error("an override must not reset the rest of the preset")
	}

	// defaults apply to every kind, then per-kind entries refine them
	defaults, err := newConfig(Settings{
		Defaults: KindSettings{MaxWidth: ptr(72)},
		Kinds:    map[string]KindSettings{"package": {MaxWidth: ptr(100)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := defaults.kinds[KindInline].maxWidth; got != 72 {
		t.Errorf("inline max-width = %d, want 72 from defaults", got)
	}
	if got := defaults.kinds[KindPackage].maxWidth; got != 100 {
		t.Errorf("package max-width = %d, want 100 from the kind entry", got)
	}

	loose, err := newConfig(Settings{Preset: "loose"})
	if err != nil {
		t.Fatal(err)
	}
	if loose.style.tags != nil || len(loose.style.patterns) != 0 {
		t.Error("loose preset should not enable the content checks")
	}
	if loose.needsStmts {
		t.Error("loose preset should not collect statements: the ratio check is off")
	}
}

func TestStatementsCollectedOnlyForRatio(t *testing.T) {
	off, err := newConfig(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if off.needsStmts {
		t.Error("balanced preset should not need statement spans")
	}
	on, err := newConfig(Settings{Ratio: RatioSettings{Enabled: ptr(true)}})
	if err != nil {
		t.Fatal(err)
	}
	if !on.needsStmts {
		t.Error("ratio check should request statement spans")
	}
}

func TestDefaultPresetIsQuietOnConventionalCode(t *testing.T) {
	src := `// Package p demonstrates ordinary Go comments that the default preset accepts.
package p

import "fmt"

// Config holds the knobs of the package.
type Config struct {
	// Timeout is the request budget.
	Timeout int // milliseconds
}

// New returns a Config with the defaults applied.
func New() *Config {
	// the zero value is a valid config, so nothing to fill in here
	return &Config{}
}

// String implements fmt.Stringer.
func (c *Config) String() string {
	return fmt.Sprintf("%d", c.Timeout)
}

//go:generate stringer -type=Kind
func helper() {
	do() // the driver is not goroutine-safe, so no errgroup here
}
`
	if got := runSrc(t, Settings{}, "p.go", src); len(got) != 0 {
		t.Errorf("default preset should accept conventional code, got %v", got)
	}
}
