package main

import (
	"strings"
	"testing"
)

func TestDecodeSettings(t *testing.T) {
	yaml := `
preset: strict
skip-generated: true
exclude-files:
  - '/vendor/'
kinds:
  inline:
    max-lines: 2
    max-width: 100
  trailing:
    forbidden: true
max-inline-per-func: 2
ratio:
  enabled: true
  max: 1.0
  kinds: [inline, unexported]
style:
  tags: [TODO, FIXME]
  patterns-extra:
    - pattern: '\b[A-Z]{2,10}-\d{1,6}\b'
      message: no ticket keys in comments
godoc:
  enabled: true
  scope: exported
`
	s, err := decodeSettings([]byte(yaml), "test.yml")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Preset != "strict" {
		t.Errorf("preset = %q, want strict", s.Preset)
	}
	if got := s.Kinds["inline"].MaxLines; got == nil || *got != 2 {
		t.Errorf("inline max-lines = %v, want 2", got)
	}
	if got := s.Kinds["trailing"].Forbidden; got == nil || !*got {
		t.Errorf("trailing forbidden = %v, want true", got)
	}
	if len(s.Style.PatternsExtra) != 1 || s.Style.PatternsExtra[0].Message != "no ticket keys in comments" {
		t.Errorf("patterns-extra = %+v", s.Style.PatternsExtra)
	}
	if len(s.Ratio.Kinds) != 2 {
		t.Errorf("ratio.kinds = %v, want two entries", s.Ratio.Kinds)
	}
}

func TestDecodeSettingsRejectsUnknownKeys(t *testing.T) {
	_, err := decodeSettings([]byte("max-line: 3\n"), "test.yml")
	if err == nil {
		t.Fatal("want an error for an unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "max-line") {
		t.Errorf("error should name the offending key, got %v", err)
	}
}

func TestDecodeSettingsErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "not a mapping", in: "- a\n- b\n", want: "want a mapping"},
		{name: "broken yaml", in: "a:\n\t- b\n", want: "parse"},
		{name: "wrong type", in: "max-inline-per-func: many\n", want: "wrong type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeSettings([]byte(tt.in), "test.yml"); err == nil {
				t.Fatal("want an error, got nil")
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestDecodeEmptySettings(t *testing.T) {
	s, err := decodeSettings(nil, "test.yml")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Preset != "" {
		t.Errorf("empty config should stay empty, got %+v", s)
	}
}
