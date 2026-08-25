package analyzer

import (
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

func (r *runner) checkSize(pass *analysis.Pass, fc *fileContext, info *commentInfo, kc *kindConfig) {
	if kc.maxLines > 0 && info.counted > kc.maxLines {
		pass.Reportf(info.overflowPos(fc, kc.maxLines),
			"%s comment is %d lines long, max %d", info.kind, info.counted, kc.maxLines)
	}
	if kc.maxWidth <= 0 {
		return
	}
	for i := range info.lines {
		l := &info.lines[i]
		if l.directive || l.width <= kc.maxWidth {
			continue
		}
		if l.codeBlock && r.cfg.ignoreCodeBlocks {
			continue
		}
		if r.cfg.ignoreURLs && overflowIsLink(l.raw, kc.maxWidth) {
			continue
		}
		pass.Reportf(l.pos, "%s comment line is %d columns wide, max %d", info.kind, l.width, kc.maxWidth)
	}
}

// overflowPos points at the first line beyond the budget rather than at the
// start of the comment, so the editor lands on the part that has to go.
func (info *commentInfo) overflowPos(fc *fileContext, maxLines int) token.Pos {
	seen := 0
	for i := range info.lines {
		if !fc.counts(&info.lines[i]) {
			continue
		}
		seen++
		if seen > maxLines {
			return info.lines[i].pos
		}
	}
	return info.group.Pos()
}

// overflowIsLink reports whether only one unbreakable token — a URL, an import
// path — pushes the line over. Wrapping those makes the comment worse.
func overflowIsLink(raw string, maxWidth int) bool {
	longest := 0
	for _, field := range strings.Fields(raw) {
		if len(field) <= longest {
			continue
		}
		if strings.Contains(field, "://") || strings.HasPrefix(field, "www.") || strings.Contains(field, "@") {
			longest = len(field)
		}
	}
	if longest == 0 {
		return false
	}
	// the link is allowed to overflow, everything around it is not
	return len(raw)-longest <= maxWidth
}
