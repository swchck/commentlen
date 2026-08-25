# commentlen

[![CI](https://github.com/swchck/commentlen/actions/workflows/ci.yml/badge.svg)](https://github.com/swchck/commentlen/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/swchck/commentlen.svg)](https://pkg.go.dev/github.com/swchck/commentlen)
[![Go Report Card](https://goreportcard.com/badge/github.com/swchck/commentlen)](https://goreportcard.com/report/github.com/swchck/commentlen)

A configurable size and shape linter for Go comments, usable as a golangci-lint
module plugin or as a standalone binary.

Existing linters check one thing each: `lll` measures any line, `godot` wants a
period, `revive` wants a doc comment to exist. None of them can say "an inline
note may be two lines, an exported doc comment may be six, and neither may be
longer than the code it explains". That is what this one does.

```
foo.go:41:2:  inline comment is 4 lines long, max 2
foo.go:58:2:  inline comment is 3 lines for 2 lines of code, max ratio 1 — say less, or extract the code
foo.go:73:14: trailing comments are not allowed
bar.go:12:1:  doc comment should start with "Config"
bar.go:19:1:  TODO marker left in a comment: finish it or ask, do not park it here
bar.go:88:2:  commented-out code (3 lines): delete it, git remembers
```

## What it checks

Every comment is classified into exactly **one kind**, and each kind has its own
limits:

| kind         | what it is                                                        |
|--------------|-------------------------------------------------------------------|
| `package`    | the package doc, directly above `package x`                        |
| `exported`   | doc comment on an exported top-level declaration                   |
| `unexported` | doc comment on an unexported top-level declaration                 |
| `field`      | comment on a struct field, interface method, or const/var element  |
| `inline`     | comment on its own line inside a function body                     |
| `trailing`   | same-line comment after code inside a function body                |
| `other`      | everything else — floating, inside literals, end of file           |

A method counts as `exported` only when its receiver type is exported too, the
way `golint` defines the exported API. A doc comment on a local `var` inside a
function is classified `inline`, because that is how it reads.

On top of the per-kind size limits:

- **ratio** — a comment must not be longer than the code it describes. For an
  inline comment the described code is the next statement plus the statements
  following it with no blank line or comment in between; for a doc comment it is
  the declaration.
- **max-inline-per-func** — a budget of inline comments per function body.
  Consecutive comment lines count as one comment. Function literals get their
  own budget.
- **style** — banned tags (`TODO`, `FIXME`, …), banned phrases (hedging
  preambles, the vague-abstraction phrases, configurable), decorative separators,
  authorship/change-history metadata, and commented-out code.
- **godoc** — the doc comment opens with the symbol name, is capitalized, ends
  with a period, and spells a bool-returning function's contract "reports
  whether" rather than "returns true if".

### What is never reported

- **Generated files** — anything with a `// Code generated … DO NOT EDIT.`
  marker above the package clause (`skip-generated`, on by default), plus
  whatever `generated-extra` adds.
- **Directives** — `//go:generate`, `//nolint`, `//+build`, `//go:build`,
  `//easyjson:json` and any `//word:word` comment, per `go/ast`'s own rule
  (`ignore-directives`, on by default). Legacy spellings and tool-specific
  prefixes are covered by `directive-prefixes-extra`.
- **godoc code blocks** — indented or ``` fenced blocks inside a doc comment are
  exempt from both width and line count (`ignore-code-blocks`).
- **Blank comment lines** — `//` on its own does not count towards `max-lines`
  unless `count-blank-lines` is set.
- **Long links** — a line over the width limit only because of one unbreakable
  token (URL, import path, email) is left alone (`ignore-urls`).

## Install

### As a golangci-lint plugin

golangci-lint loads plugins by building a binary that includes them. Add
`.custom-gcl.yml` to your repository:

```yaml
version: v2.11.4          # your golangci-lint version
name: golangci-lint-commentlen
destination: ./bin
plugins:
  - module: github.com/swchck/commentlen
    version: v0.1.0
```

Then build it and enable the linter in `.golangci.yml`:

```console
$ golangci-lint custom
$ ./bin/golangci-lint-commentlen run ./...
```

```yaml
version: "2"
linters:
  enable:
    - commentlen
  settings:
    custom:
      commentlen:
        type: module
        description: size and shape limits for Go comments
        settings:
          preset: strict
```

The whole `settings:` block below `commentlen` is what this README documents.
Unknown keys are an error, so a typo fails the run instead of being ignored.

While developing the plugin itself, point at the checkout instead of a version:

```yaml
plugins:
  - module: github.com/swchck/commentlen
    path: /path/to/commentlen
```

### As a standalone binary

```console
$ go install github.com/swchck/commentlen/cmd/commentlen@latest
$ commentlen -preset strict ./...
$ commentlen -config .commentlen.yml ./...
```

With no `-config`, the binary looks for `.commentlen.yml`, `.commentlen.yaml`
and `.commentlen.json` in the working directory. The file holds the same keys as
the golangci-lint `settings:` block, unindented — see `.commentlen.yml` in this
repository.

## Presets

A preset is the baseline; every key you set is layered on top of it, so
`preset: strict` plus `kinds.inline.max-lines: 3` means "strict, but three lines
inline".

| kind / setting        | `balanced` (default) | `strict`      | `loose` |
|-----------------------|----------------------|---------------|---------|
| `package`             | ∞ / 100              | ∞ / 100       | ∞ / ∞   |
| `exported`            | 10 / 100             | 6 / 100       | ∞ / 120 |
| `unexported`          | 6 / 100              | 2 / 100       | ∞ / 120 |
| `field`               | 3 / 100              | 2 / 100       | ∞ / 120 |
| `inline`              | 3 / 100              | 2 / 100       | 6 / 120 |
| `trailing`            | 1 / 80               | forbidden     | 1 / 120 |
| `other`               | 4 / 100              | 2 / 100       | ∞ / 120 |
| `max-inline-per-func` | off                  | 2             | off     |
| `ratio`               | off                  | on            | off     |
| `style`               | on                   | on            | off     |
| `godoc`               | off                  | on            | off     |

Cells are `max-lines / max-width`; `∞` is `0`, meaning unlimited.

## Settings reference

### File selection

| key | default | meaning |
|-----|---------|---------|
| `skip-generated` | `true` | skip files with a generated-code marker above the package clause |
| `generated-extra` | — | extra regexps that mark a file as generated |
| `skip-tests` | `false` | skip `_test.go` entirely |
| `exclude-files` | — | regexps matched against the slash-separated path |

### Comment selection

| key | default | meaning |
|-----|---------|---------|
| `exclude-patterns` | — | regexps matched against a comment's text; a match exempts the comment from every check |
| `ignore-directives` | `true` | exempt tool directives |
| `directive-prefixes-extra` | — | additional directive prefixes (e.g. `ffjson:`) |
| `ignore-urls` | `true` | let a single unbreakable token overflow the width |
| `ignore-code-blocks` | `true` | exempt godoc code blocks |
| `count-blank-lines` | `false` | count `//` lines towards `max-lines` |
| `width-includes-indent` | `true` | measure width from column 1, not from the marker |

### Sizes

```yaml
defaults:            # applied to every kind first
  max-width: 100
kinds:               # then refined per kind
  inline:
    max-lines: 2
    max-width: 100
  trailing:
    forbidden: true  # report the comment whatever its size
  other:
    disabled: true   # no checks at all for this kind
```

`max-lines: 0` and `max-width: 0` mean unlimited. Width is counted in runes, so
non-ASCII prose is measured the way it looks.

### Ratio

```yaml
ratio:
  enabled: true
  max: 1.0            # comment lines / code lines
  min-code-lines: 2   # skip when the code is shorter, so a one-liner may carry an explanation
  kinds: [inline, unexported, other]
```

### Style

```yaml
style:
  tags: [TODO, FIXME, HACK, XXX, OPTIMIZE, BUG]   # replaces the default list
  tags-enabled: true
  patterns: []             # replaces the built-in phrase list
  patterns-extra:          # appends to it
    - pattern: '\b[A-Z]{2,10}-\d{1,6}\b'
      message: ticket key in a comment — explain the reason instead
  use-default-patterns: true
  banners: true            # ===== separators and the like
  metadata: true           # @author, "Modified by:", change history — not copyright headers
  commented-code: true
  commented-code-min-lines: 2
```

A tag is only reported at the head of a line or before a colon, so "the todos of
a user" is prose, not an abandoned task. The built-in phrase list covers hedging
preambles ("Note that", "It's important to note"), the "not just X, but Y"
construction, vague abstraction ("for performance reasons", "to ensure
correctness") and signature-restating openings.

### Godoc

```yaml
godoc:
  enabled: true
  starts-with-name: true    # "Quote returns …", "Package p …"
  capitalized: true         # skipped when the symbol's own name is lower case
  ends-with-period: true    # a colon is accepted when a list or code block follows
  reports-whether: true     # flags "returns true if" on bool-returning functions
  scope: exported           # or "all", to include unexported symbols and fields
```

The naming and capitalization rules accept the usual variations: an article in
front ("A Buffer is …"), the name used as a call ("Quote(s) returns …"), and a
generic instantiation. Two cases are waived entirely, because godoc never renders
them: a `package main` doc, whose convention is "Command foo", and the
`Test`/`Benchmark`/`Fuzz`/`Example` functions of a `_test.go` file. The sentence
rules — capital, final period — still apply there.

## Turning it off in source

```go
//commentlen:disable-file          // the whole file

//commentlen:disable
// … anything here is unchecked …
//commentlen:enable

//commentlen:ignore                // this declaration only
// a doc comment that may be as long as it likes
func Documented() {}

//nolint:commentlen                // honoured by both runners
```

`//commentlen:ignore` in a doc comment covers the comment and the declaration it
documents. Everything after the directive word is free text, so you can say why:

```go
//commentlen:ignore  the doc has to quote the phrases it bans
```

Inside golangci-lint you also have the usual `linters.exclusions.rules` for
paths and message texts — see `examples/golangci.strict.yml`.

## Performance

The linter asks for **syntax only** — never type information — so golangci-lint
keeps it in the fast load mode and never has to type-check a package for it.
Per file it does one AST walk plus one pass over the comment list.

On an 86 KB generated file (200 functions, ~1600 comment lines), Apple M3 Pro:

| what | time | vs parsing | allocs |
|------|------|-----------|--------|
| `go/parser` with comments (the unavoidable baseline) | 1.14 ms | 1.0× | 19645 |
| commentlen, sizes + ratio only | 0.63 ms | 0.55× | 21 |
| commentlen, `balanced` (sizes + style) | 1.78 ms | 1.6× | 426 |
| commentlen, `strict` (everything) | 1.83 ms | 1.6× | 637 |

Three things keep it there:

- **No allocation per comment.** Lines are substrings of the source, and the
  line buffer, the prose buffer and the per-file maps and slices are reused
  across comments and across files.
- **Regexps behind a prefilter.** Every pattern is parsed once to derive the
  literals it cannot match without; a comment holding none of them skips the
  regexp entirely. This alone took the full check from 14.7 ms to 1.7 ms. The
  prefilter is fuzz-tested against the property that it never hides a match.
- **Nothing computed that is not asked for.** Statement spans are collected only
  when the ratio check is on; a disabled kind is dropped before its comment is
  even split into lines.

## Development

```console
$ make test     # unit tests
$ make race     # the same under -race
$ make bench    # benchmarks
$ make fuzz     # fuzz the prefilter invariant
$ make build    # standalone binary
$ make custom   # golangci-lint binary with the plugin compiled in
```

The linter runs on its own source under `preset: strict` and is expected to stay
clean:

```console
$ make custom && ./bin/golangci-lint-commentlen run ./...
```
