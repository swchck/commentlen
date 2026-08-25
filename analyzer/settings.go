package analyzer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Settings is the user-facing configuration of the linter. It is decoded from
// the `linters.settings.custom.commentlen.settings` block of .golangci.yml, so
// every field name here is a YAML key.
//
// All scalar fields are pointers: a nil pointer means "not set by the user" and
// falls back to the preset. Slices replace the preset value; the *-extra slices
// append to it instead.
type Settings struct {
	// Preset picks the baseline every other field is layered on top of:
	// "balanced" (default), "strict" or "loose".
	Preset string `json:"preset"`

	// SkipGenerated skips files carrying a "Code generated ... DO NOT EDIT."
	// marker above the package clause. Default true.
	SkipGenerated *bool `json:"skip-generated"`
	// GeneratedExtra are additional regexps matched against the comments above
	// the package clause; a match marks the file as generated.
	GeneratedExtra []string `json:"generated-extra"`
	// SkipTests skips _test.go files entirely. Default false.
	SkipTests *bool `json:"skip-tests"`
	// ExcludeFiles are regexps matched against the file path (slash-separated).
	ExcludeFiles []string `json:"exclude-files"`

	// ExcludePatterns are regexps matched against a comment's text; a match
	// exempts that comment from every check.
	ExcludePatterns []string `json:"exclude-patterns"`
	// IgnoreDirectives exempts tool directives (//go:generate, //nolint, …).
	// Default true.
	IgnoreDirectives *bool `json:"ignore-directives"`
	// DirectivePrefixesExtra adds prefixes treated as directives, for tools that
	// do not follow the //tool:name convention (e.g. "ffjson:").
	DirectivePrefixesExtra []string `json:"directive-prefixes-extra"`
	// IgnoreURLs excludes lines whose overlong part is a single URL or a long
	// unbreakable token from the width check. Default true.
	IgnoreURLs *bool `json:"ignore-urls"`
	// IgnoreCodeBlocks excludes godoc code blocks (indented or ``` fenced) from
	// both the width and the line-count checks. Default true.
	IgnoreCodeBlocks *bool `json:"ignore-code-blocks"`
	// CountBlankLines counts empty comment lines towards max-lines.
	// Default false.
	CountBlankLines *bool `json:"count-blank-lines"`
	// WidthIncludesIndent measures width from column 1 rather than from the
	// comment marker. Default true.
	WidthIncludesIndent *bool `json:"width-includes-indent"`

	// Defaults applies to every kind that has no explicit entry in Kinds.
	Defaults KindSettings `json:"defaults"`
	// Kinds holds the per-kind limits, keyed by kind name: package, exported,
	// unexported, field, inline, trailing, other.
	Kinds map[string]KindSettings `json:"kinds"`

	// MaxInlinePerFunc caps how many inline comments one function body may hold.
	// 0 disables the check.
	MaxInlinePerFunc *int `json:"max-inline-per-func"`

	Ratio RatioSettings `json:"ratio"`
	Style StyleSettings `json:"style"`
	Godoc GodocSettings `json:"godoc"`
}

// KindSettings are the size limits of one comment kind.
type KindSettings struct {
	// Disabled turns off every check for this kind.
	Disabled *bool `json:"disabled"`
	// Forbidden reports any comment of this kind, whatever its size.
	Forbidden *bool `json:"forbidden"`
	// MaxLines caps the number of counted lines. 0 means unlimited.
	MaxLines *int `json:"max-lines"`
	// MaxWidth caps the width of a single line in runes. 0 means unlimited.
	MaxWidth *int `json:"max-width"`
}

// RatioSettings configures the "a comment must not be longer than the code it
// describes" rule.
type RatioSettings struct {
	Enabled *bool `json:"enabled"`
	// Max is the allowed comment-lines / code-lines ratio. Default 1.0.
	Max *float64 `json:"max"`
	// MinCodeLines skips the check when the described code is shorter than this,
	// so that a one-liner may still carry a two-line explanation. Default 2.
	MinCodeLines *int `json:"min-code-lines"`
	// Kinds lists the kinds the ratio applies to. Default ["inline"].
	Kinds []string `json:"kinds"`
}

// StyleSettings configures the content checks: banned tags, banned phrases,
// decorative banners, metadata and commented-out code.
type StyleSettings struct {
	//commentlen:ignore  the doc has to name the markers it bans
	// Tags reports these words when used as a tag (TODO, FIXME, …).
	Tags        []string `json:"tags"`
	TagsEnabled *bool    `json:"tags-enabled"`

	// Patterns replaces the default banned-phrase list.
	Patterns []BannedPattern `json:"patterns"`
	// PatternsExtra appends to the default banned-phrase list.
	PatternsExtra []BannedPattern `json:"patterns-extra"`
	// UseDefaultPatterns keeps the built-in phrase list. Default true.
	UseDefaultPatterns *bool `json:"use-default-patterns"`

	// Banners reports decorative separators and ASCII art.
	Banners *bool `json:"banners"`
	//commentlen:ignore  the doc has to name what it bans
	// Metadata reports @author, dates and change-history lines.
	Metadata *bool `json:"metadata"`
	// CommentedCode reports commented-out code.
	CommentedCode *bool `json:"commented-code"`
	// CommentedCodeMinLines is how many consecutive code-looking lines are
	// needed before reporting. Default 2.
	CommentedCodeMinLines *int `json:"commented-code-min-lines"`
}

// BannedPattern is one forbidden phrase and the diagnostic it produces.
type BannedPattern struct {
	// Pattern is a Go regexp matched against the comment text.
	Pattern string `json:"pattern"`
	// Message is what the user sees. Defaults to naming the pattern.
	Message string `json:"message"`
}

// GodocSettings configures the grammar rules applied to doc comments.
type GodocSettings struct {
	Enabled *bool `json:"enabled"`
	// StartsWithName requires the Go convention "Name does ...".
	StartsWithName *bool `json:"starts-with-name"`
	// Capitalized requires an upper-case first letter.
	Capitalized *bool `json:"capitalized"`
	// EndsWithPeriod requires a terminating period on the last line.
	EndsWithPeriod *bool `json:"ends-with-period"`
	// ReportsWhether reports "returns true if" on bool-returning functions,
	// which the standard library spells "reports whether".
	ReportsWhether *bool `json:"reports-whether"`
	// Scope is "exported" (default) or "all".
	Scope string `json:"scope"`
}

type kindConfig struct {
	disabled  bool
	forbidden bool
	maxLines  int
	maxWidth  int
}

type ratioConfig struct {
	enabled      bool
	max          float64
	minCodeLines int
	kinds        [numKinds]bool
}

type bannedPattern struct {
	re      *regexp.Regexp
	pre     prefilter
	message string
}

type styleConfig struct {
	tags                  *regexp.Regexp
	tagsPre               prefilter
	patterns              []bannedPattern
	banners               bool
	metadata              bool
	commentedCode         bool
	commentedCodeMinLines int
}

type godocConfig struct {
	enabled        bool
	startsWithName bool
	capitalized    bool
	endsWithPeriod bool
	reportsWhether bool
	allSymbols     bool
}

// config is the validated, pre-compiled form of Settings: the hot path reads it
// without dereferencing pointers or compiling anything.
type config struct {
	skipGenerated       bool
	generatedExtra      []*regexp.Regexp
	skipTests           bool
	excludeFiles        []*regexp.Regexp
	excludeComments     []*regexp.Regexp
	ignoreDirectives    bool
	directivePrefixes   []string
	ignoreURLs          bool
	ignoreCodeBlocks    bool
	countBlankLines     bool
	widthIncludesIndent bool

	kinds            [numKinds]kindConfig
	maxInlinePerFunc int

	ratio ratioConfig
	style styleConfig
	godoc godocConfig

	needsStmts bool
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func floatOr(p *float64, def float64) float64 {
	if p == nil {
		return def
	}
	return *p
}

// defaultDirectivePrefixes covers directives that do not follow the
// //tool:name shape the generic detector recognizes.
var defaultDirectivePrefixes = []string{
	"+build",
	"go:",
	"nolint",
	"lint:",
	"noinspection",
	"nosec",
	"gocyclo:",
	"revive:",
	"staticcheck:",
	"deadcode:",
	"export ",
	"cgo_",
	"line ",
	"extern ",
	"sys ",
}

// defaultBannedPatterns are the machine-written-prose tells: hedging preambles,
// the "not just X, but Y" construction, and vague abstraction.
//
//commentlen:ignore  the doc has to quote the constructions it bans
var defaultBannedPatterns = []BannedPattern{
	{
		Pattern: `(?i)\b(note that|please note|it'?s (important|worth) (to note|noting)|as (a )?matter of fact|generally speaking|in many cases|keep in mind that)\b`,
		Message: "hedging preamble adds no information",
	},
	{
		Pattern: `(?i)\bnot just [^,.]{1,40}, but\b`,
		Message: `the "not just X, but Y" construction says nothing concrete`,
	},
	{
		Pattern: `(?i)\b(for performance reasons|to ensure correctness|to handle edge cases|for safety reasons|for clarity)\b`,
		Message: "vague abstraction: name the actual number, error or system",
	},
	{
		Pattern: `(?i)\bthis (function|method|struct|type|variable) (does|is|will|handles)\b`,
		Message: "restating the signature; say why instead of what",
	},
}

var defaultBannedTags = []string{"TODO", "FIXME", "HACK", "XXX", "OPTIMIZE", "BUG"}

type preset struct {
	kinds            [numKinds]kindConfig
	maxInlinePerFunc int
	ratioEnabled     bool
	ratioKinds       []string
	style            bool
	godoc            bool
	godocAll         bool
}

func presetByName(name string) (preset, error) {
	switch name {
	case "", "balanced":
		return preset{
			kinds: [numKinds]kindConfig{
				KindPackage:    {maxWidth: 100},
				KindExported:   {maxLines: 10, maxWidth: 100},
				KindUnexported: {maxLines: 6, maxWidth: 100},
				KindField:      {maxLines: 3, maxWidth: 100},
				KindInline:     {maxLines: 3, maxWidth: 100},
				KindTrailing:   {maxLines: 1, maxWidth: 80},
				KindOther:      {maxLines: 4, maxWidth: 100},
			},
			maxInlinePerFunc: 0,
			ratioEnabled:     false,
			ratioKinds:       []string{"inline"},
			style:            true,
			godoc:            false,
		}, nil

	case "strict":
		return preset{
			kinds: [numKinds]kindConfig{
				KindPackage:    {maxWidth: 100},
				KindExported:   {maxLines: 6, maxWidth: 100},
				KindUnexported: {maxLines: 2, maxWidth: 100},
				KindField:      {maxLines: 2, maxWidth: 100},
				KindInline:     {maxLines: 2, maxWidth: 100},
				KindTrailing:   {forbidden: true},
				KindOther:      {maxLines: 2, maxWidth: 100},
			},
			maxInlinePerFunc: 2,
			ratioEnabled:     true,
			ratioKinds:       []string{"inline", "unexported", "other"},
			style:            true,
			godoc:            true,
		}, nil

	case "loose":
		return preset{
			kinds: [numKinds]kindConfig{
				KindPackage:    {},
				KindExported:   {maxWidth: 120},
				KindUnexported: {maxWidth: 120},
				KindField:      {maxWidth: 120},
				KindInline:     {maxLines: 6, maxWidth: 120},
				KindTrailing:   {maxLines: 1, maxWidth: 120},
				KindOther:      {maxWidth: 120},
			},
			ratioKinds: []string{"inline"},
			style:      false,
			godoc:      false,
		}, nil

	default:
		return preset{}, fmt.Errorf("unknown preset %q: want balanced, strict or loose", name)
	}
}

func compileAll(patterns []string, what string) ([]*regexp.Regexp, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("%s: bad regexp %q: %w", what, p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// newConfig validates the settings and turns them into the form the analyzer
// runs on.
func newConfig(s Settings) (*config, error) {
	p, err := presetByName(s.Preset)
	if err != nil {
		return nil, err
	}

	cfg := &config{
		skipGenerated:       boolOr(s.SkipGenerated, true),
		skipTests:           boolOr(s.SkipTests, false),
		ignoreDirectives:    boolOr(s.IgnoreDirectives, true),
		ignoreURLs:          boolOr(s.IgnoreURLs, true),
		ignoreCodeBlocks:    boolOr(s.IgnoreCodeBlocks, true),
		countBlankLines:     boolOr(s.CountBlankLines, false),
		widthIncludesIndent: boolOr(s.WidthIncludesIndent, true),
		kinds:               p.kinds,
		maxInlinePerFunc:    intOr(s.MaxInlinePerFunc, p.maxInlinePerFunc),
	}

	if cfg.excludeFiles, err = compileAll(s.ExcludeFiles, "exclude-files"); err != nil {
		return nil, err
	}
	if cfg.excludeComments, err = compileAll(s.ExcludePatterns, "exclude-patterns"); err != nil {
		return nil, err
	}
	if cfg.generatedExtra, err = compileAll(s.GeneratedExtra, "generated-extra"); err != nil {
		return nil, err
	}

	cfg.directivePrefixes = defaultDirectivePrefixes
	if len(s.DirectivePrefixesExtra) > 0 {
		cfg.directivePrefixes = make([]string, 0, len(defaultDirectivePrefixes)+len(s.DirectivePrefixesExtra))
		cfg.directivePrefixes = append(cfg.directivePrefixes, defaultDirectivePrefixes...)
		cfg.directivePrefixes = append(cfg.directivePrefixes, s.DirectivePrefixesExtra...)
	}

	applyKind := func(k Kind, ks KindSettings) {
		c := &cfg.kinds[k]
		c.disabled = boolOr(ks.Disabled, c.disabled)
		c.forbidden = boolOr(ks.Forbidden, c.forbidden)
		c.maxLines = intOr(ks.MaxLines, c.maxLines)
		c.maxWidth = intOr(ks.MaxWidth, c.maxWidth)
	}
	for k := Kind(0); int(k) < numKinds; k++ {
		applyKind(k, s.Defaults)
	}
	for name, ks := range s.Kinds {
		k, ok := parseKind(name)
		if !ok {
			return nil, fmt.Errorf("kinds: unknown kind %q: want one of %s", name, strings.Join(KindNames(), ", "))
		}
		applyKind(k, ks)
	}
	for k := Kind(0); int(k) < numKinds; k++ {
		if c := cfg.kinds[k]; c.maxLines < 0 || c.maxWidth < 0 {
			return nil, fmt.Errorf("kinds.%s: max-lines and max-width must not be negative", k)
		}
	}
	if cfg.maxInlinePerFunc < 0 {
		return nil, fmt.Errorf("max-inline-per-func must not be negative")
	}

	if err := cfg.applyRatio(s.Ratio, p); err != nil {
		return nil, err
	}
	if err := cfg.applyStyle(s.Style, p); err != nil {
		return nil, err
	}
	cfg.applyGodoc(s.Godoc, p)

	cfg.needsStmts = cfg.ratio.enabled
	return cfg, nil
}

func (cfg *config) applyRatio(s RatioSettings, p preset) error {
	cfg.ratio.enabled = boolOr(s.Enabled, p.ratioEnabled)
	cfg.ratio.max = floatOr(s.Max, 1.0)
	cfg.ratio.minCodeLines = intOr(s.MinCodeLines, 2)
	if cfg.ratio.max <= 0 {
		return fmt.Errorf("ratio.max must be positive")
	}

	names := s.Kinds
	if names == nil {
		names = p.ratioKinds
	}
	for _, name := range names {
		k, ok := parseKind(name)
		if !ok {
			return fmt.Errorf("ratio.kinds: unknown kind %q: want one of %s", name, strings.Join(KindNames(), ", "))
		}
		cfg.ratio.kinds[k] = true
	}
	return nil
}

func (cfg *config) applyStyle(s StyleSettings, p preset) error {
	cfg.style.banners = boolOr(s.Banners, p.style)
	cfg.style.metadata = boolOr(s.Metadata, p.style)
	cfg.style.commentedCode = boolOr(s.CommentedCode, p.style)
	cfg.style.commentedCodeMinLines = intOr(s.CommentedCodeMinLines, 2)
	if cfg.style.commentedCodeMinLines < 1 {
		return fmt.Errorf("style.commented-code-min-lines must be at least 1")
	}

	if boolOr(s.TagsEnabled, p.style) {
		tags := s.Tags
		if tags == nil {
			tags = defaultBannedTags
		}
		if len(tags) > 0 {
			quoted := make([]string, 0, len(tags))
			for _, t := range tags {
				if t == "" {
					continue
				}
				quoted = append(quoted, regexp.QuoteMeta(t))
			}
			// tags are only tags at the head of a line or before a colon; the word
			// "todo" inside a sentence is prose, not an abandoned task
			sort.Strings(quoted)
			pattern := `(?m)(?:^|\s)(` + strings.Join(quoted, "|") + `)\b\s*[:(]?`
			re, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("style.tags: %w", err)
			}
			cfg.style.tags = re
			cfg.style.tagsPre = newPrefilter(pattern)
		}
	}

	var raw []BannedPattern
	if s.Patterns != nil {
		raw = s.Patterns
	} else if boolOr(s.UseDefaultPatterns, p.style) {
		raw = defaultBannedPatterns
	}
	raw = append(raw[:len(raw):len(raw)], s.PatternsExtra...)

	cfg.style.patterns = make([]bannedPattern, 0, len(raw))
	for _, bp := range raw {
		if bp.Pattern == "" {
			return fmt.Errorf("style.patterns: empty pattern")
		}
		re, err := regexp.Compile(bp.Pattern)
		if err != nil {
			return fmt.Errorf("style.patterns: bad regexp %q: %w", bp.Pattern, err)
		}
		msg := bp.Message
		if msg == "" {
			msg = fmt.Sprintf("matches banned pattern %q", bp.Pattern)
		}
		cfg.style.patterns = append(cfg.style.patterns, bannedPattern{
			re:      re,
			pre:     newPrefilter(bp.Pattern),
			message: msg,
		})
	}
	return nil
}

func (cfg *config) applyGodoc(s GodocSettings, p preset) {
	on := boolOr(s.Enabled, p.godoc)
	cfg.godoc = godocConfig{
		enabled:        on,
		startsWithName: on && boolOr(s.StartsWithName, true),
		capitalized:    on && boolOr(s.Capitalized, true),
		endsWithPeriod: on && boolOr(s.EndsWithPeriod, true),
		reportsWhether: on && boolOr(s.ReportsWhether, true),
		allSymbols:     s.Scope == "all" || (s.Scope == "" && p.godocAll),
	}
}
