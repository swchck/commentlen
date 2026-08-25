package analyzer

import (
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// metadataRe matches authorship and change-history lines. Copyright headers are
// deliberately not in here: those are legal boilerplate, not stale metadata.
const metadataPattern = `(?i)(^|\s)(@author|@date|@version|@since|@modified|@created)\b|` +
	`(?im)^\s*(author|created|modified|updated|changed|revision|history)\s*(by)?\s*[:=]`

var (
	metadataRe  = regexp.MustCompile(metadataPattern)
	metadataPre = newPrefilter(metadataPattern)
)

// codeishRe recognizes lines that read as Go source. Both ends are anchored, so
// a keyword inside a sentence cannot match.
var codeishRe = regexp.MustCompile(`^\s*(?:` +
	`[\w.\[\]*]+(?:\s*,\s*[\w.\[\]*]+)*\s*(?::=|\+=|-=|\*=|/=|%=|=[^=])\s*\S|` +
	`(?:return(?:\s+[\w.\[\]{}("'*&!-]\S*)?|break|continue|fallthrough|goto \w+|else|default:)\s*$|` +
	`(?:if|for|switch|select|go|defer|func|type|var|const|case|else|range)\b[^.?]*[{;]\s*$|` +
	`[})\]]+[,;{]?\s*$|` +
	`[\w.]+\([^()]*\)\s*[;{]?\s*$` +
	`)`)

func checkStyle(pass *analysis.Pass, fc *fileContext, info *commentInfo, sc *scratch) {
	st := &fc.cfg.style
	if st.tags == nil && len(st.patterns) == 0 && !st.banners && !st.metadata && !st.commentedCode {
		return
	}

	if st.banners {
		for i := range info.lines {
			if l := &info.lines[i]; isBanner(l.body) {
				pass.Reportf(l.pos, "decorative separator in a comment: carries no information")
			}
		}
	}
	if st.commentedCode {
		reportCommentedCode(pass, st, info)
	}

	text := info.prose(sc)
	if text == "" {
		return
	}
	lower := info.proseLower(sc)
	if st.tags != nil && st.tagsPre.mayMatch(lower) {
		if m := st.tags.FindStringSubmatch(text); m != nil {
			pass.Reportf(info.group.Pos(),
				"%s marker left in a comment: finish it or ask, do not park it here", strings.TrimSpace(m[1]))
		}
	}
	for i := range st.patterns {
		p := &st.patterns[i]
		if p.pre.mayMatch(lower) && p.re.MatchString(text) {
			pass.Reportf(info.group.Pos(), "%s", p.message)
		}
	}
	if st.metadata && metadataPre.mayMatch(lower) && metadataRe.MatchString(text) {
		pass.Reportf(info.group.Pos(), "authorship or change-history metadata in a comment: that is git's job")
	}
}

// reportCommentedCode looks for a run of consecutive code-looking lines, long
// enough that prose is an unlikely explanation.
func reportCommentedCode(pass *analysis.Pass, st *styleConfig, info *commentInfo) {
	need := st.commentedCodeMinLines
	run, start := 0, 0
	flush := func() {
		if run >= need {
			pass.Reportf(info.lines[start].pos,
				"commented-out code (%d lines): delete it, git remembers", run)
		}
		run = 0
	}
	for i := range info.lines {
		l := &info.lines[i]
		if l.directive || l.blank {
			continue
		}
		if looksLikeCode(l.body) {
			if run == 0 {
				start = i
			}
			run++
			continue
		}
		flush()
	}
	flush()
}

// bareStatements are the statements that carry no punctuation at all, so the
// cheap pre-filter has to look for them by name.
var bareStatements = [...]string{"return", "break", "continue", "fallthrough", "goto ", "else"}

// looksLikeCode keeps the regexp off the hot path: a sentence has neither
// statement punctuation nor a leading statement keyword.
func looksLikeCode(body string) bool {
	if !strings.ContainsAny(body, "{};()=") {
		trimmed := strings.TrimLeft(body, " \t")
		found := false
		for _, kw := range bareStatements {
			if strings.HasPrefix(trimmed, kw) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return codeishRe.MatchString(body)
}

// bannerRunes are the characters people build separators out of.
const bannerRunes = "=-*#~_+.^"

func isBanner(body string) bool {
	t := strings.TrimSpace(body)
	if len(t) < 4 {
		return false
	}
	c := t[0]
	if strings.IndexByte(bannerRunes, c) < 0 {
		return false
	}
	for i := 1; i < len(t); i++ {
		if t[i] != c {
			return false
		}
	}
	return true
}
