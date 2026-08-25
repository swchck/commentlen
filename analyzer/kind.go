package analyzer

// Kind is the classification of a comment. Every comment in a file belongs to
// exactly one kind, and all size limits are configured per kind.
type Kind uint8

// The comment kinds recognized by the linter.
const (
	// KindPackage is the package doc comment, the one directly above `package x`.
	KindPackage Kind = iota
	// KindExported is a doc comment on an exported top-level declaration.
	KindExported
	// KindUnexported is a doc comment on an unexported top-level declaration.
	KindUnexported
	// KindField is a comment on a struct field, interface method or group element.
	KindField
	// KindInline is a comment on its own line inside a function body.
	KindInline
	// KindTrailing is a same-line comment after code inside a function body.
	KindTrailing
	// KindOther is everything else: floating, inside literals, end of file.
	KindOther

	numKinds = int(KindOther) + 1
)

var kindNames = [numKinds]string{
	KindPackage:    "package",
	KindExported:   "exported",
	KindUnexported: "unexported",
	KindField:      "field",
	KindInline:     "inline",
	KindTrailing:   "trailing",
	KindOther:      "other",
}

// String returns the configuration key of the kind.
func (k Kind) String() string {
	if int(k) >= numKinds {
		return "unknown"
	}
	return kindNames[k]
}

// docKind reports whether the kind is a documentation comment, the only place
// where godoc formatting rules such as pre-formatted blocks apply.
func docKind(k Kind) bool {
	switch k {
	case KindPackage, KindExported, KindUnexported, KindField:
		return true
	}
	return false
}

func parseKind(s string) (Kind, bool) {
	for i, name := range kindNames {
		if name == s {
			return Kind(i), true
		}
	}
	return 0, false
}

// KindNames returns every valid kind key, in declaration order.
func KindNames() []string {
	out := make([]string, 0, numKinds)
	out = append(out, kindNames[:]...)
	return out
}
