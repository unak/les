package ast

// Node represents any AST node in Les.
type Node interface {
	node()
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
	ModuleName string
	Body       []Stmt
}

func (p *Program) node() {}

// ModuleDecl represents explicit module declaration at file top: module <name>.
type ModuleDecl struct {
	Name string
}

func (m *ModuleDecl) node() {}
func (m *ModuleDecl) stmt() {}

// ImportStmt represents module import: import <name> or import "<path>".
type ImportStmt struct {
	Path  string
	Alias string
}

func (imp *ImportStmt) node() {}
func (imp *ImportStmt) stmt() {}

// Identifier represents variable or function name.
type Identifier struct {
	Name string
}

func (i *Identifier) node() {}
func (i *Identifier) expr() {}

// ModuleAccessExpr represents @module.Member expression.
type ModuleAccessExpr struct {
	Module string
	Member string
}

func (m *ModuleAccessExpr) node() {}
func (m *ModuleAccessExpr) expr() {}

// IntLit represents integer literal.
type IntLit struct {
	Value int64
}

func (i *IntLit) node() {}
func (i *IntLit) expr() {}

// FloatLit represents floating point literal.
type FloatLit struct {
	Value float64
}

func (f *FloatLit) node() {}
func (f *FloatLit) expr() {}

// StringLit represents string literal.
type StringLit struct {
	Value string
}

func (s *StringLit) node() {}
func (s *StringLit) expr() {}

// BoolLit represents boolean literal.
type BoolLit struct {
	Value bool
}

func (b *BoolLit) node() {}
func (b *BoolLit) expr() {}

// NilLit represents nil literal.
type NilLit struct{}

func (n *NilLit) node() {}
func (n *NilLit) expr() {}

// TupleLit represents tuple literal, e.g. (1, 2, 3), (), (1,).
type TupleLit struct {
	Elements []Expr
}

func (t *TupleLit) node() {}
func (t *TupleLit) expr() {}

// DictEntry represents key-value pair in dictionary.
type DictEntry struct {
	Key   Expr
	Value Expr
}

// DictLit represents dictionary literal, e.g. {"a": 1}.
type DictLit struct {
	Entries []DictEntry
}

func (d *DictLit) node() {}
func (d *DictLit) expr() {}

// IndexExpr represents indexing with [], e.g. target[index].
type IndexExpr struct {
	Target Expr
	Index  Expr
}

func (idx *IndexExpr) node() {}
func (idx *IndexExpr) expr() {}

// BinaryExpr represents binary operations (+, -, *, /, %, ==, !=, <, <=, >, >=).
type BinaryExpr struct {
	Op    string
	Left  Expr
	Right Expr
}

func (b *BinaryExpr) node() {}
func (b *BinaryExpr) expr() {}

// UnaryExpr represents unary operations (-, !).
type UnaryExpr struct {
	Op   string
	Expr Expr
}

func (u *UnaryExpr) node() {}
func (u *UnaryExpr) expr() {}

// TernaryExpr represents C-style ternary expression: cond ? then : else.
type TernaryExpr struct {
	Cond Expr
	Then Expr
	Else Expr
}

func (t *TernaryExpr) node() {}
func (t *TernaryExpr) expr() {}

// IsExpr represents type check expression: expr is Type.
type IsExpr struct {
	Expr      Expr
	TypeAnnot string
}

func (i *IsExpr) node() {}
func (i *IsExpr) expr() {}

// MatchCase represents a single case in a match expression: case Pattern: Body
type MatchCase struct {
	Pattern string // type name, e.g. "Int", "String", or "_" for wildcard
	Body    []Stmt
}

// MatchExpr represents pattern match expression: match expr: case ...
type MatchExpr struct {
	Target Expr
	Cases  []MatchCase
}

func (m *MatchExpr) node() {}
func (m *MatchExpr) expr() {}

// CallExpr represents function call. Arg is always a TupleLit expression (or single expr desugared).
type CallExpr struct {
	Callee Expr
	Args   []Expr
	Block  *BlockLiteral // optional trailing do |x|: block
}

func (c *CallExpr) node() {}
func (c *CallExpr) expr() {}

// BlockLiteral represents do |params|: body block closure.
type BlockLiteral struct {
	Params []FuncParam
	Body   []Stmt
}

func (b *BlockLiteral) node() {}
func (b *BlockLiteral) expr() {}

// VarDecl represents variable assignment with optional type annotation.
type VarDecl struct {
	Name      string
	TypeAnnot string
	Value     Expr
}

func (v *VarDecl) node() {}
func (v *VarDecl) stmt() {}

// ReturnStmt represents explicit return.
type ReturnStmt struct {
	Value Expr
}

func (r *ReturnStmt) node() {}
func (r *ReturnStmt) stmt() {}

// ExprStmt represents expression evaluated as statement.
type ExprStmt struct {
	Value Expr
}

func (e *ExprStmt) node() {}
func (e *ExprStmt) stmt() {}

// FuncParam represents function parameter with optional type annotation.
type FuncParam struct {
	Name      string
	TypeAnnot string
}

// FuncDef represents function definition block: def name(params): body.
type FuncDef struct {
	Name       string
	Params     []FuncParam
	ReturnType string
	Body       []Stmt
}

func (f *FuncDef) node() {}
func (f *FuncDef) stmt() {}

// IfStmt represents if-else block statement.
type IfStmt struct {
	Cond     Expr
	ThenBody []Stmt
	ElseBody []Stmt
}

func (i *IfStmt) node() {}
func (i *IfStmt) stmt() {}
