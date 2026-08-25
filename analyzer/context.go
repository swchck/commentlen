package analyzer

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"
	"unicode"
)

// docTarget is the declaration a doc comment belongs to.
type docTarget struct {
	kind Kind
	// name is the documented symbol, empty for groups. Only godoc reads it.
	name string
	// codeStart and codeEnd delimit the declaration, for the ratio check.
	codeStart, codeEnd token.Pos
	// boolFunc marks a function whose single result is bool.
	boolFunc bool
	// testFunc marks a test, benchmark or fuzz target, which godoc never shows.
	testFunc bool
}

type span struct {
	start, end token.Pos
}

type funcSpan struct {
	span
	name string
}

// describe names the function in a diagnostic.
func (f *funcSpan) describe() string {
	if f.name == "" {
		return "function literal"
	}
	return "func " + f.name
}

type blockSpan struct {
	span
	list []ast.Stmt
}

// fileContext is everything the checks need about one file, collected in a
// single AST walk.
type fileContext struct {
	fset *token.FileSet
	tf   *token.File
	src  []byte
	file *ast.File
	cfg  *config

	docs   map[*ast.CommentGroup]docTarget
	funcs  []funcSpan
	blocks []blockSpan

	// testFile marks a _test.go file, where Test/Benchmark/Fuzz functions are
	// exported by form only.
	testFile bool

	disabled     []span
	fileDisabled bool
}

func newFileContext(cfg *config, fset *token.FileSet, tf *token.File, src []byte, file *ast.File, sc *scratch) *fileContext {
	if sc.docs == nil {
		sc.docs = make(map[*ast.CommentGroup]docTarget, len(file.Decls)+8)
	} else {
		clear(sc.docs)
	}
	fc := &fileContext{
		fset:     fset,
		tf:       tf,
		src:      src,
		file:     file,
		cfg:      cfg,
		docs:     sc.docs,
		funcs:    sc.funcs[:0],
		blocks:   sc.blocks[:0],
		disabled: sc.disabled[:0],
	}
	fc.testFile = strings.HasSuffix(tf.Name(), "_test.go")
	fc.collectDirectives()
	if !fc.fileDisabled {
		fc.walk()
		if len(fc.disabled) > 1 {
			sort.Slice(fc.disabled, func(i, j int) bool { return fc.disabled[i].start < fc.disabled[j].start })
		}
	}
	// hand the grown buffers back for the next file in the package
	sc.funcs, sc.blocks, sc.disabled = fc.funcs, fc.blocks, fc.disabled
	return fc
}

// collectDirectives finds the linter's own directives before any check runs, so
// a disable region also covers the comment that opens it.
func (fc *fileContext) collectDirectives() {
	var open token.Pos
	hasOpen := false

	for _, group := range fc.file.Comments {
		for _, c := range group.List {
			switch directiveAction(c.Text) {
			case actionDisableFile:
				fc.fileDisabled = true
				return
			case actionDisable:
				if !hasOpen {
					open, hasOpen = c.Pos(), true
				}
			case actionEnable:
				if hasOpen {
					fc.disabled = append(fc.disabled, span{open, c.End()})
					hasOpen = false
				}
			}
		}
	}
	if hasOpen {
		fc.disabled = append(fc.disabled, span{open, token.Pos(fc.tf.Base() + fc.tf.Size())})
	}
}

func (fc *fileContext) walk() {
	if fc.file.Doc != nil {
		fc.addDoc(fc.file.Doc, docTarget{
			kind:      KindPackage,
			name:      fc.file.Name.Name,
			codeStart: fc.file.Package,
			codeEnd:   fc.file.Name.End(),
		})
	}

	ast.Inspect(fc.file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			fc.addFunc(n)

		case *ast.FuncLit:
			if n.Body != nil {
				fc.funcs = append(fc.funcs, funcSpan{span: span{n.Body.Lbrace, n.Body.Rbrace}})
			}

		case *ast.GenDecl:
			fc.addGenDecl(n)

		case *ast.ValueSpec:
			fc.addSpec(n.Doc, n.Comment, specName(n.Names), n.Pos(), n.End())

		case *ast.TypeSpec:
			fc.addSpec(n.Doc, n.Comment, n.Name.Name, n.Pos(), n.End())

		case *ast.ImportSpec:
			fc.addSpec(n.Doc, n.Comment, "", n.Pos(), n.End())

		case *ast.Field:
			fc.addSpec(n.Doc, n.Comment, specName(n.Names), n.Pos(), n.End())

		case *ast.BlockStmt:
			if fc.cfg.needsStmts {
				fc.blocks = append(fc.blocks, blockSpan{span{n.Lbrace, n.Rbrace}, n.List})
			}

		case *ast.CaseClause:
			if fc.cfg.needsStmts && len(n.Body) > 0 {
				fc.blocks = append(fc.blocks, blockSpan{span{n.Colon, n.End()}, n.Body})
			}

		case *ast.CommClause:
			if fc.cfg.needsStmts && len(n.Body) > 0 {
				fc.blocks = append(fc.blocks, blockSpan{span{n.Colon, n.End()}, n.Body})
			}
		}
		return true
	})

	// ast.Inspect is pre-order, so spans arrive nearly sorted; the checks index
	// them by position and need the order to be exact.
	if len(fc.funcs) > 1 && !sort.SliceIsSorted(fc.funcs, func(i, j int) bool { return fc.funcs[i].start < fc.funcs[j].start }) {
		sort.Slice(fc.funcs, func(i, j int) bool { return fc.funcs[i].start < fc.funcs[j].start })
	}
	if len(fc.blocks) > 1 && !sort.SliceIsSorted(fc.blocks, func(i, j int) bool { return fc.blocks[i].start < fc.blocks[j].start }) {
		sort.Slice(fc.blocks, func(i, j int) bool { return fc.blocks[i].start < fc.blocks[j].start })
	}
}

func (fc *fileContext) addFunc(fd *ast.FuncDecl) {
	if fd.Body != nil {
		fc.funcs = append(fc.funcs, funcSpan{span{fd.Body.Lbrace, fd.Body.Rbrace}, fd.Name.Name})
	}
	if fd.Doc == nil {
		return
	}
	kind := KindUnexported
	if funcExported(fd) {
		kind = KindExported
	}
	fc.addDoc(fd.Doc, docTarget{
		kind:      kind,
		name:      fd.Name.Name,
		codeStart: fd.Pos(),
		codeEnd:   fd.End(),
		boolFunc:  returnsBool(fd.Type),
		testFunc:  fc.testFile && testFuncName(fd.Name.Name),
	})
}

func (fc *fileContext) addGenDecl(gd *ast.GenDecl) {
	if gd.Doc == nil {
		return
	}
	name, exported := genDeclSubject(gd)
	kind := KindUnexported
	if exported {
		kind = KindExported
	}
	if gd.Lparen.IsValid() && len(gd.Specs) > 1 {
		name = "" // a group doc describes the group, not one symbol
	}
	fc.addDoc(gd.Doc, docTarget{
		kind:      kind,
		name:      name,
		codeStart: gd.Pos(),
		codeEnd:   gd.End(),
	})
}

func (fc *fileContext) addSpec(doc, line *ast.CommentGroup, name string, start, end token.Pos) {
	if doc == nil && line == nil {
		return
	}
	t := docTarget{kind: KindField, name: name, codeStart: start, codeEnd: end}
	if doc != nil {
		fc.addDoc(doc, t)
	}
	if line != nil {
		fc.addDoc(line, t)
	}
}

func (fc *fileContext) addDoc(group *ast.CommentGroup, t docTarget) {
	// a GenDecl holding a single spec exposes the same comment group as both
	// decl.Doc and spec.Doc; the declaration classification wins
	if _, exists := fc.docs[group]; exists {
		return
	}
	fc.docs[group] = t
	// the ignored range has to start at the comment, not at the declaration: a
	// doc comment sits above the code it documents
	if groupHasIgnore(group) {
		fc.disabled = append(fc.disabled, span{group.Pos(), t.codeEnd})
	}
}

// classify returns a comment's kind and the declaration it documents. Function
// bodies come first: a local `var x` carries a spec doc comment, but reads inline.
func (fc *fileContext) classify(group *ast.CommentGroup) (Kind, docTarget) {
	if fc.enclosingFunc(group.Pos()) >= 0 {
		if fc.isTrailing(group.List[0]) {
			return KindTrailing, docTarget{kind: KindTrailing}
		}
		return KindInline, docTarget{kind: KindInline}
	}
	if t, ok := fc.docs[group]; ok {
		return t.kind, t
	}
	if fc.isTrailing(group.List[0]) {
		return KindTrailing, docTarget{kind: KindTrailing}
	}
	return KindOther, docTarget{kind: KindOther}
}

// enclosingFunc returns the index of the innermost function body containing pos,
// or -1. Spans are sorted by start, so the last candidate is the innermost one.
func (fc *fileContext) enclosingFunc(pos token.Pos) int {
	i := sort.Search(len(fc.funcs), func(i int) bool { return fc.funcs[i].start > pos })
	for i--; i >= 0; i-- {
		if fc.funcs[i].end >= pos {
			return i
		}
	}
	return -1
}

// isTrailing reports whether code precedes the comment on its own line.
func (fc *fileContext) isTrailing(c *ast.Comment) bool {
	off := fc.tf.Offset(c.Pos())
	if off <= 0 || off > len(fc.src) {
		return false
	}
	lineStart := off - (fc.tf.Position(c.Pos()).Column - 1)
	if lineStart < 0 {
		lineStart = 0
	}
	for _, b := range fc.src[lineStart:off] {
		if b != ' ' && b != '\t' {
			return true
		}
	}
	return false
}

func (fc *fileContext) isDisabled(pos token.Pos) bool {
	if fc.fileDisabled {
		return true
	}
	for _, d := range fc.disabled {
		if d.start > pos {
			return false
		}
		if pos <= d.end {
			return true
		}
	}
	return false
}

func specName(names []*ast.Ident) string {
	if len(names) == 0 {
		return ""
	}
	return names[0].Name
}

func genDeclSubject(gd *ast.GenDecl) (name string, exported bool) {
	for _, spec := range gd.Specs {
		var idents []*ast.Ident
		switch s := spec.(type) {
		case *ast.ValueSpec:
			idents = s.Names
		case *ast.TypeSpec:
			idents = []*ast.Ident{s.Name}
		}
		for _, id := range idents {
			if name == "" {
				name = id.Name
			}
			if id.IsExported() {
				return id.Name, true
			}
		}
	}
	return name, false
}

// funcExported follows the golint rule: a method belongs to the exported API
// only when its receiver type is exported too.
func funcExported(fd *ast.FuncDecl) bool {
	if !fd.Name.IsExported() {
		return false
	}
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return true
	}
	return typeExported(fd.Recv.List[0].Type)
}

func typeExported(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return typeExported(t.X)
	case *ast.IndexExpr: // generic receiver: Foo[T]
		return typeExported(t.X)
	case *ast.IndexListExpr:
		return typeExported(t.X)
	case *ast.Ident:
		return t.IsExported()
	}
	return false
}

// testFuncName reports whether the name is one `go test` picks up: TestXxx,
// BenchmarkXxx, FuzzXxx or ExampleXxx, where Xxx does not start lower case.
func testFuncName(name string) bool {
	for _, prefix := range [...]string{"Test", "Benchmark", "Fuzz", "Example"} {
		rest, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		if rest == "" {
			return true
		}
		if r := []rune(rest)[0]; !unicode.IsLower(r) {
			return true
		}
	}
	return false
}

func returnsBool(ft *ast.FuncType) bool {
	if ft.Results == nil || len(ft.Results.List) != 1 {
		return false
	}
	id, ok := ft.Results.List[0].Type.(*ast.Ident)
	return ok && id.Name == "bool"
}
