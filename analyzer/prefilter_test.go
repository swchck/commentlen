package analyzer

import (
	"regexp"
	"testing"
)

// TestPrefilterNeverHidesAMatch is the safety property the optimization rests
// on: whatever the regexp matches, the prefilter must admit.
func TestPrefilterNeverHidesAMatch(t *testing.T) {
	patterns := []string{
		`(?i)\b(note that|please note|it'?s (important|worth) (to note|noting))\b`,
		`(?i)\bnot just [^,.]{1,40}, but\b`,
		`(?i)\b(for performance reasons|to ensure correctness)\b`,
		metadataPattern,
		reportsWhetherPattern,
		`(?m)(?:^|\s)(TODO|FIXME|HACK)\b\s*[:(]?`,
		`^SPDX-`,
		`obviously`,
		`(?i)(foo|bar)baz`,
		`a+bc`,
		`x?ylophone`,
		`(?i)ПРИМЕЧАНИЕ`, // non-ASCII: the filter must give up, not mis-filter
	}
	texts := []string{
		"",
		"Note that this blocks",
		"note THAT this blocks",
		"it's important to note the ordering",
		"handles not just the header, but the body",
		"cached for performance reasons",
		"@author someone",
		"Author: someone",
		"history = old",
		"Ready returns true if the thing is ready",
		"returns FALSE when empty",
		"TODO: later",
		"a TODO(user) marker",
		"the todos of a user",
		"SPDX-License-Identifier: MIT",
		"obviously works",
		"OBVIOUSLY works",
		"foobaz and barbaz",
		"aaabc",
		"ylophone",
		"xylophone",
		"примечание в комментарии",
		"ПРИМЕЧАНИЕ в комментарии",
		"plain prose with nothing special in it at all",
	}

	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			t.Fatalf("compile %q: %v", pattern, err)
		}
		pre := newPrefilter(pattern)
		for _, text := range texts {
			if !re.MatchString(text) {
				continue
			}
			if !pre.mayMatch(foldASCII(nil, text)) {
				t.Errorf("prefilter hid a match:\npattern %q\ntext    %q\nanchors %q", pattern, text, pre.anchors)
			}
		}
	}
}

func TestPrefilterDoesFilter(t *testing.T) {
	// the point of the exercise: ordinary prose is rejected without the regexp
	pre := newPrefilter(`(?i)\b(note that|please note)\b`)
	if len(pre.anchors) == 0 {
		t.Fatal("expected anchors to be derived from an alternation of literals")
	}
	if pre.mayMatch(foldASCII(nil, "a comment with no hedging in it")) {
		t.Error("prose without any anchor should be filtered out")
	}
	if !pre.mayMatch(foldASCII(nil, "Please Note the ordering")) {
		t.Error("prose containing an anchor must pass")
	}

	// a pattern with no provable literal must not filter anything
	open := newPrefilter(`\w+`)
	if len(open.anchors) != 0 {
		t.Errorf("expected no anchors for %q, got %q", `\w+`, open.anchors)
	}
	if !open.mayMatch(foldASCII(nil, "anything")) {
		t.Error("a filter without anchors must admit everything")
	}
}

func FuzzPrefilter(f *testing.F) {
	f.Add(`(?i)note that`, "Note that x")
	f.Add(`(a|b)cd`, "bcd")
	f.Add(metadataPattern, "@author me")

	f.Fuzz(func(t *testing.T, pattern, text string) {
		re, err := regexp.Compile(pattern)
		if err != nil {
			t.Skip()
		}
		if !re.MatchString(text) {
			return
		}
		if !newPrefilter(pattern).mayMatch(foldASCII(nil, text)) {
			t.Fatalf("prefilter hid a match: pattern=%q text=%q", pattern, text)
		}
	})
}
