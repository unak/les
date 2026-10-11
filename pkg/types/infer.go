package types

import (
	"fmt"
	"strings"

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
	// Built-in type: (Any) -> String
	_ = rootScope.Declare("type", &FuncType{
		Arg: TypeAny,
		Ret: TypeString,
	})
	// Built-in len: (Any) -> Int
	_ = rootScope.Declare("len", &FuncType{
		Arg: TypeAny,
		Ret: TypeInt,
	})
	// Built-in exit: (Any) -> Nil
	_ = rootScope.Declare("exit", &FuncType{
		Arg: TypeAny,
		Ret: TypeNil,
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
			expected := inf.parseTypeAnnot(s.TypeAnnot)
			if !IsAssignableTo(valType, expected) {
				return fmt.Errorf("type mismatch for variable '%s': expected %s, got %s", s.Name, expected, valType)
			}
			targetType = expected
		}
		// Enforce same-scope duplicate declaration prohibition
		if err := inf.scope.Declare(s.Name, targetType); err != nil {
			return err
		}
		return nil

	case *ast.UnpackDecl:
		_, err := inf.inferExpr(s.Value)
		if err != nil {
			return err
		}
		for _, name := range s.Pattern.Elements {
			if name != "_" {
				_ = inf.scope.Declare(name, TypeAny)
			}
		}
		if s.Pattern.RestName != "" && s.Pattern.RestName != "_" {
			_ = inf.scope.Declare(s.Pattern.RestName, &TupleType{Elems: []Type{TypeAny}, IsVariadic: true})
		}
		return nil

	case *ast.FuncDef:
		paramTypes := make([]Type, len(s.Params))
		for i, p := range s.Params {
			var elemType Type = TypeAny
			if p.TypeAnnot != "" {
				elemType = inf.parseTypeAnnot(p.TypeAnnot)
			}
			if p.IsVariadic {
				emptyTuple := &TupleType{Elems: []Type{}}
				streamTuple := &TupleType{Elems: []Type{elemType}, IsVariadic: true}
				paramTypes[i] = NewUnionType(emptyTuple, streamTuple)
			} else {
				paramTypes[i] = elemType
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

	case *ast.ReturnStmt:
		if s.Value != nil {
			_, err := inf.inferExpr(s.Value)
			return err
		}
		return nil

	case *ast.BreakStmt:
		if s.Value != nil {
			_, err := inf.inferExpr(s.Value)
			return err
		}
		return nil

	case *ast.StructDef:
		st := &StructType{
			Name:       s.Name,
			Fields:     make(map[string]Type),
			FieldOrder: make([]string, 0),
			Embedded:   make([]*StructType, 0),
			Methods:    make(map[string]*FuncType),
		}

		// Declare struct type in current scope so methods/self can reference it
		if err := inf.scope.Declare(s.Name, st); err != nil {
			return err
		}

		// Process fields
		for _, f := range s.Fields {
			if f.IsEmbedded {
				embType, ok := inf.scope.Lookup(f.TypeAnnot)
				if !ok {
					return fmt.Errorf("undefined embedded struct '%s' in struct '%s'", f.TypeAnnot, s.Name)
				}
				embStruct, ok := embType.(*StructType)
				if !ok {
					return fmt.Errorf("cannot embed non-struct type '%s' in struct '%s'", f.TypeAnnot, s.Name)
				}
				st.Embedded = append(st.Embedded, embStruct)
				st.Fields[f.Name] = embStruct
				st.FieldOrder = append(st.FieldOrder, f.Name)
			} else {
				fieldType := inf.parseTypeAnnot(f.TypeAnnot)
				st.Fields[f.Name] = fieldType
				st.FieldOrder = append(st.FieldOrder, f.Name)
			}
		}

		// Process methods
		for _, m := range s.Methods {
			methodScope := NewScope(inf.scope)
			_ = methodScope.Declare("self", st)
			for fName, fType := range st.Fields {
				_ = methodScope.Declare(fName, fType)
			}

			paramTypes := make([]Type, 0, len(m.Params))
			for _, p := range m.Params {
				if p.Name == "self" {
					continue
				}
				pType := inf.parseTypeAnnot(p.TypeAnnot)
				paramTypes = append(paramTypes, pType)
				_ = methodScope.Declare(p.Name, pType)
			}

			prevScope := inf.scope
			inf.scope = methodScope
			var lastExprType Type = TypeAny
			for _, bStmt := range m.Body {
				_ = inf.inferStmt(bStmt)
				if es, ok := bStmt.(*ast.ExprStmt); ok {
					if et, err := inf.inferExpr(es.Value); err == nil {
						lastExprType = et
					}
				} else if rs, ok := bStmt.(*ast.ReturnStmt); ok {
					if rs.Value != nil {
						if et, err := inf.inferExpr(rs.Value); err == nil {
							lastExprType = et
						}
					}
				}
			}
			inf.scope = prevScope

			var retType Type = TypeAny
			if m.ReturnType != "" {
				retType = inf.parseTypeAnnot(m.ReturnType)
			} else if lastExprType != nil {
				retType = lastExprType
			}

			mType := &FuncType{
				Arg: &TupleType{Elems: paramTypes},
				Ret: retType,
			}
			st.Methods[m.Name] = mType
		}

		return nil

	case *ast.InterfaceDef:
		iface := &InterfaceType{
			Name:     s.Name,
			Methods:  make(map[string]*FuncType),
			Embedded: make([]*InterfaceType, 0),
		}

		// First, collect methods and check for conflicts from embedded interfaces
		for _, embName := range s.Embedded {
			embType, ok := inf.scope.Lookup(embName)
			if !ok {
				return fmt.Errorf("undefined embedded interface '%s' in interface '%s'", embName, s.Name)
			}
			embIface, ok := embType.(*InterfaceType)
			if !ok {
				return fmt.Errorf("cannot embed non-interface type '%s' in interface '%s'", embName, s.Name)
			}
			iface.Embedded = append(iface.Embedded, embIface)

			for mName, mSig := range embIface.Methods {
				if existingSig, exists := iface.Methods[mName]; exists {
					if !existingSig.Equals(mSig) {
						return fmt.Errorf("conflicting method signatures for '%s' in embedded interface '%s' of '%s'", mName, embName, s.Name)
					}
				} else {
					iface.Methods[mName] = mSig
				}
			}
		}

		// Next, process direct methods on interface
		for _, m := range s.Methods {
			paramTypes := make([]Type, len(m.Params))
			for i, p := range m.Params {
				paramTypes[i] = inf.parseTypeAnnot(p.TypeAnnot)
			}
			retType := inf.parseTypeAnnot(m.ReturnType)
			mSig := &FuncType{
				Arg: &TupleType{Elems: paramTypes},
				Ret: retType,
			}

			if existingSig, exists := iface.Methods[m.Name]; exists {
				if !existingSig.Equals(mSig) {
					return fmt.Errorf("method '%s' in interface '%s' conflicts with embedded method signature", m.Name, s.Name)
				}
			}
			iface.Methods[m.Name] = mSig
		}

		if err := inf.scope.Declare(s.Name, iface); err != nil {
			return err
		}
		return nil

	case *ast.ImportStmt:
		return nil
	case *ast.ModuleDecl:
		return nil

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
	case *ast.NilLit:
		return TypeNil, nil
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
		elseT, err := inf.inferExpr(e.Else)
		if err != nil {
			return nil, err
		}
		if thenT.Equals(elseT) {
			return thenT, nil
		}
		return NewUnionType(thenT, elseT), nil
	case *ast.CallExpr:
		calleeT, err := inf.inferExpr(e.Callee)
		if err != nil {
			return nil, err
		}
		if st, ok := calleeT.(*StructType); ok {
			return st, nil
		}
		if fnT, ok := calleeT.(*FuncType); ok {
			return fnT.Ret, nil
		}
		return TypeAny, nil
	case *ast.MemberAccessExpr:
		targetT, err := inf.inferExpr(e.Target)
		if err != nil {
			return nil, err
		}
		if st, ok := targetT.(*StructType); ok {
			return st.LookupMember(e.Member)
		}
		if it, ok := targetT.(*InterfaceType); ok {
			if mSig, ok := it.LookupMethod(e.Member); ok {
				return mSig, nil
			}
			return nil, fmt.Errorf("interface %s has no method '%s'", it.Name, e.Member)
		}
		if targetT == TypeAny {
			return TypeAny, nil
		}
		return nil, fmt.Errorf("cannot access member '%s' on non-struct/interface type %s", e.Member, targetT)
	case *ast.NamedArgExpr:
		return inf.inferExpr(e.Value)
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
	case *ast.ModuleAccessExpr:
		return TypeAny, nil // type of module member (flexible/Any for dynamic module lookup)
	case *ast.IsExpr:
		_, err := inf.inferExpr(e.Expr)
		if err != nil {
			return nil, err
		}
		return TypeBool, nil
	case *ast.MatchExpr:
		_, err := inf.inferExpr(e.Target)
		if err != nil {
			return nil, err
		}
		var caseTypes []Type
		for _, c := range e.Cases {
			caseScope := NewScope(inf.scope)
			if c.TuplePattern != nil {
				for _, name := range c.TuplePattern.Elements {
					if name != "_" {
						_ = caseScope.Declare(name, TypeAny)
					}
				}
				if c.TuplePattern.RestName != "" && c.TuplePattern.RestName != "_" {
					_ = caseScope.Declare(c.TuplePattern.RestName, &TupleType{Elems: []Type{TypeAny}, IsVariadic: true})
				}
			}
			prevScope := inf.scope
			inf.scope = caseScope

			var lastT Type = TypeAny
			for _, stmt := range c.Body {
				if es, ok := stmt.(*ast.ExprStmt); ok {
					t, err := inf.inferExpr(es.Value)
					if err != nil {
						inf.scope = prevScope
						return nil, err
					}
					lastT = t
				} else if rs, ok := stmt.(*ast.ReturnStmt); ok {
					t, err := inf.inferExpr(rs.Value)
					if err != nil {
						inf.scope = prevScope
						return nil, err
					}
					lastT = t
				} else {
					if err := inf.inferStmt(stmt); err != nil {
						inf.scope = prevScope
						return nil, err
					}
				}
			}
			inf.scope = prevScope
			caseTypes = append(caseTypes, lastT)
		}
		if len(caseTypes) == 0 {
			return TypeAny, nil
		}
		return NewUnionType(caseTypes...), nil
	case *ast.LambdaExpr:
		paramTypes := make([]Type, len(e.Params))
		for i, p := range e.Params {
			var elemType Type = TypeAny
			if p.TypeAnnot != "" {
				elemType = inf.parseTypeAnnot(p.TypeAnnot)
			}
			if p.IsVariadic {
				emptyTuple := &TupleType{Elems: []Type{}}
				streamTuple := &TupleType{Elems: []Type{elemType}, IsVariadic: true}
				paramTypes[i] = NewUnionType(emptyTuple, streamTuple)
			} else {
				paramTypes[i] = elemType
			}
		}

		childScope := NewScope(inf.scope)
		for i, p := range e.Params {
			_ = childScope.Declare(p.Name, paramTypes[i])
		}

		prevScope := inf.scope
		inf.scope = childScope
		var lastT Type = TypeAny
		for _, stmt := range e.Body {
			if es, ok := stmt.(*ast.ExprStmt); ok {
				t, err := inf.inferExpr(es.Value)
				if err != nil {
					inf.scope = prevScope
					return nil, err
				}
				lastT = t
			} else if rs, ok := stmt.(*ast.ReturnStmt); ok {
				t, err := inf.inferExpr(rs.Value)
				if err != nil {
					inf.scope = prevScope
					return nil, err
				}
				lastT = t
			} else if bs, ok := stmt.(*ast.BreakStmt); ok {
				t, err := inf.inferExpr(bs.Value)
				if err != nil {
					inf.scope = prevScope
					return nil, err
				}
				lastT = t
			} else {
				if err := inf.inferStmt(stmt); err != nil {
					inf.scope = prevScope
					return nil, err
				}
			}
		}
		inf.scope = prevScope
		return &FuncType{
			Arg: &TupleType{Elems: paramTypes},
			Ret: lastT,
		}, nil

	default:
		return TypeAny, nil
	}
}

func parseTypeAnnot(annot string) Type {
	return (*Inferencer)(nil).parseTypeAnnot(annot)
}

func (inf *Inferencer) parseTypeAnnot(annot string) Type {
	annot = strings.TrimSpace(annot)
	if annot == "" {
		return TypeAny
	}

	// Check top-level Union: A | B
	// Careful not to split inside parentheses or brackets
	var parts []string
	parenDepth := 0
	bracketDepth := 0
	start := 0
	runes := []rune(annot)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '(' {
			parenDepth++
		} else if r == ')' {
			parenDepth--
		} else if r == '[' {
			bracketDepth++
		} else if r == ']' {
			bracketDepth--
		} else if r == '|' && parenDepth == 0 && bracketDepth == 0 {
			parts = append(parts, string(runes[start:i]))
			start = i + 1
		}
	}
	if len(parts) > 0 {
		parts = append(parts, string(runes[start:]))
		var memberTypes []Type
		for _, p := range parts {
			memberTypes = append(memberTypes, inf.parseTypeAnnot(p))
		}
		return NewUnionType(memberTypes...)
	}

	switch annot {
	case "Int":
		return TypeInt
	case "Float":
		return TypeFloat
	case "String":
		return TypeString
	case "Bool":
		return TypeBool
	case "Nil":
		return TypeNil
	case "Any":
		return TypeAny
	}

	// Tuple annotation: (Int, ...) or (Int, String)
	if strings.HasPrefix(annot, "(") && strings.HasSuffix(annot, ")") {
		inner := strings.TrimSpace(annot[1 : len(annot)-1])
		if inner == "" {
			return &TupleType{Elems: []Type{}}
		}
		rawParts := strings.Split(inner, ",")
		var elems []Type
		isVariadic := false

		for _, p := range rawParts {
			part := strings.TrimSpace(p)
			if part == "..." {
				isVariadic = true
				break
			}
			if part != "" {
				elems = append(elems, inf.parseTypeAnnot(part))
			}
		}
		return &TupleType{
			Elems:      elems,
			IsVariadic: isVariadic,
		}
	}

	if inf != nil && inf.scope != nil {
		if t, ok := inf.scope.Lookup(annot); ok {
			return t
		}
	}

	return TypeAny
}
