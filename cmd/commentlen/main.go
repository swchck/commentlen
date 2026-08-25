// Command commentlen runs the comment linter on its own, outside golangci-lint.
//
// Usage:
//
//	commentlen [-config path] [-preset name] [packages]
//
// The configuration file is the same YAML (or JSON) block that goes under
// linters.settings.custom.commentlen.settings in .golangci.yml, without the
// surrounding keys. With no -config flag the command looks for .commentlen.yml,
// .commentlen.yaml and .commentlen.json in the working directory, and falls back
// to the balanced preset.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"
	"gopkg.in/yaml.v3"

	"github.com/swchck/commentlen/analyzer"
)

var (
	configPath string
	presetName string

	once     sync.Once
	delegate *analysis.Analyzer
	initErr  error
)

var defaultConfigNames = []string{".commentlen.yml", ".commentlen.yaml", ".commentlen.json"}

func main() {
	a := &analysis.Analyzer{
		Name:             analyzer.Name,
		Doc:              analyzer.Doc,
		URL:              "https://github.com/swchck/commentlen",
		Run:              run,
		RunDespiteErrors: true,
	}
	a.Flags.StringVar(&configPath, "config", "", "path to a commentlen settings file (YAML or JSON)")
	a.Flags.StringVar(&presetName, "preset", "", "preset to start from: balanced, strict or loose")
	singlechecker.Main(a)
}

// run builds the real analyzer on first use: singlechecker parses the flags only
// after the analyzer value exists.
func run(pass *analysis.Pass) (any, error) {
	once.Do(func() {
		var settings analyzer.Settings
		if settings, initErr = loadSettings(); initErr != nil {
			return
		}
		if presetName != "" {
			settings.Preset = presetName
		}
		delegate, initErr = analyzer.New(settings)
	})
	if initErr != nil {
		return nil, initErr
	}
	return delegate.Run(pass)
}

func loadSettings() (analyzer.Settings, error) {
	path := configPath
	if path == "" {
		for _, name := range defaultConfigNames {
			if _, err := os.Stat(name); err == nil {
				path = name
				break
			}
		}
	}
	if path == "" {
		return analyzer.Settings{}, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return analyzer.Settings{}, fmt.Errorf("read %s: %w", path, err)
	}
	return decodeSettings(raw, path)
}

// decodeSettings reuses golangci-lint's own decoder, so a config behaves
// identically in both runners.
func decodeSettings(raw []byte, path string) (analyzer.Settings, error) {
	var tree any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		return analyzer.Settings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if tree == nil {
		return analyzer.Settings{}, nil
	}
	if _, ok := tree.(map[string]any); !ok {
		return analyzer.Settings{}, fmt.Errorf("parse %s: want a mapping of settings at the top level", path)
	}
	s, err := register.DecodeSettings[analyzer.Settings](tree)
	if err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return s, fmt.Errorf("%s: field %q has the wrong type", path, typeErr.Field)
		}
		return s, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}
