// Package analyzer implements commentlen, a configurable size and style linter
// for Go comments.
//
// Every comment in a file is classified into exactly one kind — package doc,
// exported doc, unexported doc, field, inline, trailing or other — and gets the
// limits configured for that kind: number of lines and width. On top of the
// size rules the linter enforces a comment-to-code ratio, a per-function budget
// of inline comments, a set of banned phrases and tags, and godoc grammar.
//
// Nothing is mandatory: every check, kind, file and single comment can be
// silenced through settings, through a path or text pattern, or through a
// //commentlen:disable directive in the source.
//
// The analyzer needs syntax only. It never asks for type information, so it
// stays in golangci-lint's fast load mode, and it does one AST walk plus one
// pass over the file's comments — no per-comment allocation, no regexp in the
// hot path unless a check is configured to need one.
package analyzer

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Name is the linter name, as used in .golangci.yml and in //nolint directives.
const Name = "commentlen"

// Doc is the analyzer's one-line description.
const Doc = "checks the size and shape of comments: per-kind length limits, " +
	"comment-to-code ratio, banned phrases and godoc grammar"

// New builds the analyzer from already-decoded settings.
func New(s Settings) (*analysis.Analyzer, error) {
	cfg, err := newConfig(s)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Name, err)
	}
	r := &runner{cfg: cfg}
	return &analysis.Analyzer{
		Name:             Name,
		Doc:              Doc,
		URL:              "https://github.com/swchck/commentlen",
		Run:              r.run,
		RunDespiteErrors: true,
	}, nil
}

// MustNew is New for callers that build the analyzer from a literal, such as
// the standalone binary.
func MustNew(s Settings) *analysis.Analyzer {
	a, err := New(s)
	if err != nil {
		panic(err)
	}
	return a
}

type runner struct {
	cfg *config
}

// run keeps the reusable buffers on the stack: one analyzer value runs on many
// packages at once, so mutable state may not live on the runner.
func (r *runner) run(pass *analysis.Pass) (any, error) {
	var sc scratch
	for _, file := range pass.Files {
		r.checkFile(pass, file, &sc)
	}
	return nil, nil
}

func (r *runner) checkFile(pass *analysis.Pass, file *ast.File, sc *scratch) {
	if len(file.Comments) == 0 {
		return
	}
	tf := pass.Fset.File(file.Pos())
	if tf == nil {
		return
	}
	name := tf.Name()
	if r.skipPath(name) {
		return
	}
	if r.cfg.skipGenerated && isGenerated(file, r.cfg.generatedExtra) {
		return
	}
	src, err := readFile(pass, name)
	if err != nil {
		return // an unreadable file is the build's problem, not the linter's
	}

	fc := newFileContext(r.configFor(name), pass.Fset, tf, src, file, sc)
	if fc.fileDisabled {
		return
	}

	inline := growInts(sc.inline, len(fc.funcs))
	sc.inline = inline
	for _, group := range file.Comments {
		if fc.isDisabled(group.Pos()) || groupHasIgnore(group) {
			continue
		}
		kind, target := fc.classify(group)
		kc := &fc.cfg.kinds[kind]
		if kc.disabled {
			continue
		}
		info := fc.prepare(group, kind, target, sc)
		if len(info.lines) == 0 {
			continue
		}
		if fc.cfg.ignoreDirectives && info.allDirectives {
			continue
		}
		if excluded(fc, info, sc) {
			continue
		}

		if kind == KindInline {
			if i := fc.enclosingFunc(group.Pos()); i >= 0 {
				inline[i]++
			}
		}

		if kc.forbidden {
			pass.Reportf(group.Pos(), "%s comments are not allowed", kind)
			continue
		}
		checkSize(pass, fc, info, kc)
		checkRatio(pass, fc, info)
		checkStyle(pass, fc, info, sc)
		checkGodoc(pass, fc, info, sc)
	}
	checkInlineBudget(pass, fc, inline)
}

// configFor returns the settings that apply to one file: the first matching
// override, or the base config.
func (r *runner) configFor(name string) *config {
	if len(r.cfg.overrides) == 0 {
		return r.cfg
	}
	slashed := filepath.ToSlash(name)
	for i := range r.cfg.overrides {
		if r.cfg.overrides[i].re.MatchString(slashed) {
			return r.cfg.overrides[i].cfg
		}
	}
	return r.cfg
}

func (r *runner) skipPath(name string) bool {
	if r.cfg.skipTests && strings.HasSuffix(name, "_test.go") {
		return true
	}
	if len(r.cfg.excludeFiles) == 0 {
		return false
	}
	slashed := filepath.ToSlash(name)
	for _, re := range r.cfg.excludeFiles {
		if re.MatchString(slashed) {
			return true
		}
	}
	return false
}

func excluded(fc *fileContext, info *commentInfo, sc *scratch) bool {
	if len(fc.cfg.excludeComments) == 0 {
		return false
	}
	text := info.prose(sc)
	for _, re := range fc.cfg.excludeComments {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

// growInts returns a zeroed slice of length n, reusing the buffer when it fits.
func growInts(buf []int, n int) []int {
	if cap(buf) < n {
		return make([]int, n)
	}
	buf = buf[:n]
	clear(buf)
	return buf
}

// readFile prefers the framework's reader, which records the read as a
// dependency, and falls back to the OS when a runner provides none.
func readFile(pass *analysis.Pass, name string) ([]byte, error) {
	if pass.ReadFile != nil {
		return pass.ReadFile(name)
	}
	return os.ReadFile(name)
}
