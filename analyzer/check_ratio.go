package analyzer

import (
	"go/token"
	"sort"
	"strconv"

	"golang.org/x/tools/go/analysis"
)

// checkRatio enforces "a comment is never longer than the code it describes".
func checkRatio(pass *analysis.Pass, fc *fileContext, info *commentInfo) {
	cfg := fc.cfg
	if !cfg.ratio.enabled || !cfg.ratio.kinds[info.kind] || info.counted == 0 {
		return
	}

	codeLines := 0
	if info.kind == KindInline {
		codeLines = fc.describedStmtLines(info)
	} else if info.target.codeStart.IsValid() && info.target.codeEnd.IsValid() {
		codeLines = fc.lineSpan(info.target.codeStart, info.target.codeEnd)
	}
	if codeLines < cfg.ratio.minCodeLines {
		return
	}
	if float64(info.counted) <= float64(codeLines)*cfg.ratio.max {
		return
	}
	pass.Reportf(info.group.Pos(),
		"%s comment is %s for %s of code, max ratio %.2g — say less, or extract the code",
		info.kind, plural(info.counted, "line"), plural(codeLines, "line"), cfg.ratio.max)
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

func (fc *fileContext) lineSpan(start, end token.Pos) int {
	return fc.tf.Line(end) - fc.tf.Line(start) + 1
}

// describedStmtLines measures the code below an inline comment: the next
// statement plus those following it with no blank line or comment in between.
func (fc *fileContext) describedStmtLines(info *commentInfo) int {
	after := info.group.End()
	bi := fc.enclosingBlock(after)
	if bi < 0 {
		return 0
	}
	list := fc.blocks[bi].list
	i := sort.Search(len(list), func(i int) bool { return list[i].Pos() > after })
	if i >= len(list) {
		return 0
	}

	first := list[i]
	startLine := fc.tf.Line(first.Pos())
	endLine := fc.tf.Line(first.End())
	prevEnd := first.End()

	for j := i + 1; j < len(list); j++ {
		next := list[j]
		if fc.tf.Line(next.Pos()) > endLine+1 {
			break
		}
		if fc.hasCommentBetween(prevEnd, next.Pos()) {
			break
		}
		endLine = fc.tf.Line(next.End())
		prevEnd = next.End()
	}
	return endLine - startLine + 1
}

// enclosingBlock returns the index of the innermost statement list containing
// pos, or -1.
func (fc *fileContext) enclosingBlock(pos token.Pos) int {
	i := sort.Search(len(fc.blocks), func(i int) bool { return fc.blocks[i].start > pos })
	for i--; i >= 0; i-- {
		if fc.blocks[i].end >= pos {
			return i
		}
	}
	return -1
}

func (fc *fileContext) hasCommentBetween(from, to token.Pos) bool {
	groups := fc.file.Comments
	i := sort.Search(len(groups), func(i int) bool { return groups[i].Pos() >= from })
	return i < len(groups) && groups[i].Pos() < to
}
