package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

const classifySrc = `// Package p is a package doc.
package p

import "fmt" // an import gets a field comment

// Exported is exported.
func Exported() {
	// inline note
	fmt.Println("x") // trailing note
}

// unexported is not.
func unexported() {}

// Config groups the knobs.
type Config struct {
	// Timeout in milliseconds.
	Timeout int
	retries int // how many
}

// A group doc.
var (
	// First element.
	First = 1
	second = 2
)

func (c *Config) Exported() {}

// hidden is a method on an unexported receiver.
func (c *config) Hidden() {}

type config struct{}

// floating between declarations

func withLocal() {
	// x holds the answer
	var x = 42
	_ = x
}
`

func TestClassify(t *testing.T) {
	cfg, err := newConfig(Settings{})
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", classifySrc, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	tf := fset.File(file.Pos())
	fc := newFileContext(cfg, fset, tf, []byte(classifySrc), file, &scratch{})

	want := map[int]Kind{
		1:  KindPackage,    // package doc
		4:  KindField,      // import trailing comment
		6:  KindExported,   // func Exported
		8:  KindInline,     // inline note
		9:  KindTrailing,   // trailing note
		12: KindUnexported, // func unexported
		15: KindExported,   // type Config
		17: KindField,      // Timeout doc
		19: KindField,      // retries trailing
		22: KindExported,   // var group doc, First is exported
		24: KindField,      // First element
		31: KindUnexported, // method on an unexported receiver
		36: KindOther,      // floating comment
		39: KindInline,     // local var doc inside a function body
	}

	got := map[int]Kind{}
	for _, group := range file.Comments {
		kind, _ := fc.classify(group)
		got[fset.Position(group.Pos()).Line] = kind
	}

	for line, wantKind := range want {
		if gotKind, ok := got[line]; !ok {
			t.Errorf("line %d: no comment classified", line)
		} else if gotKind != wantKind {
			t.Errorf("line %d: got kind %s, want %s", line, gotKind, wantKind)
		}
	}
	for line, gotKind := range got {
		if _, ok := want[line]; !ok {
			t.Errorf("line %d: unexpected comment classified as %s", line, gotKind)
		}
	}
}

func TestFuncExported(t *testing.T) {
	src := `package p
func Plain() {}
func hidden() {}
func (E) Method() {}
func (*E) PtrMethod() {}
func (u unexported) Method() {}
func (g Generic[T]) Method() {}
type E struct{}
type unexported struct{}
type Generic[T any] struct{}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string][]bool{}
	for _, decl := range file.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			got[fd.Name.Name] = append(got[fd.Name.Name], funcExported(fd))
		}
	}
	if len(got) == 0 {
		t.Fatal("no functions found")
	}
	if !got["Plain"][0] {
		t.Error("Plain should be exported")
	}
	if got["hidden"][0] {
		t.Error("hidden should not be exported")
	}
	if !got["PtrMethod"][0] {
		t.Error("(*E).PtrMethod should be exported")
	}
	methods := got["Method"]
	if len(methods) != 3 {
		t.Fatalf("got %d Method decls, want 3", len(methods))
	}
	if !methods[0] || methods[1] || !methods[2] {
		t.Errorf("Method exportedness: got %v, want [true false true]", methods)
	}
}
