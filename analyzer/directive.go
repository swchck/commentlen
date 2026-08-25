package analyzer

import (
	"go/ast"
	"strings"
)

type directiveAct uint8

const (
	actionNone directiveAct = iota
	actionDisable
	actionEnable
	actionDisableFile
	actionIgnore
)

// DirectivePrefix is the marker the linter answers to in source files.
const DirectivePrefix = "//commentlen:"

// directiveAction reports which of the linter's own directives a comment
// carries. The text is the raw comment, marker included.
func directiveAction(text string) directiveAct {
	rest, ok := trimDirective(text)
	if !ok {
		return actionNone
	}
	// only the first word matters; "disable // because X" stays a disable
	if i := strings.IndexAny(rest, " \t/"); i >= 0 {
		rest = rest[:i]
	}
	switch rest {
	case "disable":
		return actionDisable
	case "enable":
		return actionEnable
	case "disable-file":
		return actionDisableFile
	case "ignore":
		return actionIgnore
	}
	return actionNone
}

func trimDirective(text string) (string, bool) {
	if !strings.HasPrefix(text, "//") {
		return "", false
	}
	body := strings.TrimLeft(text[2:], " \t")
	const name = "commentlen:"
	if !strings.HasPrefix(body, name) {
		return "", false
	}
	return strings.TrimLeft(body[len(name):], " \t"), true
}

func groupHasIgnore(group *ast.CommentGroup) bool {
	for _, c := range group.List {
		switch directiveAction(c.Text) {
		case actionIgnore, actionDisable, actionDisableFile:
			return true
		}
		if nolintCovers(c.Text) {
			return true
		}
	}
	return false
}

// nolintCovers reports whether a //nolint directive silences this linter, which
// golangci-lint handles itself but the standalone binary does not.
func nolintCovers(text string) bool {
	if !strings.HasPrefix(text, "//") {
		return false
	}
	body := strings.TrimLeft(text[2:], " \t")
	if !strings.HasPrefix(body, "nolint") {
		return false
	}
	rest := body[len("nolint"):]
	if rest == "" || strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t") {
		return true // bare //nolint disables everything
	}
	if !strings.HasPrefix(rest, ":") {
		return false
	}
	for _, name := range strings.Split(rest[1:], ",") {
		if name, _, _ = strings.Cut(name, " "); name == Name || name == "all" {
			return true
		}
	}
	return false
}

// isDirective reports whether a comment is a tool directive, which exempts it
// from every rule. The generic test is copied from go/ast.
func isDirective(text string, extraPrefixes []string) bool {
	if !strings.HasPrefix(text, "//") {
		return false
	}
	body := text[2:]
	if body == "" {
		return false
	}
	if goDirective(body) {
		return true
	}
	for _, p := range extraPrefixes {
		if strings.HasPrefix(body, p) {
			return true
		}
	}
	return false
}

// goDirective mirrors go/ast.isDirective: `//[a-z0-9]+:[^\s]`, plus the three
// legacy spellings the compiler still honours.
func goDirective(c string) bool {
	if strings.HasPrefix(c, "line ") || strings.HasPrefix(c, "extern ") || strings.HasPrefix(c, "export ") {
		return true
	}
	colon := strings.IndexByte(c, ':')
	if colon <= 0 || colon+1 >= len(c) {
		return false
	}
	for i := 0; i <= colon+1; i++ {
		if i == colon {
			continue
		}
		if b := c[i]; !('a' <= b && b <= 'z' || '0' <= b && b <= '9') {
			return false
		}
	}
	return true
}
