// Package commentlen registers the commentlen linter as a golangci-lint module
// plugin.
//
// Build a custom golangci-lint binary that includes it by listing this module in
// .custom-gcl.yml and running `golangci-lint custom`; then enable the linter
// under linters.settings.custom in .golangci.yml. The settings block is decoded
// into analyzer.Settings, and unknown keys are rejected, so a typo in the
// configuration fails the run instead of being silently ignored.
package commentlen

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/swchck/commentlen/analyzer"
)

func init() {
	register.Plugin(analyzer.Name, newPlugin)
}

type plugin struct {
	settings analyzer.Settings
}

func newPlugin(raw any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[analyzer.Settings](raw)
	if err != nil {
		return nil, err
	}
	return &plugin{settings: s}, nil
}

// BuildAnalyzers returns the single analyzer this plugin provides.
func (p *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	a, err := analyzer.New(p.settings)
	if err != nil {
		return nil, err
	}
	return []*analysis.Analyzer{a}, nil
}

// GetLoadMode keeps the linter in the syntax-only load mode: it reads comments
// and the AST, never type information, which is what makes it cheap to run.
func (p *plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}
