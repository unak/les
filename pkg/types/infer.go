package types

import (
	"fmt"

	"les/pkg/ast"
)

// Scope tracks lexical bindings and detects duplicate declarations in the same scope.
type Scope struct {
	parent   *Scope
	bindings map[string]Type
	declared map[string]bool // declarations made specifically in this lexical scope
}

func NewScope(parent *Scope) *Scope {
	return &Scope{
		parent:   parent,
		bindings: make(map[string]Type),
		declared: make(map[string]bool),
	}
}

func (s *Scope) Declare(name string, t Type) error {
	if s.declared[name] {
		return fmt.Errorf("duplicate declaration of '%s' in the same scope", name)
	}
	s.declared[name] = true
	s.bindings[name] = t
	return nil
}

func (s *Scope) Lookup(name string) (Type, bool) {
	if t, ok := s.bindings[name]; ok {
		return t, true
	}
	if s.parent != nil {
		return s.parent.Lookup(name)
	}
	return nil, false
}

// Inferencer performs Hindley-Milner type inference with lexical scope checking.
type Inferencer struct {
	nextVarID int
	scope     *Scope
}

func NewInferencer() *Inferencer {
	rootScope := NewScope(nil)
	// Built-in print: (Any) -> Any
	_ = rootScope.Declare("print", &FuncType{
		Arg: TypeAny,
		Ret: TypeAny,
	})
	return &Inferencer{
		nextVarID: 0,
		scope:     rootScope,
	}
}

func (inf *Inferencer) freshVar() *TypeVar {
	v := &TypeVar{ID: inf.nextVarID}
	inf.nextVarID++
	return v
}

func (inf *Inferencer) InferProgram(prog *ast.Program) error {
	for _, stmt := range prog.Body {
		if err := inf.inferStmt(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (inf *Inferencer) inferStmt(stmt ast.Stmt) error {
	switch s := stmt.(type) {
	case *ast.VarDecl:
		valType, err := inf.inferExpr(s.Value)
		if err != nil {
			return err
		}
		targetType := valType
		if s.TypeAnnot != "" {
			expected := parseTypeAnnot(s.TypeAnnot)
			if !expected.Equals(valType) {
				return fmt.Errorf("type mismatch for variable '%s': expected %s, got %s", s.Name, expected, valType)
			}
			targetType = expected
		}
		// Enforce same-scope duplicate declaration prohibition
		if err := inf.scope.Declare(s.Name, targetType); err != nil {
			return err
		}
		return nil

	case *ast.FuncDef:
		paramTypes := make([]Type, len(s.Params))
		for i, p := range s.Params {
			if p.TypeAnnot != "" {
				paramTypes[i] = parseTypeAnnot(p.TypeAnnot)
			} else {
				paramTypes[i] = TypeAny // default flexible for MVP inference
			}
		}
		fnType := &FuncType{
			Arg: &TupleType{Elems: paramTypes},
			Ret: TypeAny,
		}
		// Function declaration in current scope
		if err := inf.scope.Declare(s.Name, fnType); err != nil {
			return err
		}

		// Create child scope for function parameters and body (shadowing allowed)
		childScope := NewScope(inf.scope)
		for i, p := range s.Params {
			if err := childScope.Declare(p.Name, paramTypes[i]); err != nil {
				return fmt.Errorf("duplicate parameter '%s' in function '%s'", p.Name, s.Name)
			}
		}

		prevScope := inf.scope
		inf.scope = childScope
		for _, bStmt := range s.Body {
			if err := inf.inferStmt(bStmt); err != nil {
				inf.scope = prevScope
				return err
			}
		}
		inf.scope = prevScope
		return nil

	case *ast.ExprStmt:
		_, err := inf.inferExpr(s.Value)
		return err

	default:
		return nil
	}
}

func (inf *Inferencer) inferExpr(expr ast.Expr) (Type, error) {
	if expr == nil {
		return TypeAny, nil
	}
	switch e := expr.(type) {
	case *ast.IntLit:
		return TypeInt, nil
	case *ast.FloatLit:
		return TypeFloat, nil
	case *ast.StringLit:
		return TypeString, nil
	case *ast.BoolLit:
		return TypeBool, nil
	case *ast.Identifier:
		if t, ok := inf.scope.Lookup(e.Name); ok {
			return t, nil
		}
		return TypeAny, nil // forward reference or flexible variable
	case *ast.TupleLit:
		elems := make([]Type, len(e.Elements))
		for i, el := range e.Elements {
			t, err := inf.inferExpr(el)
			if err != nil {
				return nil, err
			}
			elems[i] = t
		}
		return &TupleType{Elems: elems}, nil
	case *ast.DictLit:
		var keyT Type = TypeAny
		var valT Type = TypeAny
		if len(e.Entries) > 0 {
			k, _ := inf.inferExpr(e.Entries[0].Key)
			v, _ := inf.inferExpr(e.Entries[0].Value)
			keyT = k
			valT = v
		}
		return &DictType{Key: keyT, Value: valT}, nil
	case *ast.BinaryExpr:
		leftT, err := inf.inferExpr(e.Left)
		if err != nil {
			return nil, err
		}
		rightT, err := inf.inferExpr(e.Right)
		if err != nil {
			return nil, err
		}
		switch e.Op {
		case "+", "-", "*", "/", "%":
			if leftT == TypeFloat || rightT == TypeFloat {
				return TypeFloat, nil
			}
			return TypeInt, nil
		case "==", "!=", "<", "<=", ">", ">=":
			return TypeBool, nil
		case "&&", "||":
			return TypeBool, nil
		default:
			return TypeAny, nil
		}
	case *ast.TernaryExpr:
		_, err := inf.inferExpr(e.Cond)
		if err != nil {
			return nil, err
		}
		thenT, err := inf.inferExpr(e.Then)
		if err != nil {
			return nil, err
		}
		return thenT, nil
	case *ast.CallExpr:
		calleeT, err := inf.inferExpr(e.Callee)
		if err != nil {
			return nil, err
		}
		if fnT, ok := calleeT.(*FuncType); ok {
			return fnT.Ret, nil
		}
		return TypeAny, nil
	case *ast.IndexExpr:
		targetT, err := inf.inferExpr(e.Target)
		if err != nil {
			return nil, err
		}
		if tt, ok := targetT.(*TupleType); ok {
			if len(tt.Elems) > 0 {
				return tt.Elems[0], nil
			}
		} else if dt, ok := targetT.(*DictType); ok {
			return dt.Value, nil
		}
		return TypeAny, nil
	default:
		return TypeAny, nil
	}
}

func parseTypeAnnot(annot string) Type {
	switch annot {
	case "Int":
		return TypeInt
	case "Float":
		return TypeFloat
	case "String":
		return TypeString
	case "Bool":
		return TypeBool
	default:
		return TypeAny
	}
}
