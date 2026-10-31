package ast

import (
	"math/big"
	"strings"
	"unicode"
)

// Position holds source line and column numbers (1-indexed).
type Position struct {
	Line int
	Col  int
}

// Node represents any AST node in Les.
type Node interface {
	node()
	Pos() Position
}

// Expr represents an expression node.
type Expr interface {
	Node
	expr()
}

// Stmt represents a statement node.
type Stmt interface {
	Node
	stmt()
}

// Program is the root of the AST.
type Program struct {
	Position   Position
	Filename   string
	ModuleName string
	Body       []Stmt
}

func (p *Program) node()         {}
func (p *Program) Pos() Position { return p.Position }

// ModuleDecl represents explicit module declaration at file top: module <name>.
type ModuleDecl struct {
	Position Position
	Name     string
}

func (m *ModuleDecl) node()         {}
func (m *ModuleDecl) stmt()         {}
func (m *ModuleDecl) Pos() Position { return m.Position }

// ImportStmt represents module import: import <name> or import "<path>".
type ImportStmt struct {
	Position Position
	Path     string
	Alias    string
}

func (imp *ImportStmt) node()         {}
func (imp *ImportStmt) stmt()         {}
func (imp *ImportStmt) Pos() Position { return imp.Position }

// Identifier represents variable or function name.
type Identifier struct {
	Position Position
	Name     string
}

func (i *Identifier) node()         {}
func (i *Identifier) expr()         {}
func (i *Identifier) Pos() Position { return i.Position }

// ModuleAccessExpr represents @module.Member expression.
type ModuleAccessExpr struct {
	Position Position
	Module   string
	Member   string
}

func (m *ModuleAccessExpr) node()         {}
func (m *ModuleAccessExpr) expr()         {}
func (m *ModuleAccessExpr) Pos() Position { return m.Position }

// MemberAccessExpr represents dot access on an expression: target.member.
type MemberAccessExpr struct {
	Position Position
	Target   Expr
	Member   string
}

func (m *MemberAccessExpr) node()         {}
func (m *MemberAccessExpr) expr()         {}
func (m *MemberAccessExpr) Pos() Position { return m.Position }

// NamedArgExpr represents name: expr keyword argument in call / struct constructor.
type NamedArgExpr struct {
	Position Position
	Name     string
	Value    Expr
}

func (n *NamedArgExpr) node()         {}
func (n *NamedArgExpr) expr()         {}
func (n *NamedArgExpr) Pos() Position { return n.Position }

// IntLit represents integer literal.
type IntLit struct {
	Position Position
	Value    int64
	BigValue *big.Int // non-nil only when literal exceeds int64 range
}

func (i *IntLit) node()         {}
func (i *IntLit) expr()         {}
func (i *IntLit) Pos() Position { return i.Position }

// FloatLit represents floating point literal.
type FloatLit struct {
	Position Position
	Value    float64
}

func (f *FloatLit) node()         {}
func (f *FloatLit) expr()         {}
func (f *FloatLit) Pos() Position { return f.Position }

// StringLit represents string literal.
type StringLit struct {
	Position Position
	Value    string
}

func (s *StringLit) node()         {}
func (s *StringLit) expr()         {}
func (s *StringLit) Pos() Position { return s.Position }

// BoolLit represents boolean literal.
type BoolLit struct {
	Position Position
	Value    bool
}

func (b *BoolLit) node()         {}
func (b *BoolLit) expr()         {}
func (b *BoolLit) Pos() Position { return b.Position }

// NilLit represents nil literal.
type NilLit struct {
	Position Position
}

func (n *NilLit) node()         {}
func (n *NilLit) expr()         {}
func (n *NilLit) Pos() Position { return n.Position }

// RegexLit represents regex literal, e.g. /pattern/flags.
type RegexLit struct {
	Position Position
	Pattern  string
	Flags    string
}

func (r *RegexLit) node()         {}
func (r *RegexLit) expr()         {}
func (r *RegexLit) Pos() Position { return r.Position }

// TupleLit represents tuple literal, e.g. (1, 2, 3), (), (1,).
type TupleLit struct {
	Position   Position
	Elements   []Expr
	IsVariadic bool
}

func (t *TupleLit) node()         {}
func (t *TupleLit) expr()         {}
func (t *TupleLit) Pos() Position { return t.Position }

// SpreadExpr represents tuple spreading inside a tuple literal, e.g. ...expr.
type SpreadExpr struct {
	Position Position
	Expr     Expr
}

func (s *SpreadExpr) node()         {}
func (s *SpreadExpr) expr()         {}
func (s *SpreadExpr) Pos() Position { return s.Position }

// DictEntry represents key-value pair in dictionary.
type DictEntry struct {
	Position Position
	Key      Expr
	Value    Expr
}

// DictLit represents dictionary literal, e.g. {"a": 1}.
type DictLit struct {
	Position Position
	Entries  []DictEntry
}

func (d *DictLit) node()         {}
func (d *DictLit) expr()         {}
func (d *DictLit) Pos() Position { return d.Position }

// IndexExpr represents indexing with [], e.g. target[index] or single type argument target[T].
type IndexExpr struct {
	Position Position
	Target   Expr
	Index    Expr
	TypeArg  string
}

func (idx *IndexExpr) node()         {}
func (idx *IndexExpr) expr()         {}
func (idx *IndexExpr) Pos() Position { return idx.Position }

// TypeInstExpr represents explicit type argument instantiation, e.g. target[T1, T2] or target[-> Int].
type TypeInstExpr struct {
	Position Position
	Target   Expr
	TypeArgs []string
}

func (t *TypeInstExpr) node()         {}
func (t *TypeInstExpr) expr()         {}
func (t *TypeInstExpr) Pos() Position { return t.Position }

// BinaryExpr represents binary operations (+, -, *, /, %, ==, !=, <, <=, >, >=).
type BinaryExpr struct {
	Position Position
	Op       string
	Left     Expr
	Right    Expr
}

func (b *BinaryExpr) node()         {}
func (b *BinaryExpr) expr()         {}
func (b *BinaryExpr) Pos() Position { return b.Position }

// UnaryExpr represents unary operations (-, !).
type UnaryExpr struct {
	Position Position
	Op       string
	Expr     Expr
}

func (u *UnaryExpr) node()         {}
func (u *UnaryExpr) expr()         {}
func (u *UnaryExpr) Pos() Position { return u.Position }

// ForceExpr represents strict evaluation postfix operator: expr!
type ForceExpr struct {
	Position Position
	Expr     Expr
}

func (f *ForceExpr) node()         {}
func (f *ForceExpr) expr()         {}
func (f *ForceExpr) Pos() Position { return f.Position }

// TernaryExpr represents C-style ternary expression: cond ? then : else.
type TernaryExpr struct {
	Position Position
	Cond     Expr
	Then     Expr
	Else     Expr
}

func (t *TernaryExpr) node()         {}
func (t *TernaryExpr) expr()         {}
func (t *TernaryExpr) Pos() Position { return t.Position }

// IsExpr represents type check expression: expr is Type.
type IsExpr struct {
	Position  Position
	Expr      Expr
	TypeAnnot string
}

func (i *IsExpr) node()         {}
func (i *IsExpr) expr()         {}
func (i *IsExpr) Pos() Position { return i.Position }

// TuplePattern represents unpacked tuple pattern, e.g. (h, ...t) or (a, b).
type TuplePattern struct {
	Position Position
	Elements []string // variable names before rest, e.g. ["h"]
	RestName string   // variable name after '...', e.g. "t", or empty if no rest
}

func (tp *TuplePattern) node()         {}
func (tp *TuplePattern) Pos() Position { return tp.Position }

// MatchCase represents a single case in a match expression: case Pattern: Body
type MatchCase struct {
	Position     Position
	Pattern      string        // type name ("Int", "_") or representation
	RegexPattern *RegexLit     // non-nil if regex pattern in case
	AsName       string        // variable name bound via 'as <ident>', or empty
	TuplePattern *TuplePattern // non-nil if tuple unpacking pattern (e.g. (h, ...t))
	Body         []Stmt
}

// MatchExpr represents pattern match expression: match expr: case ...
type MatchExpr struct {
	Position Position
	Target   Expr
	Cases    []MatchCase
}

func (m *MatchExpr) node()         {}
func (m *MatchExpr) expr()         {}
func (m *MatchExpr) Pos() Position { return m.Position }

// CallExpr represents function call. Arg is always a TupleLit expression (or single expr desugared).
type CallExpr struct {
	Position Position
	Callee   Expr
	Args     []Expr
	Block    *BlockLiteral // optional trailing do |x|: block
}

func (c *CallExpr) node()         {}
func (c *CallExpr) expr()         {}
func (c *CallExpr) Pos() Position { return c.Position }

// BlockLiteral represents do |params|: body block closure.
type BlockLiteral struct {
	Position Position
	Params   []FuncParam
	Body     []Stmt
}

func (b *BlockLiteral) node()         {}
func (b *BlockLiteral) expr()         {}
func (b *BlockLiteral) Pos() Position { return b.Position }

// LambdaExpr represents anonymous lambda function: |params|: expr or |params|: block
type LambdaExpr struct {
	Position Position
	Params   []FuncParam
	Body     []Stmt
	Expr     Expr // single-line expression if non-nil
}

func (l *LambdaExpr) node()         {}
func (l *LambdaExpr) expr()         {}
func (l *LambdaExpr) Pos() Position { return l.Position }

// VarDecl represents variable assignment with optional type annotation.
type VarDecl struct {
	Position  Position
	Name      string
	TypeAnnot string
	Value     Expr
}

func (v *VarDecl) node()         {}
func (v *VarDecl) stmt()         {}
func (v *VarDecl) Pos() Position { return v.Position }

// UnpackDecl represents unpacked tuple assignment, e.g. (h, ...t) = stream.
type UnpackDecl struct {
	Position Position
	Pattern  *TuplePattern
	Value    Expr
}

func (u *UnpackDecl) node()         {}
func (u *UnpackDecl) stmt()         {}
func (u *UnpackDecl) Pos() Position { return u.Position }

// ReturnStmt represents explicit return.
type ReturnStmt struct {
	Position Position
	Value    Expr
}

func (r *ReturnStmt) node()         {}
func (r *ReturnStmt) stmt()         {}
func (r *ReturnStmt) Pos() Position { return r.Position }

// BreakStmt represents breaking out of a closure (lambda/block) or loop.
type BreakStmt struct {
	Position Position
	Value    Expr
}

func (b *BreakStmt) node()         {}
func (b *BreakStmt) stmt()         {}
func (b *BreakStmt) Pos() Position { return b.Position }

// ContinueStmt represents continuing to the next iteration of a loop.
type ContinueStmt struct {
	Position Position
}

func (c *ContinueStmt) node()         {}
func (c *ContinueStmt) stmt()         {}
func (c *ContinueStmt) Pos() Position { return c.Position }

// ForStmt represents for loop: for [cond]: body. Body is executed as an implicit closure per iteration.
type ForStmt struct {
	Position Position
	Cond     Expr // nil for infinite loop `for:`
	Body     []Stmt
}

func (f *ForStmt) node()         {}
func (f *ForStmt) stmt()         {}
func (f *ForStmt) Pos() Position { return f.Position }

// ExprStmt represents expression evaluated as statement.
type ExprStmt struct {
	Position Position
	Value    Expr
}

func (e *ExprStmt) node()         {}
func (e *ExprStmt) stmt()         {}
func (e *ExprStmt) Pos() Position { return e.Position }

// TypeParam represents a type parameter with optional constraint, e.g. T or T: Constraint.
type TypeParam struct {
	Position   Position
	Name       string
	Constraint string // Empty if unconstrained; otherwise type annotation (e.g. "@io.ReadCloser", "Int | Float")
}

func (tp TypeParam) Pos() Position { return tp.Position }

// FuncParam represents function parameter with optional type annotation.
type FuncParam struct {
	Position     Position
	Name         string
	TypeAnnot    string
	DefaultValue Expr // nil if required
	IsVariadic   bool
}

// FuncDef represents function definition block: def name[T](params): body.
type FuncDef struct {
	Position   Position
	Name       string
	TypeParams []TypeParam
	Params     []FuncParam
	BlockParam *FuncParam // optional &block parameter
	ReturnType string
	Body       []Stmt
}

func (f *FuncDef) node()         {}
func (f *FuncDef) stmt()         {}
func (f *FuncDef) Pos() Position { return f.Position }

// IfStmt represents if-else block statement.
type IfStmt struct {
	Position Position
	Cond     Expr
	ThenBody []Stmt
	ElseBody []Stmt
}

func (i *IfStmt) node()         {}
func (i *IfStmt) stmt()         {}
func (i *IfStmt) Pos() Position { return i.Position }

// StructField represents a field or embedded struct in a struct definition.
type StructField struct {
	Position     Position
	Name         string
	TypeAnnot    string
	DefaultValue Expr
	IsEmbedded   bool
}

// StructDef represents struct definition block: struct Name[T]: ...
type StructDef struct {
	Position   Position
	Name       string
	TypeParams []TypeParam
	Fields     []StructField
	Methods    []*FuncDef
}

func (s *StructDef) node()         {}
func (s *StructDef) stmt()         {}
func (s *StructDef) Pos() Position { return s.Position }

// MemberAssignStmt represents target.member = expr (e.g. self.x = 10)
type MemberAssignStmt struct {
	Position Position
	Target   Expr
	Member   string
	Value    Expr
}

func (m *MemberAssignStmt) node()         {}
func (m *MemberAssignStmt) stmt()         {}
func (m *MemberAssignStmt) Pos() Position { return m.Position }

// MethodSig represents a method signature in an interface definition.
type MethodSig struct {
	Position   Position
	Name       string
	TypeParams []TypeParam
	Params     []FuncParam
	BlockParam *FuncParam
	ReturnType string
}

func (f FuncParam) Pos() Position { return f.Position }
func (m MethodSig) Pos() Position { return m.Position }

// InterfaceDef represents interface definition block: interface Name[T]: ...
type InterfaceDef struct {
	Position   Position
	Name       string
	TypeParams []TypeParam
	Embedded   []string // Embedded interface names (may include type arguments, e.g. Reader[T])
	Methods    []MethodSig
}

func (i *InterfaceDef) node()         {}
func (i *InterfaceDef) stmt()         {}
func (i *InterfaceDef) Pos() Position { return i.Position }

// UnescapeRegexPattern unescapes \/ to / while preserving \\.
func UnescapeRegexPattern(rawPat string) string {
	var patSb strings.Builder
	for i := 0; i < len(rawPat); i++ {
		if rawPat[i] == '\\' && i+1 < len(rawPat) {
			if rawPat[i+1] == '/' {
				patSb.WriteByte('/')
			} else {
				patSb.WriteByte('\\')
				patSb.WriteByte(rawPat[i+1])
			}
			i++
		} else {
			patSb.WriteByte(rawPat[i])
		}
	}
	return patSb.String()
}

// SkipPOSIXCharClass checks if s[start:] begins with a valid POSIX character class
// such as [:alpha:] or [:^digit:] (format: [:^?[a-zA-Z]+:]).
// If so, it returns the index in s immediately following :].
// Otherwise, it returns start.
func SkipPOSIXCharClass(s string, start int) int {
	if start < 0 || start+3 >= len(s) || s[start] != '[' || s[start+1] != ':' {
		return start
	}
	i := start + 2
	if i < len(s) && s[i] == '^' {
		i++
	}
	nameStart := i
	for i < len(s) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z')) {
		i++
	}
	if i == nameStart {
		return start
	}
	if i+1 < len(s) && s[i] == ':' && s[i+1] == ']' {
		return i + 2
	}
	return start
}

// SkipPOSIXCharClassInRunes is the []rune variant of SkipPOSIXCharClass.
func SkipPOSIXCharClassInRunes(runes []rune, start int) int {
	if start < 0 || start+3 >= len(runes) || runes[start] != '[' || runes[start+1] != ':' {
		return start
	}
	i := start + 2
	if i < len(runes) && runes[i] == '^' {
		i++
	}
	nameStart := i
	for i < len(runes) && ((runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z')) {
		i++
	}
	if i == nameStart {
		return start
	}
	if i+1 < len(runes) && runes[i] == ':' && runes[i+1] == ']' {
		return i + 2
	}
	return start
}

// SkipRegexInTypeAnnot checks if runes[start] starts a regex literal (/pattern/flags).
// If so, it scans past the closing / and trailing flags, respecting escaped characters
// and character classes ([]...], [^]...]), and returns the index after the regex literal.
// If it is not a valid regex literal or has no closing slash, it returns start.
func SkipRegexInTypeAnnot(runes []rune, start int) int {
	if start < 0 || start >= len(runes) || runes[start] != '/' {
		return start
	}
	i := start + 1
	inCharClass := false
	charClassStart := -1
	for i < len(runes) {
		if runes[i] == '\\' {
			i += 2
			continue
		}
		if inCharClass {
			if runes[i] == '[' {
				if nextIdx := SkipPOSIXCharClassInRunes(runes, i); nextIdx > i {
					i = nextIdx
					continue
				}
			}
			if runes[i] == ']' && i > charClassStart {
				inCharClass = false
			}
			i++
			continue
		}
		if runes[i] == '[' {
			inCharClass = true
			charClassStart = i + 1
			if i+1 < len(runes) && runes[i+1] == '^' {
				charClassStart = i + 2
			}
			i++
			continue
		}
		if runes[i] == '/' && i > start+1 {
			i++
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
			return i
		}
		i++
	}
	return start
}

// HasEnclosingDelims returns true if s starts with open and ends with close,
// and the opening delimiter matches the final character at index len(runes)-1
// without closing earlier. Regex literals inside are skipped.
func HasEnclosingDelims(s string, open, close rune) bool {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) < 2 || runes[0] != open || runes[len(runes)-1] != close {
		return false
	}
	depth := 0
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '/' {
			nextIdx := SkipRegexInTypeAnnot(runes, i)
			if nextIdx > i {
				i = nextIdx - 1
				continue
			}
		}
		if r == open {
			depth++
		} else if r == close {
			depth--
			if depth == 0 {
				return i == len(runes)-1
			}
		}
	}
	return false
}

// HasTopLevelComma returns true if s contains a comma at the top level
// (outside parens, brackets, braces, and regex literals).
func HasTopLevelComma(s string) bool {
	pDepth := 0
	bDepth := 0
	brDepth := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '/' {
			nextIdx := SkipRegexInTypeAnnot(runes, i)
			if nextIdx > i {
				i = nextIdx - 1
				continue
			}
		}
		if r == '(' {
			pDepth++
		} else if r == ')' {
			pDepth--
		} else if r == '[' {
			bDepth++
		} else if r == ']' {
			bDepth--
		} else if r == '{' {
			brDepth++
		} else if r == '}' {
			brDepth--
		} else if r == ',' && pDepth == 0 && bDepth == 0 && brDepth == 0 {
			return true
		}
	}
	return false
}