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

	// Overrides relax or tighten the settings for the files matching a path.
	// The first matching entry wins, so order them from most to least specific.
	Overrides []Override `json:"overrides"`
}

// Override is a settings layer applied to the files whose path matches Path.
// It accepts every key of Settings except preset, overrides, and the four that
// select files at all: skip-generated, skip-tests, exclude-files and
// generated-extra.
//
// The typical use is lifting the size limits in tests while keeping the content
// rules, which apply to test prose just as much.
type Override struct {
	// Path is a regexp matched against the slash-separated file path.
	Path string `json:"path"`

	Settings
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

	// overrides are consulted in order; the first match replaces this config
	// wholesale for that file.
	overrides []pathConfig
}

type pathConfig struct {
	re  *regexp.Regexp
	cfg *config
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

	cfg := baseConfig(p)
	if err := cfg.apply(s, ""); err != nil {
		return nil, err
	}

	for i := range s.Overrides {
		ov := &s.Overrides[i]
		where := fmt.Sprintf("overrides[%d]", i)
		if err := validateOverride(ov, where); err != nil {
			return nil, err
		}
		re, err := regexp.Compile(ov.Path)
		if err != nil {
			return nil, fmt.Errorf("%s.path: bad regexp %q: %w", where, ov.Path, err)
		}
		child := cfg.clone()
		if err := child.apply(ov.Settings, where+"."); err != nil {
			return nil, err
		}
		cfg.overrides = append(cfg.overrides, pathConfig{re: re, cfg: child})
	}
	return cfg, nil
}

func validateOverride(ov *Override, where string) error {
	switch {
	case ov.Path == "":
		return fmt.Errorf("%s: path is required", where)
	case ov.Preset != "":
		return fmt.Errorf("%s: preset belongs at the top level", where)
	case len(ov.Overrides) > 0:
		return fmt.Errorf("%s: overrides cannot nest", where)
	case ov.SkipGenerated != nil, ov.SkipTests != nil, len(ov.ExcludeFiles) > 0, len(ov.GeneratedExtra) > 0:
		return fmt.Errorf("%s: file selection (skip-generated, skip-tests, exclude-files, generated-extra) is global", where)
	}
	return nil
}

// baseConfig is the preset expanded into concrete values, before any user
// setting is layered on top.
func baseConfig(p preset) *config {
	cfg := &config{
		skipGenerated:       true,
		ignoreDirectives:    true,
		ignoreURLs:          true,
		ignoreCodeBlocks:    true,
		widthIncludesIndent: true,
		directivePrefixes:   defaultDirectivePrefixes,
		kinds:               p.kinds,
		maxInlinePerFunc:    p.maxInlinePerFunc,
		ratio: ratioConfig{
			enabled:      p.ratioEnabled,
			max:          1.0,
			minCodeLines: 2,
		},
		style: styleConfig{
			banners:               p.style,
			metadata:              p.style,
			commentedCode:         p.style,
			commentedCodeMinLines: 2,
		},
		godoc: godocConfig{
			enabled:        p.godoc,
			startsWithName: true,
			capitalized:    true,
			endsWithPeriod: true,
			reportsWhether: true,
			allSymbols:     p.godocAll,
		},
	}
	for _, name := range p.ratioKinds {
		if k, ok := parseKind(name); ok {
			cfg.ratio.kinds[k] = true
		}
	}
	if p.style {
		cfg.style.tags, cfg.style.tagsPre = compileTags(defaultBannedTags)
		cfg.style.patterns, _ = compilePatterns(defaultBannedPatterns, nil)
	}
	cfg.needsStmts = cfg.ratio.enabled
	return cfg
}

// clone copies a config deeply enough that apply cannot write through to the
// original: the arrays are values, and the slices are capped so append copies.
func (cfg *config) clone() *config {
	c := *cfg
	c.overrides = nil
	c.style.patterns = c.style.patterns[:len(c.style.patterns):len(c.style.patterns)]
	c.directivePrefixes = c.directivePrefixes[:len(c.directivePrefixes):len(c.directivePrefixes)]
	return &c
}

// apply layers one Settings value on top of the config; a nil field leaves the
// current value alone. The where prefix names the block in error messages.
func (cfg *config) apply(s Settings, where string) error {
	cfg.skipGenerated = boolOr(s.SkipGenerated, cfg.skipGenerated)
	cfg.skipTests = boolOr(s.SkipTests, cfg.skipTests)
	cfg.ignoreDirectives = boolOr(s.IgnoreDirectives, cfg.ignoreDirectives)
	cfg.ignoreURLs = boolOr(s.IgnoreURLs, cfg.ignoreURLs)
	cfg.ignoreCodeBlocks = boolOr(s.IgnoreCodeBlocks, cfg.ignoreCodeBlocks)
	cfg.countBlankLines = boolOr(s.CountBlankLines, cfg.countBlankLines)
	cfg.widthIncludesIndent = boolOr(s.WidthIncludesIndent, cfg.widthIncludesIndent)
	cfg.maxInlinePerFunc = intOr(s.MaxInlinePerFunc, cfg.maxInlinePerFunc)

	var err error
	if s.ExcludeFiles != nil {
		if cfg.excludeFiles, err = compileAll(s.ExcludeFiles, where+"exclude-files"); err != nil {
			return err
		}
	}
	if s.ExcludePatterns != nil {
		if cfg.excludeComments, err = compileAll(s.ExcludePatterns, where+"exclude-patterns"); err != nil {
			return err
		}
	}
	if s.GeneratedExtra != nil {
		if cfg.generatedExtra, err = compileAll(s.GeneratedExtra, where+"generated-extra"); err != nil {
			return err
		}
	}
	if len(s.DirectivePrefixesExtra) > 0 {
		cfg.directivePrefixes = append(cfg.directivePrefixes, s.DirectivePrefixesExtra...)
	}

	if err := cfg.applyKinds(s, where); err != nil {
		return err
	}
	if cfg.maxInlinePerFunc < 0 {
		return fmt.Errorf("%smax-inline-per-func must not be negative", where)
	}
	if err := cfg.applyRatio(s.Ratio, where); err != nil {
		return err
	}
	if err := cfg.applyStyle(s.Style, where); err != nil {
		return err
	}
	if err := cfg.applyGodoc(s.Godoc, where); err != nil {
		return err
	}
	cfg.needsStmts = cfg.ratio.enabled
	return nil
}

func (cfg *config) applyKinds(s Settings, where string) error {
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
			return fmt.Errorf("%skinds: unknown kind %q: want one of %s", where, name, strings.Join(KindNames(), ", "))
		}
		applyKind(k, ks)
	}
	for k := Kind(0); int(k) < numKinds; k++ {
		if c := cfg.kinds[k]; c.maxLines < 0 || c.maxWidth < 0 {
			return fmt.Errorf("%skinds.%s: max-lines and max-width must not be negative", where, k)
		}
	}
	return nil
}

func (cfg *config) applyRatio(s RatioSettings, where string) error {
	cfg.ratio.enabled = boolOr(s.Enabled, cfg.ratio.enabled)
	cfg.ratio.max = floatOr(s.Max, cfg.ratio.max)
	cfg.ratio.minCodeLines = intOr(s.MinCodeLines, cfg.ratio.minCodeLines)
	if cfg.ratio.max <= 0 {
		return fmt.Errorf("%sratio.max must be positive", where)
	}
	if s.Kinds == nil {
		return nil
	}
	cfg.ratio.kinds = [numKinds]bool{}
	for _, name := range s.Kinds {
		k, ok := parseKind(name)
		if !ok {
			return fmt.Errorf("%sratio.kinds: unknown kind %q: want one of %s", where, name, strings.Join(KindNames(), ", "))
		}
		cfg.ratio.kinds[k] = true
	}
	return nil
}

// compileTags turns a tag list into the regexp that finds one. A tag only counts
// at the head of a line or before a colon, so "the todos of a user" is prose.
func compileTags(tags []string) (*regexp.Regexp, prefilter) {
	quoted := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != "" {
			quoted = append(quoted, regexp.QuoteMeta(tag))
		}
	}
	if len(quoted) == 0 {
		return nil, prefilter{}
	}
	sort.Strings(quoted)
	pattern := `(?m)(?:^|\s)(` + strings.Join(quoted, "|") + `)\b\s*[:(]?`
	return regexp.MustCompile(pattern), newPrefilter(pattern)
}

func compilePatterns(list, extra []BannedPattern) ([]bannedPattern, error) {
	out := make([]bannedPattern, 0, len(list)+len(extra))
	for _, bp := range append(list[:len(list):len(list)], extra...) {
		if bp.Pattern == "" {
			return nil, fmt.Errorf("empty pattern")
		}
		re, err := regexp.Compile(bp.Pattern)
		if err != nil {
			return nil, fmt.Errorf("bad regexp %q: %w", bp.Pattern, err)
		}
		msg := bp.Message
		if msg == "" {
			msg = fmt.Sprintf("matches banned pattern %q", bp.Pattern)
		}
		out = append(out, bannedPattern{re: re, pre: newPrefilter(bp.Pattern), message: msg})
	}
	return out, nil
}

func (cfg *config) applyStyle(s StyleSettings, where string) error {
	cfg.style.banners = boolOr(s.Banners, cfg.style.banners)
	cfg.style.metadata = boolOr(s.Metadata, cfg.style.metadata)
	cfg.style.commentedCode = boolOr(s.CommentedCode, cfg.style.commentedCode)
	cfg.style.commentedCodeMinLines = intOr(s.CommentedCodeMinLines, cfg.style.commentedCodeMinLines)
	if cfg.style.commentedCodeMinLines < 1 {
		return fmt.Errorf("%sstyle.commented-code-min-lines must be at least 1", where)
	}

	switch {
	case s.TagsEnabled != nil && !*s.TagsEnabled:
		cfg.style.tags, cfg.style.tagsPre = nil, prefilter{}
	case s.Tags != nil:
		cfg.style.tags, cfg.style.tagsPre = compileTags(s.Tags)
	case s.TagsEnabled != nil && cfg.style.tags == nil:
		cfg.style.tags, cfg.style.tagsPre = compileTags(defaultBannedTags)
	}

	switch {
	case s.Patterns != nil:
		patterns, err := compilePatterns(s.Patterns, s.PatternsExtra)
		if err != nil {
			return fmt.Errorf("%sstyle.patterns: %w", where, err)
		}
		cfg.style.patterns = patterns
	case s.UseDefaultPatterns != nil:
		var base []BannedPattern
		if *s.UseDefaultPatterns {
			base = defaultBannedPatterns
		}
		patterns, err := compilePatterns(base, s.PatternsExtra)
		if err != nil {
			return fmt.Errorf("%sstyle.patterns: %w", where, err)
		}
		cfg.style.patterns = patterns
	case len(s.PatternsExtra) > 0:
		extra, err := compilePatterns(nil, s.PatternsExtra)
		if err != nil {
			return fmt.Errorf("%sstyle.patterns: %w", where, err)
		}
		cfg.style.patterns = append(cfg.style.patterns, extra...)
	}
	return nil
}

func (cfg *config) applyGodoc(s GodocSettings, where string) error {
	cfg.godoc.enabled = boolOr(s.Enabled, cfg.godoc.enabled)
	cfg.godoc.startsWithName = boolOr(s.StartsWithName, cfg.godoc.startsWithName)
	cfg.godoc.capitalized = boolOr(s.Capitalized, cfg.godoc.capitalized)
	cfg.godoc.endsWithPeriod = boolOr(s.EndsWithPeriod, cfg.godoc.endsWithPeriod)
	cfg.godoc.reportsWhether = boolOr(s.ReportsWhether, cfg.godoc.reportsWhether)
	switch s.Scope {
	case "":
	case "exported":
		cfg.godoc.allSymbols = false
	case "all":
		cfg.godoc.allSymbols = true
	default:
		return fmt.Errorf("%sgodoc.scope: unknown scope %q: want exported or all", where, s.Scope)
	}
	return nil
}
