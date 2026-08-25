package analyzer

import (
	"go/ast"
	"go/token"
	"strings"
	"unicode/utf8"
)

// commentLine is one physical line of a comment group.
type commentLine struct {
	// pos is the comment's first character on this line (the marker, or the
	// column for a block comment's continuation).
	pos token.Pos
	// raw is the line as written, comment markers included, trailing space cut.
	raw string
	// body is raw without the marker, indentation kept so code blocks show.
	body string
	// width is the line's rune count, indentation included on request.
	width int

	blank     bool
	codeBlock bool
	directive bool
}

// commentInfo is a comment group prepared for the checks.
type commentInfo struct {
	group  *ast.CommentGroup
	kind   Kind
	target docTarget

	lines []commentLine
	// counted is the number of lines that count towards max-lines.
	counted int
	// allDirectives marks a group made up entirely of tool directives.
	allDirectives bool
	// hasBlock marks a group containing a /* */ comment.
	hasBlock bool

	text  string
	lower []byte
}

// scratch holds the buffers reused across comment groups and files, so a package
// allocates a handful of times instead of once per comment.
type scratch struct {
	info  commentInfo
	lines []commentLine
	sb    strings.Builder

	lower    []byte
	docs     map[*ast.CommentGroup]docTarget
	funcs    []funcSpan
	blocks   []blockSpan
	disabled []span
	inline   []int
}

// prepare splits a comment group into lines and classifies each of them. The
// returned value is owned by the scratch buffer and is valid until the next call.
func (fc *fileContext) prepare(group *ast.CommentGroup, kind Kind, target docTarget, sc *scratch) *commentInfo {
	info := &sc.info
	*info = commentInfo{group: group, kind: kind, target: target, lines: sc.lines[:0]}

	inFence := false
	for _, c := range group.List {
		block := strings.HasPrefix(c.Text, "/*")
		info.hasBlock = info.hasBlock || block

		raw := c.Text // substring slicing below: no copy, no per-line allocation
		col := fc.tf.Position(c.Pos()).Column

		offset := 0
		for lineNo := 0; offset <= len(raw); lineNo++ {
			nl := strings.IndexByte(raw[offset:], '\n')
			end := len(raw)
			if nl >= 0 {
				nl += offset
				end = nl
			}
			text := strings.TrimRight(raw[offset:end], " \t\r")

			indent := 0
			if lineNo == 0 && fc.cfg.widthIncludesIndent {
				indent = col - 1
			}

			line := commentLine{
				pos:   c.Pos() + token.Pos(offset),
				raw:   text,
				body:  stripMarkers(text, block, lineNo == 0),
				width: indent + utf8.RuneCountInString(text),
			}
			trimmed := strings.TrimSpace(line.body)
			line.blank = trimmed == ""
			// with ignore-directives off, a directive is measured like any other
			// line, so it must not be flagged as one here
			line.directive = fc.cfg.ignoreDirectives && !block && isDirective(text, fc.cfg.directivePrefixes)

			switch {
			case strings.HasPrefix(trimmed, "```"):
				line.codeBlock = true
				inFence = !inFence
			case inFence:
				line.codeBlock = true
			case !block && !line.blank && docKind(kind) && isIndentedCode(line.body):
				line.codeBlock = true
			}

			info.lines = append(info.lines, line)

			if nl < 0 {
				break
			}
			offset = nl + 1
		}
	}

	info.allDirectives = len(info.lines) > 0
	for i := range info.lines {
		l := &info.lines[i]
		if !l.directive && !l.blank {
			info.allDirectives = false
		}
		if fc.counts(l) {
			info.counted++
		}
	}

	sc.lines = info.lines
	return info
}

// counts reports whether a line contributes to the max-lines budget.
func (fc *fileContext) counts(l *commentLine) bool {
	switch {
	case l.blank && !fc.cfg.countBlankLines:
		return false
	case l.directive:
		return false
	case l.codeBlock && fc.cfg.ignoreCodeBlocks:
		return false
	}
	return true
}

// prose joins the group's non-directive lines into one string, built at most
// once per group.
func (info *commentInfo) prose(sc *scratch) string {
	if info.text != "" {
		return info.text
	}
	if l := info.onlyProseLine(); l != nil {
		info.text = strings.TrimSpace(l.body)
		return info.text
	}
	sc.sb.Reset()
	for i := range info.lines {
		l := &info.lines[i]
		if l.directive {
			continue
		}
		if sc.sb.Len() > 0 {
			sc.sb.WriteByte('\n')
		}
		sc.sb.WriteString(strings.TrimSpace(l.body))
	}
	info.text = sc.sb.String()
	return info.text
}

// proseLower returns the prose folded to lower-case ASCII, for the prefilters.
func (info *commentInfo) proseLower(sc *scratch) []byte {
	if info.lower == nil {
		info.lower = foldASCII(sc.lower, info.prose(sc))
		sc.lower = info.lower
	}
	return info.lower
}

// onlyProseLine returns the group's single line of words, or nil when it has
// none or several. A one-line comment then needs no string building.
func (info *commentInfo) onlyProseLine() *commentLine {
	var found *commentLine
	for i := range info.lines {
		l := &info.lines[i]
		if l.directive || l.blank {
			continue
		}
		if found != nil {
			return nil
		}
		found = l
	}
	return found
}

// firstProseLine returns the first line carrying words, which is the one godoc
// grammar rules apply to.
func (info *commentInfo) firstProseLine() *commentLine {
	for i := range info.lines {
		if l := &info.lines[i]; !l.blank && !l.directive {
			return l
		}
	}
	return nil
}

func (info *commentInfo) lastProseLine() *commentLine {
	for i := len(info.lines) - 1; i >= 0; i-- {
		if l := &info.lines[i]; !l.blank && !l.directive && !l.codeBlock {
			return l
		}
	}
	return nil
}

// stripMarkers removes the comment markers but keeps the indentation that
// follows them.
func stripMarkers(text string, block, first bool) string {
	if !block {
		return strings.TrimPrefix(text, "//")
	}
	if first {
		text = strings.TrimPrefix(text, "/*")
	}
	text = strings.TrimSuffix(text, "*/")
	if !first {
		// the " * " continuation style is decoration, not content
		if t := strings.TrimLeft(text, " \t"); strings.HasPrefix(t, "*") {
			return t[1:]
		}
	}
	return text
}

// isIndentedCode reports whether a doc-comment line is a pre-formatted block:
// one tab or four spaces of indentation. Elsewhere indentation is mere layout.
func isIndentedCode(body string) bool {
	if strings.HasPrefix(body, "\t") || strings.HasPrefix(body, " \t") {
		return true
	}
	return strings.HasPrefix(body, "     ") // one space separator plus four
}
