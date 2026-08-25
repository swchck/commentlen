package analyzer

import (
	"go/ast"
	"regexp"
	"strings"
)

// isGenerated reports whether the file carries a generated-code marker. Only the
// comments above the package clause are read: https://go.dev/s/generatedcode.
func isGenerated(file *ast.File, extra []*regexp.Regexp) bool {
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			return false
		}
		for _, c := range group.List {
			if generatedMarker(c.Text) {
				return true
			}
			for _, re := range extra {
				if re.MatchString(c.Text) {
					return true
				}
			}
		}
	}
	return false
}

// generatedMarker matches `^// Code generated .* DO NOT EDIT\.$` without paying
// for a regexp.
func generatedMarker(text string) bool {
	const prefix = "// Code generated "
	const suffix = " DO NOT EDIT."
	if len(text) < len(prefix)+len(suffix) {
		return false
	}
	if !strings.HasPrefix(text, prefix) {
		return false
	}
	return strings.HasSuffix(strings.TrimRight(text, " \t"), suffix)
}
