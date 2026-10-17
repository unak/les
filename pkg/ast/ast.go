package ast

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

// TupleLit represents tuple literal, e.g. (1, 2, 3), (), (1,).
type TupleLit struct {
	Position Position
	Elements []Expr
}

func (t *TupleLit) node()         {}
func (t *TupleLit) expr()         {}
func (t *TupleLit) Pos() Position { return t.Position }

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

// IndexExpr represents indexing with [], e.g. target[index].
type IndexExpr struct {
	Position Position
	Target   Expr
	Index    Expr
}

func (idx *IndexExpr) node()         {}
func (idx *IndexExpr) expr()         {}
func (idx *IndexExpr) Pos() Position { return idx.Position }

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

// FuncParam represents function parameter with optional type annotation.
type FuncParam struct {
	Position     Position
	Name         string
	TypeAnnot    string
	DefaultValue Expr // nil if required
	IsVariadic   bool
}

// FuncDef represents function definition block: def name(params): body.
type FuncDef struct {
	Position   Position
	Name       string
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

// StructDef represents struct definition block: struct Name: ...
type StructDef struct {
	Position Position
	Name     string
	Fields   []StructField
	Methods  []*FuncDef
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
	Params     []FuncParam
	BlockParam *FuncParam
	ReturnType string
}

func (f FuncParam) Pos() Position { return f.Position }
func (m MethodSig) Pos() Position { return m.Position }

// InterfaceDef represents interface definition block: interface Name: ...
type InterfaceDef struct {
	Position Position
	Name     string
	Embedded []string // Embedded interface names
	Methods  []MethodSig
}

func (i *InterfaceDef) node()         {}
func (i *InterfaceDef) stmt()         {}
func (i *InterfaceDef) Pos() Position { return i.Position }
