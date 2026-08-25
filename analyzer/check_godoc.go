package analyzer

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"
)

// reportsWhetherRe catches the phrasing the standard library spells
// "reports whether".
const reportsWhetherPattern = `(?i)\breturns? (true|false) (if|when|whether)\b`

var (
	reportsWhetherRe  = regexp.MustCompile(reportsWhetherPattern)
	reportsWhetherPre = newPrefilter(reportsWhetherPattern)
)

// sentenceEnders are the characters a doc comment may end on: a period for
// prose, a colon when a list or a code block follows.
const sentenceEnders = ".!?:"

func (r *runner) checkGodoc(pass *analysis.Pass, fc *fileContext, info *commentInfo, sc *scratch) {
	g := &r.cfg.godoc
	if !g.enabled {
		return
	}
	switch info.kind {
	case KindPackage, KindExported:
	case KindUnexported, KindField:
		if !g.allSymbols {
			return
		}
	default:
		return
	}
	if info.kind == KindField && isValueNote(fc, info) {
		return
	}

	first := info.firstProseLine()
	if first == nil {
		return
	}
	text := strings.TrimSpace(first.body)
	if text == "" {
		return
	}

	name := info.target.name
	want := name
	if info.kind == KindPackage {
		if name == "main" {
			name = "" // a command's doc opens with "Command foo", never "Package main"
		}
		want = "Package " + name
	}
	opensWithName := name != "" && opensWith(text, want)

	if g.startsWithName && name != "" && !opensWithName {
		pass.Reportf(first.pos, "doc comment should start with %q", want)
	}
	// an unexported symbol is documented by its own name, which starts lower
	// case: "// f returns nothing." is correct and must not be capitalized
	if g.capitalized && !opensWithName && wantsCapital(info.kind, name) {
		if r0 := []rune(text)[0]; unicode.IsLetter(r0) && unicode.IsLower(r0) {
			pass.Reportf(first.pos, "doc comment should start with a capital letter")
		}
	}
	if g.endsWithPeriod {
		if last := info.lastProseLine(); last != nil {
			if t := strings.TrimSpace(last.body); t != "" && !strings.ContainsRune(sentenceEnders, rune(t[len(t)-1])) {
				pass.Reportf(last.pos, "doc comment should end with a period")
			}
		}
	}
	if g.reportsWhether && info.target.boolFunc {
		prose := info.prose(sc)
		if m := findIfPossible(reportsWhetherPre, reportsWhetherRe, prose, info.proseLower(sc)); m != "" {
			pass.Reportf(first.pos, "%q on a bool-returning function: the convention is \"reports whether\"", m)
		}
	}
}

// isValueNote reports whether the comment is a same-line note on a field — a
// unit or a value remark rather than a sentence.
func isValueNote(fc *fileContext, info *commentInfo) bool {
	return fc.isTrailing(info.group.List[0])
}

// wantsCapital reports whether the first word must be capitalized: a package doc
// always is, a named symbol lends the comment its own case.
func wantsCapital(kind Kind, name string) bool {
	if kind == KindPackage || name == "" {
		return true
	}
	return !unicode.IsLower([]rune(name)[0])
}

// opensWith reports whether the comment starts with the symbol name, allowing an
// article in front ("A Buffer is …") or the name used as a call.
func opensWith(text, want string) bool {
	for _, article := range [...]string{"", "A ", "An ", "The "} {
		rest, ok := cutPrefix(text, article+want)
		if !ok {
			continue
		}
		if rest == "" {
			return true
		}
		switch rest[0] {
		case ' ', ',', '.', '(', '[', ':', ';', '\'', '-':
			return true
		}
	}
	return false
}

func cutPrefix(s, prefix string) (string, bool) {
	if !strings.HasPrefix(s, prefix) {
		return s, false
	}
	return s[len(prefix):], true
}

// checkInlineBudget reports functions carrying more inline comments than the
// budget allows.
func (r *runner) checkInlineBudget(pass *analysis.Pass, fc *fileContext, counts []int) {
	limit := r.cfg.maxInlinePerFunc
	if limit <= 0 {
		return
	}
	for i, n := range counts {
		if n <= limit {
			continue
		}
		fn := &fc.funcs[i]
		if fc.isDisabled(fn.start) {
			continue
		}
		pass.Reportf(fn.start, "%s has %d inline comments, max %d — reconsider the code before adding another",
			fn.describe(), n, limit)
	}
}
