package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/swchck/commentlen/analyzer"
)

// TestRepoConfigsAreValid keeps the shipped examples honest: every config in the
// repository must decode into Settings and survive validation.
func TestRepoConfigsAreValid(t *testing.T) {
	root := filepath.Join("..", "..")

	t.Run("standalone", func(t *testing.T) {
		path := filepath.Join(root, ".commentlen.yml")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s, err := decodeSettings(raw, path)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, err := analyzer.New(s); err != nil {
			t.Errorf("validate: %v", err)
		}
	})

	golangciConfigs, err := filepath.Glob(filepath.Join(root, "examples", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	golangciConfigs = append(golangciConfigs, filepath.Join(root, ".golangci.yml"))
	if len(golangciConfigs) < 3 {
		t.Fatalf("expected the example configs to be found, got %v", golangciConfigs)
	}

	for _, path := range golangciConfigs {
		t.Run(filepath.Base(path), func(t *testing.T) {
			settings, err := extractPluginSettings(path)
			if err != nil {
				t.Fatal(err)
			}
			s, err := reencode(settings)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if _, err := analyzer.New(s); err != nil {
				t.Errorf("validate: %v", err)
			}
		})
	}
}

// extractPluginSettings digs out linters.settings.custom.commentlen.settings.
func extractPluginSettings(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Linters struct {
			Settings struct {
				Custom struct {
					Commentlen struct {
						Settings any `yaml:"settings"`
					} `yaml:"commentlen"`
				} `yaml:"custom"`
			} `yaml:"settings"`
		} `yaml:"linters"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc.Linters.Settings.Custom.Commentlen.Settings, nil
}

func reencode(settings any) (analyzer.Settings, error) {
	if settings == nil {
		return analyzer.Settings{}, nil
	}
	out, err := yaml.Marshal(settings)
	if err != nil {
		return analyzer.Settings{}, err
	}
	return decodeSettings(out, "extracted")
}
