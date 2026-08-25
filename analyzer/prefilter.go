package analyzer

import (
	"bytes"
	"regexp"
	"regexp/syntax"
	"strings"
	"unicode/utf8"
)

// minAnchor is the shortest literal worth screening on: below that the scan
// costs more than the regexp it saves.
const minAnchor = 3

// prefilter is a necessary condition for a regexp to match: text holding none of
// the anchors cannot match, so the regexp skips ordinary prose.
type prefilter struct {
	anchors [][]byte
}

// newPrefilter derives the anchors from the pattern. When nothing can be proven
// the filter admits everything, trading speed for correctness.
func newPrefilter(pattern string) prefilter {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return prefilter{}
	}
	lits := requiredLiterals(re.Simplify())
	anchors := make([][]byte, 0, len(lits))
	for _, l := range lits {
		// a short literal screens nothing, and a non-ASCII one cannot be matched
		// against the ASCII-folded text: either way, give up on filtering
		if len(l) < minAnchor || !isASCII(l) {
			return prefilter{}
		}
		anchors = append(anchors, []byte(l))
	}
	return prefilter{anchors: anchors}
}

// mayMatch reports whether the text, folded to lower case by foldASCII, can
// possibly match.
func (p prefilter) mayMatch(lower []byte) bool {
	if len(p.anchors) == 0 {
		return true
	}
	for _, a := range p.anchors {
		if bytes.Contains(lower, a) {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// foldASCII writes text into dst with ASCII letters lower-cased; other bytes are
// copied as they are, which is why non-ASCII anchors are rejected.
func foldASCII(dst []byte, text string) []byte {
	dst = dst[:0]
	for i := 0; i < len(text); i++ {
		c := text[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		dst = append(dst, c)
	}
	return dst
}

// requiredLiterals returns lowercase literals such that every match of re
// contains at least one of them, or nil when no such set can be derived.
func requiredLiterals(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpLiteral:
		s := strings.ToLower(string(re.Rune))
		if s == "" {
			return nil
		}
		return []string{s}

	case syntax.OpCapture:
		return requiredLiterals(re.Sub[0])

	case syntax.OpPlus:
		return requiredLiterals(re.Sub[0])

	case syntax.OpRepeat:
		if re.Min > 0 {
			return requiredLiterals(re.Sub[0])
		}
		return nil

	case syntax.OpConcat:
		// every branch of a concatenation must match, so the longest anchor set
		// among them is enough
		var best []string
		for _, sub := range re.Sub {
			lits := requiredLiterals(sub)
			if len(lits) == 0 {
				continue
			}
			if shortest(lits) > shortest(best) {
				best = lits
			}
		}
		return best

	case syntax.OpAlternate:
		// only one branch matches, so every branch needs its own anchor
		var all []string
		for _, sub := range re.Sub {
			lits := requiredLiterals(sub)
			if len(lits) == 0 {
				return nil
			}
			all = append(all, lits...)
		}
		return all
	}
	return nil
}

func shortest(lits []string) int {
	if len(lits) == 0 {
		return 0
	}
	n := len(lits[0])
	for _, l := range lits[1:] {
		if len(l) < n {
			n = len(l)
		}
	}
	return n
}

// findIfPossible runs the regexp only when the prefilter admits the text.
func findIfPossible(p prefilter, re *regexp.Regexp, text string, lower []byte) string {
	if !p.mayMatch(lower) {
		return ""
	}
	return re.FindString(text)
}
