package eval

import (
	"sync"

	"les/pkg/ast"
)

// newCapturedThunk creates a lazy thunk for expr that keeps observing the
// bindings visible now, even if some of them are rebound before it is forced.
func newCapturedThunk(expr ast.Expr, env *Environment) *Thunk {
	if id, ok := expr.(*ast.Identifier); ok && env != nil {
		// A bare variable reference needs no scope of its own: the current
		// value (possibly an unevaluated thunk itself, which Force follows)
		// is exactly what has to be observed later.
		if v, found := env.Get(id.Name); found {
			return &Thunk{Evaluated: true, Value: v, Expr: expr, Env: nil}
		}
	}
	return NewThunk(expr, env.captureEnv(expr))
}

// captureEnv returns an environment in which expr can be evaluated later and
// still observe the bindings that are visible now.
//
// Rebinding an existing variable (e.g. `j = j + 1`) replaces the entry in
// env.store in place.  A thunk that simply kept a reference to env would
// therefore see the new value when it is eventually forced, so values pushed
// into a tuple inside a loop all collapsed into the last one.  To keep lazy
// evaluation referentially transparent, the identifiers referenced by expr
// are copied into a fresh child scope.  Values are immutable, so copying the
// references (without forcing them) is enough.  Identifiers that are not
// bound yet are left to the parent, so forward and self references keep
// working.
func (env *Environment) captureEnv(expr ast.Expr) *Environment {
	if env == nil || expr == nil {
		return env
	}
	names := referencedNames(expr)
	if len(names) == 0 {
		return env
	}
	var snap *Environment
	for _, name := range names {
		v, ok := env.Get(name)
		if !ok {
			continue
		}
		if snap == nil {
			snap = NewEnvironment(env)
		}
		snap.store[name] = v
	}
	if snap == nil {
		return env
	}
	return snap
}

// forceExpr evaluates expr immediately in env.  Unlike newThunk it does not
// capture the referenced bindings: the expression is evaluated before env can
// change, so there is nothing to preserve.
func (env *Environment) forceExpr(expr ast.Expr) Value {
	return Force(NewThunk(expr, env))
}

var referencedNamesCache sync.Map // ast.Expr -> []string

// referencedNames returns the identifier names that appear anywhere in expr,
// including inside nested lambdas, blocks and match cases.  It is a
// conservative over-approximation of the free variables of expr: capturing a
// name that turns out to be shadowed by an inner binding is harmless.
func referencedNames(expr ast.Expr) []string {
	if cached, ok := referencedNamesCache.Load(expr); ok {
		return cached.([]string)
	}
	c := &nameCollector{seen: make(map[string]bool)}
	c.expr(expr)
	referencedNamesCache.Store(expr, c.names)
	return c.names
}

type nameCollector struct {
	seen  map[string]bool
	names []string
}

func (c *nameCollector) add(name string) {
	if name == "" || c.seen[name] {
		return
	}
	c.seen[name] = true
	c.names = append(c.names, name)
}

func (c *nameCollector) expr(expr ast.Expr) {
	switch e := expr.(type) {
	case nil:
	case *ast.Identifier:
		c.add(e.Name)
	case *ast.MemberAccessExpr:
		c.expr(e.Target)
	case *ast.NamedArgExpr:
		c.expr(e.Value)
	case *ast.TupleLit:
		for _, el := range e.Elements {
			c.expr(el)
		}
	case *ast.SpreadExpr:
		c.expr(e.Expr)
	case *ast.DictLit:
		for _, entry := range e.Entries {
			c.expr(entry.Key)
			c.expr(entry.Value)
		}
	case *ast.IndexExpr:
		c.expr(e.Target)
		c.expr(e.Index)
	case *ast.TypeInstExpr:
		c.expr(e.Target)
	case *ast.BinaryExpr:
		c.expr(e.Left)
		c.expr(e.Right)
	case *ast.UnaryExpr:
		c.expr(e.Expr)
	case *ast.ForceExpr:
		c.expr(e.Expr)
	case *ast.TernaryExpr:
		c.expr(e.Cond)
		c.expr(e.Then)
		c.expr(e.Else)
	case *ast.IsExpr:
		c.expr(e.Expr)
	case *ast.MatchExpr:
		c.expr(e.Target)
		for _, mc := range e.Cases {
			c.stmts(mc.Body)
		}
	case *ast.CallExpr:
		c.expr(e.Callee)
		for _, a := range e.Args {
			c.expr(a)
		}
		if e.Block != nil {
			c.expr(e.Block)
		}
	case *ast.BlockLiteral:
		c.params(e.Params)
		c.stmts(e.Body)
	case *ast.LambdaExpr:
		c.params(e.Params)
		c.stmts(e.Body)
		c.expr(e.Expr)
	}
}

func (c *nameCollector) params(params []ast.FuncParam) {
	for _, p := range params {
		c.expr(p.DefaultValue)
	}
}

func (c *nameCollector) stmts(stmts []ast.Stmt) {
	for _, s := range stmts {
		c.stmt(s)
	}
}

func (c *nameCollector) stmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.VarDecl:
		c.expr(s.Value)
	case *ast.UnpackDecl:
		c.expr(s.Value)
	case *ast.ReturnStmt:
		c.expr(s.Value)
	case *ast.BreakStmt:
		c.expr(s.Value)
	case *ast.ForStmt:
		c.expr(s.Cond)
		c.stmts(s.Body)
	case *ast.ExprStmt:
		c.expr(s.Value)
	case *ast.IfStmt:
		c.expr(s.Cond)
		c.stmts(s.ThenBody)
		c.stmts(s.ElseBody)
	case *ast.FuncDef:
		c.params(s.Params)
		if s.BlockParam != nil {
			c.expr(s.BlockParam.DefaultValue)
		}
		c.stmts(s.Body)
	case *ast.StructDef:
		for _, f := range s.Fields {
			c.expr(f.DefaultValue)
		}
		for _, m := range s.Methods {
			c.stmt(m)
		}
	case *ast.MemberAssignStmt:
		c.expr(s.Target)
		c.expr(s.Value)
	}
}
