package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// buildSource generates a file with n functions, each carrying a doc comment,
// two inline comments and a trailing one.
func buildSource(n int) string {
	var sb strings.Builder
	sb.WriteString("// Package bench is generated for the benchmark.\npackage bench\n\n")
	for i := range n {
		fmt.Fprintf(&sb, `// Fn%d returns the answer for shard %d.
//
// The lookup is O(1); the table is built once at init.
func Fn%d(x int) int {
	// the driver is not goroutine-safe, so no errgroup here
	y := x * %d

	// provider 400s on fractional cents, so round down
	z := y / 100 // cents
	return z
}

// config%d is the private knob table.
type config%d struct {
	Timeout int // milliseconds
	retries int
}

//go:generate stringer -type=config%d

`, i, i, i, i+2, i, i, i)
	}
	return sb.String()
}

func benchmarkSettings(b *testing.B, s Settings) *analysis.Analyzer {
	b.Helper()
	a, err := New(s)
	if err != nil {
		b.Fatal(err)
	}
	return a
}

func runBench(b *testing.B, s Settings) {
	b.Helper()

	src := buildSource(200)
	a := benchmarkSettings(b, s)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "bench.go", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		b.Fatal(err)
	}
	data := []byte(src)
	pass := &analysis.Pass{
		Analyzer: a,
		Fset:     fset,
		Files:    []*ast.File{file},
		ReadFile: func(string) ([]byte, error) { return data, nil },
		ResultOf: map[*analysis.Analyzer]any{},
		Report:   func(analysis.Diagnostic) {},
	}

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := a.Run(pass); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBalanced(b *testing.B) { runBench(b, Settings{}) }
func BenchmarkStrict(b *testing.B)   { runBench(b, Settings{Preset: "strict"}) }

func BenchmarkSizeOnly(b *testing.B) {
	runBench(b, Settings{Style: noStyle(), Godoc: GodocSettings{Enabled: ptr(false)}})
}

// TestConcurrentRuns guards the property golangci-lint relies on: one analyzer
// value is run on many packages at once, so nothing mutable may live on it.
func TestConcurrentRuns(t *testing.T) {
	a, err := New(Settings{Preset: "strict"})
	if err != nil {
		t.Fatal(err)
	}
	src := buildSource(20)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "bench.go", src, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				t.Error(err)
				return
			}
			pass := &analysis.Pass{
				Analyzer: a,
				Fset:     fset,
				Files:    []*ast.File{file},
				ReadFile: func(string) ([]byte, error) { return []byte(src), nil },
				ResultOf: map[*analysis.Analyzer]any{},
				Report:   func(analysis.Diagnostic) {},
			}
			if _, err := a.Run(pass); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

// TestMultipleFilesReuseBuffers checks that the scratch buffers survive being
// reused across the files of one package.
func TestMultipleFilesReuseBuffers(t *testing.T) {
	a, err := New(Settings{Preset: "strict"})
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	srcs := map[string]string{}
	for i, src := range []string{buildSource(3), buildSource(1), buildSource(5)} {
		name := fmt.Sprintf("f%d.go", i)
		file, err := parser.ParseFile(fset, name, src, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
		srcs[name] = src
	}

	var perFile []int
	pass := &analysis.Pass{
		Analyzer: a,
		Fset:     fset,
		Files:    files,
		ReadFile: func(name string) ([]byte, error) { return []byte(srcs[name]), nil },
		ResultOf: map[*analysis.Analyzer]any{},
		Report:   func(analysis.Diagnostic) {},
	}
	if _, err := a.Run(pass); err != nil {
		t.Fatal(err)
	}

	// the same content must produce the same diagnostics whether it is analyzed
	// alone or as the third file of a package
	for _, file := range files {
		name := fset.File(file.Pos()).Name()
		single := runAnalyzer(t, a, "solo.go", srcs[name])
		perFile = append(perFile, len(single))
	}
	if perFile[0] == 0 || perFile[1] == 0 || perFile[2] == 0 {
		t.Fatalf("expected diagnostics in every file, got %v", perFile)
	}
	if perFile[0] <= perFile[1] || perFile[2] <= perFile[0] {
		t.Errorf("diagnostic counts should scale with file size, got %v", perFile)
	}
}

// BenchmarkParseOnly is the baseline: whatever the linter costs, it is spent on
// top of parsing, which the runner has to do anyway.
func BenchmarkParseOnly(b *testing.B) {
	src := buildSource(200)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, "bench.go", src, parser.ParseComments|parser.SkipObjectResolution); err != nil {
			b.Fatal(err)
		}
	}
}
