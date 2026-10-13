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
		existingType := s.bindings[name]
		if !IsAssignableTo(t, existingType) {
			return fmt.Errorf("cannot assign value of type %s to variable '%s' of type %s", t, name, existingType)
		}
		s.bindings[name] = t
		return nil
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
	nextVarID    int
	scope        *Scope
	modules      map[string]*ModuleType
	moduleLoader func(path string) (*ModuleType, error)
}

var builtinForceFuncType = &FuncType{
	Arg: TypeAny,
	Ret: TypeAny,
}

var builtinBytesFuncType = &FuncType{
	Arg: TypeAny,
	Ret: TypeBytes,
}

func NewInferencer() *Inferencer {
	rootScope := NewScope(nil)

	// Built-in Error interface: Message(): String, ToString(): String
	errorIface := &InterfaceType{
		Name: "Error",
		Methods: map[string]*FuncType{
			"Message": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: TypeString,
			},
			"ToString": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: TypeString,
			},
		},
		Embedded: []*InterfaceType{},
	}
	_ = rootScope.Declare("Error", errorIface)

	// Built-in Show interface: ToString(): String
	showIface := &InterfaceType{
		Name: "Show",
		Methods: map[string]*FuncType{
			"ToString": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: TypeString,
			},
		},
		Embedded: []*InterfaceType{},
	}
	_ = rootScope.Declare("Show", showIface)

	// Built-in print: (*args: Show) -> ()
	_ = rootScope.Declare("print", &FuncType{
		Arg: &TupleType{
			Elems:      []Type{showIface},
			IsVariadic: true,
		},
		Ret: &TupleType{Elems: []Type{}},
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
	// Built-in force: (Any) -> Any
	_ = rootScope.Declare("force", builtinForceFuncType)
	// Built-in Bytes: (String?) -> Bytes
	_ = rootScope.Declare("Bytes", builtinBytesFuncType)
	// Built-in gets: () -> String | Nil
	_ = rootScope.Declare("gets", &FuncType{
		Arg: &TupleType{Elems: []Type{}},
		Ret: NewUnionType(TypeString, TypeNil),
	})
	// Built-in Args: (String, ...)
	_ = rootScope.Declare("Args", &TupleType{
		Elems:      []Type{TypeString},
		IsVariadic: true,
	})

	// Global standard streams: Stdin (bufio.Reader), Stdout (bufio.Writer), Stderr (bufio.Writer)
	bufioReaderGlobal := &StructType{
		Name: "bufio.Reader",
		Methods: map[string]*FuncType{
			"Read": {
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: NewUnionType(TypeBytes, TypeNil),
			},
			"ReadLine": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: NewUnionType(TypeString, TypeNil),
			},
			"ReadString": {
				Arg: &TupleType{Elems: []Type{TypeString}},
				Ret: NewUnionType(TypeString, TypeNil),
			},
			"Peek": {
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: NewUnionType(TypeBytes, TypeNil),
			},
		},
	}
	bufioWriterGlobal := &StructType{
		Name: "bufio.Writer",
		Methods: map[string]*FuncType{
			"Write": {
				Arg: &TupleType{Elems: []Type{TypeBytes}},
				Ret: TypeInt,
			},
			"Flush": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: &TupleType{Elems: []Type{}},
			},
		},
	}
	stderrGlobal := &StructType{
		Name: "io.Stderr",
		Methods: map[string]*FuncType{
			"Write": {
				Arg: &TupleType{Elems: []Type{TypeBytes}},
				Ret: TypeInt,
			},
			"Close": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: &TupleType{Elems: []Type{}},
			},
			"Flush": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: &TupleType{Elems: []Type{}},
			},
		},
	}
	_ = rootScope.Declare("Stdin", bufioReaderGlobal)
	_ = rootScope.Declare("Stdout", bufioWriterGlobal)
	_ = rootScope.Declare("Stderr", stderrGlobal)

	// User code and module bodies evaluate in a child scope so that predeclared globals
	// are not included in local module exports.
	fileScope := NewScope(rootScope)

	inf := &Inferencer{
		nextVarID: 0,
		scope:     fileScope,
		modules:   make(map[string]*ModuleType),
	}
	inf.modules["io"] = makeIOModuleType("io")
	inf.modules["bufio"] = makeBufioModuleType("bufio")
	return inf
}

func makeIOModuleType(alias string) *ModuleType {
	readerIface := &InterfaceType{
		Name: "io.Reader",
		Methods: map[string]*FuncType{
			"Read": {
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: NewUnionType(TypeBytes, TypeNil),
			},
		},
		Embedded: []*InterfaceType{},
	}
	writerIface := &InterfaceType{
		Name: "io.Writer",
		Methods: map[string]*FuncType{
			"Write": {
				Arg: &TupleType{Elems: []Type{TypeBytes}},
				Ret: TypeInt,
			},
		},
		Embedded: []*InterfaceType{},
	}
	closerIface := &InterfaceType{
		Name: "io.Closer",
		Methods: map[string]*FuncType{
			"Close": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: &TupleType{Elems: []Type{}},
			},
		},
		Embedded: []*InterfaceType{},
	}
	readCloserIface := &InterfaceType{
		Name:     "io.ReadCloser",
		Embedded: []*InterfaceType{readerIface, closerIface},
		Methods: map[string]*FuncType{
			"Read":  readerIface.Methods["Read"],
			"Close": closerIface.Methods["Close"],
		},
	}
	writeCloserIface := &InterfaceType{
		Name:     "io.WriteCloser",
		Embedded: []*InterfaceType{writerIface, closerIface},
		Methods: map[string]*FuncType{
			"Write": writerIface.Methods["Write"],
			"Close": closerIface.Methods["Close"],
		},
	}
	stderrStruct := &StructType{
		Name: "io.Stderr",
		Methods: map[string]*FuncType{
			"Write": writerIface.Methods["Write"],
			"Close": closerIface.Methods["Close"],
			"Flush": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: &TupleType{Elems: []Type{}},
			},
		},
	}
	stdoutStruct := &StructType{
		Name: "io.Stdout",
		Methods: map[string]*FuncType{
			"Write": writerIface.Methods["Write"],
			"Close": closerIface.Methods["Close"],
			"Flush": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: &TupleType{Elems: []Type{}},
			},
		},
	}

	if alias == "" {
		alias = "io"
	}
	return &ModuleType{
		Name: alias,
		Exports: map[string]Type{
			"Reader":      readerIface,
			"Writer":      writerIface,
			"Closer":      closerIface,
			"ReadCloser":  readCloserIface,
			"WriteCloser": writeCloserIface,
			"Stdin":       readCloserIface,
			"Stdout":      stdoutStruct,
			"Stderr":      stderrStruct,
		},
		Types: map[string]Type{
			"Reader":      readerIface,
			"Writer":      writerIface,
			"Closer":      closerIface,
			"ReadCloser":  readCloserIface,
			"WriteCloser": writeCloserIface,
		},
	}
}

func makeBufioModuleType(alias string) *ModuleType {
	readerIface := &InterfaceType{
		Name: "io.Reader",
		Methods: map[string]*FuncType{
			"Read": {
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: NewUnionType(TypeBytes, TypeNil),
			},
		},
	}
	writerIface := &InterfaceType{
		Name: "io.Writer",
		Methods: map[string]*FuncType{
			"Write": {
				Arg: &TupleType{Elems: []Type{TypeBytes}},
				Ret: TypeInt,
			},
		},
	}
	readerStruct := &StructType{
		Name: "bufio.Reader",
		Methods: map[string]*FuncType{
			"Read": {
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: NewUnionType(TypeBytes, TypeNil),
			},
			"ReadLine": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: NewUnionType(TypeString, TypeNil),
			},
			"ReadString": {
				Arg: &TupleType{Elems: []Type{TypeString}},
				Ret: NewUnionType(TypeString, TypeNil),
			},
			"Peek": {
				Arg: &TupleType{Elems: []Type{TypeInt}},
				Ret: NewUnionType(TypeBytes, TypeNil),
			},
		},
	}
	writerStruct := &StructType{
		Name: "bufio.Writer",
		Methods: map[string]*FuncType{
			"Write": {
				Arg: &TupleType{Elems: []Type{TypeBytes}},
				Ret: TypeInt,
			},
			"Flush": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: &TupleType{Elems: []Type{}},
			},
		},
	}
	readerConstructorFunc := &FuncType{
		Arg: &TupleType{Elems: []Type{readerIface}},
		Ret: readerStruct,
	}
	writerConstructorFunc := &FuncType{
		Arg: &TupleType{Elems: []Type{writerIface}},
		Ret: writerStruct,
	}

	if alias == "" {
		alias = "bufio"
	}
	return &ModuleType{
		Name: alias,
		Exports: map[string]Type{
			"Reader": readerConstructorFunc,
			"Writer": writerConstructorFunc,
		},
		Types: map[string]Type{
			"Reader": readerStruct,
			"Writer": writerStruct,
		},
	}
}

func (inf *Inferencer) SetModuleLoader(loader func(path string) (*ModuleType, error)) {
	inf.moduleLoader = loader
}

func (inf *Inferencer) RegisterModule(name string, mod *ModuleType) {
	inf.modules[name] = mod
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
		valT, err := inf.inferExpr(s.Value)
		if err != nil {
			return err
		}
		if tt, ok := valT.(*TupleType); ok {
			for i, name := range s.Pattern.Elements {
				if name != "_" {
					var elemT Type = TypeAny
					if i < len(tt.Elems) {
						elemT = tt.Elems[i]
					} else if tt.IsVariadic && len(tt.Elems) > 0 {
						elemT = tt.Elems[len(tt.Elems)-1]
					}
					_ = inf.scope.Declare(name, elemT)
				}
			}
			if s.Pattern.RestName != "" && s.Pattern.RestName != "_" {
				var restElemT Type = TypeAny
				if tt.IsVariadic && len(tt.Elems) > 0 {
					restElemT = tt.Elems[len(tt.Elems)-1]
				} else if len(tt.Elems) > len(s.Pattern.Elements) {
					restElemT = tt.Elems[len(s.Pattern.Elements)]
				}
				_ = inf.scope.Declare(s.Pattern.RestName, &TupleType{Elems: []Type{restElemT}, IsVariadic: true})
			}
		} else {
			for _, name := range s.Pattern.Elements {
				if name != "_" {
					_ = inf.scope.Declare(name, TypeAny)
				}
			}
			if s.Pattern.RestName != "" && s.Pattern.RestName != "_" {
				_ = inf.scope.Declare(s.Pattern.RestName, &TupleType{Elems: []Type{TypeAny}, IsVariadic: true})
			}
		}
		return nil

	case *ast.FuncDef:
		paramTypes := make([]Type, len(s.Params))
		callerParamTypes := make([]Type, len(s.Params))
		isVariadic := false
		for i, p := range s.Params {
			var elemType Type = TypeAny
			if p.TypeAnnot != "" {
				elemType = inf.parseTypeAnnot(p.TypeAnnot)
			}
			if p.IsVariadic {
				isVariadic = true
				callerParamTypes[i] = elemType
				emptyTuple := &TupleType{Elems: []Type{}}
				streamTuple := &TupleType{Elems: []Type{elemType}, IsVariadic: true}
				paramTypes[i] = NewUnionType(emptyTuple, streamTuple)
			} else {
				callerParamTypes[i] = elemType
				paramTypes[i] = elemType
			}
		}
		var declaredRetType Type = TypeAny
		if s.ReturnType != "" {
			declaredRetType = inf.parseTypeAnnot(s.ReturnType)
		}

		fnType := &FuncType{
			Arg: &TupleType{
				Elems:      callerParamTypes,
				IsVariadic: isVariadic,
			},
			Ret: declaredRetType,
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
		var lastT Type = TypeAny
		for _, bStmt := range s.Body {
			if es, ok := bStmt.(*ast.ExprStmt); ok {
				if et, err := inf.inferExpr(es.Value); err == nil {
					lastT = et
				}
			} else if rs, ok := bStmt.(*ast.ReturnStmt); ok {
				if rs.Value != nil {
					if et, err := inf.inferExpr(rs.Value); err == nil {
						lastT = et
					}
				}
			}
			if err := inf.inferStmt(bStmt); err != nil {
				inf.scope = prevScope
				return err
			}
		}
		inf.scope = prevScope

		if s.ReturnType != "" {
			if !IsAssignableTo(lastT, declaredRetType) {
				return fmt.Errorf("return type mismatch in function '%s': expected %s, got %s", s.Name, declaredRetType, lastT)
			}
		} else if lastT != TypeAny {
			fnType.Ret = lastT
		}
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

	case *ast.ContinueStmt:
		return nil

	case *ast.ForStmt:
		if s.Cond != nil {
			_, err := inf.inferExpr(s.Cond)
			if err != nil {
				return err
			}
		}
		for _, bStmt := range s.Body {
			if err := inf.inferStmt(bStmt); err != nil {
				return err
			}
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
		if s.Path == "io" {
			alias := s.Alias
			if alias == "" {
				alias = "io"
			}
			inf.modules[alias] = makeIOModuleType(alias)
		} else if s.Path == "bufio" {
			alias := s.Alias
			if alias == "" {
				alias = "bufio"
			}
			inf.modules[alias] = makeBufioModuleType(alias)
		} else if inf.moduleLoader != nil {
			mod, err := inf.moduleLoader(s.Path)
			if err != nil {
				return err
			}
			inf.modules[s.Alias] = mod
		}
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
	case *ast.ForceExpr:
		return inf.inferExpr(e.Expr)
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
	case *ast.UnaryExpr:
		t, err := inf.inferExpr(e.Expr)
		if err != nil {
			return nil, err
		}
		switch e.Op {
		case "+", "-":
			if t == TypeFloat {
				return TypeFloat, nil
			}
			return TypeInt, nil
		case "!":
			return TypeBool, nil
		default:
			return TypeAny, nil
		}
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
		case "+":
			if leftT == TypeBytes && rightT == TypeBytes {
				return TypeBytes, nil
			}
			if leftT == TypeBytes || rightT == TypeBytes {
				return nil, fmt.Errorf("cannot add %s and %s", leftT, rightT)
			}
			if leftT == TypeString || rightT == TypeString {
				return TypeString, nil
			}
			if leftT == TypeFloat || rightT == TypeFloat {
				return TypeFloat, nil
			}
			return TypeInt, nil
		case "-", "*", "/", "%":
			if leftT == TypeBytes || rightT == TypeBytes {
				return nil, fmt.Errorf("cannot perform %s operation on %s and %s", e.Op, leftT, rightT)
			}
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
		if id, ok := e.Callee.(*ast.Identifier); ok && id.Name == "force" && len(e.Args) == 1 {
			if t, ok := inf.scope.Lookup("force"); ok && t == builtinForceFuncType {
				return inf.inferExpr(e.Args[0])
			}
		}
		if id, ok := e.Callee.(*ast.Identifier); ok && id.Name == "Bytes" {
			if t, ok := inf.scope.Lookup("Bytes"); ok && t == builtinBytesFuncType {
				if len(e.Args) > 1 {
					return nil, fmt.Errorf("Bytes expects 0 or 1 arguments, got %d", len(e.Args))
				}
				if len(e.Args) == 1 {
					argT, err := inf.inferExpr(e.Args[0])
					if err != nil {
						return nil, err
					}
					if !IsAssignableTo(argT, TypeString) && !IsAssignableTo(argT, TypeBytes) {
						return nil, fmt.Errorf("Bytes expects String or Bytes, got %s", argT)
					}
				}
				return TypeBytes, nil
			}
		}
		if st, ok := calleeT.(*StructType); ok {
			return st, nil
		}
		if iface, ok := calleeT.(*InterfaceType); ok && iface.Name == "Error" {
			return iface, nil
		}
		if fnT, ok := calleeT.(*FuncType); ok {
			args := e.Args
			if e.Block != nil {
				args = append(append([]ast.Expr(nil), e.Args...), &ast.LambdaExpr{
					Params: e.Block.Params,
					Body:   e.Block.Body,
				})
			}
			if argTuple, ok := fnT.Arg.(*TupleType); ok {
				if !argTuple.IsVariadic && len(args) != len(argTuple.Elems) {
					return nil, fmt.Errorf("function expects %d arguments, got %d", len(argTuple.Elems), len(args))
				}
				if argTuple.IsVariadic && len(argTuple.Elems) > 0 && len(args) < len(argTuple.Elems)-1 {
					return nil, fmt.Errorf("function expects at least %d arguments, got %d", len(argTuple.Elems)-1, len(args))
				}
				for i, argExpr := range args {
					argT, err := inf.inferExpr(argExpr)
					if err != nil {
						return nil, err
					}
					if i < len(argTuple.Elems) {
						expectedParamT := argTuple.Elems[i]
						if !IsAssignableTo(argT, expectedParamT) {
							return nil, fmt.Errorf("type mismatch in argument %d: expected %s, got %s", i+1, expectedParamT, argT)
						}
					} else if argTuple.IsVariadic && len(argTuple.Elems) > 0 {
						expectedParamT := argTuple.Elems[len(argTuple.Elems)-1]
						if !IsAssignableTo(argT, expectedParamT) {
							return nil, fmt.Errorf("type mismatch in variadic argument %d: expected %s, got %s", i+1, expectedParamT, argT)
						}
					}
				}
			}
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
		if bSig, ok := LookupBuiltinTypeMethod(targetT, e.Member); ok {
			return bSig, nil
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
		} else if targetT == TypeString {
			return TypeString, nil
		} else if targetT == TypeBytes {
			return TypeInt, nil
		}
		return TypeAny, nil
	case *ast.ModuleAccessExpr:
		if mod, ok := inf.modules[e.Module]; ok {
			if t, ok := mod.Exports[e.Member]; ok {
				return t, nil
			}
			return nil, fmt.Errorf("undefined member '%s' in module '%s'", e.Member, e.Module)
		}
		return TypeAny, nil
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
	case *ast.BlockLiteral:
		return inf.inferExpr(&ast.LambdaExpr{
			Params: e.Params,
			Body:   e.Body,
		})

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
	// Careful not to split inside parentheses, brackets, or braces
	var parts []string
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
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
		} else if r == '{' {
			braceDepth++
		} else if r == '}' {
			braceDepth--
		} else if r == '|' && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
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

	// 2. Check top-level Function Type: Arg -> Ret
	// Arrow '->' is right-associative: A -> B -> C is A -> (B -> C)
	arrowIdx := -1
	parenDepth = 0
	bracketDepth = 0
	braceDepth = 0
	for i := 0; i < len(runes)-1; i++ {
		r := runes[i]
		if r == '(' {
			parenDepth++
		} else if r == ')' {
			parenDepth--
		} else if r == '[' {
			bracketDepth++
		} else if r == ']' {
			bracketDepth--
		} else if r == '{' {
			braceDepth++
		} else if r == '}' {
			braceDepth--
		} else if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && r == '-' && runes[i+1] == '>' {
			arrowIdx = i
			break
		}
	}

	if arrowIdx != -1 {
		argStr := strings.TrimSpace(string(runes[:arrowIdx]))
		retStr := strings.TrimSpace(string(runes[arrowIdx+2:]))

		var argType Type
		if strings.HasPrefix(argStr, "(") && strings.HasSuffix(argStr, ")") {
			inner := strings.TrimSpace(argStr[1 : len(argStr)-1])
			if inner == "" {
				argType = &TupleType{Elems: []Type{}}
			} else {
				elems := inf.parseTupleElements(inner)
				argType = &TupleType{Elems: elems}
			}
		} else if argStr == "" {
			argType = &TupleType{Elems: []Type{}}
		} else {
			elemType := inf.parseTypeAnnot(argStr)
			argType = &TupleType{Elems: []Type{elemType}}
		}

		retType := inf.parseTypeAnnot(retStr)
		return &FuncType{
			Arg: argType,
			Ret: retType,
		}
	}

	switch annot {
	case "Int":
		return TypeInt
	case "Float":
		return TypeFloat
	case "String":
		return TypeString
	case "Bytes":
		return TypeBytes
	case "Bool":
		return TypeBool
	case "Nil", "nil":
		return TypeNil
	case "Any":
		return TypeAny
	}

	// Tuple annotation or grouping: (Int, ...) or (Int, String) or ((Int) -> String)
	if strings.HasPrefix(annot, "(") && strings.HasSuffix(annot, ")") {
		inner := strings.TrimSpace(annot[1 : len(annot)-1])
		if inner == "" {
			return &TupleType{Elems: []Type{}}
		}
		// Check if there is a top-level comma
		hasTopLevelComma := false
		pDepth := 0
		bDepth := 0
		brDepth := 0
		for _, r := range inner {
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
				hasTopLevelComma = true
				break
			}
		}

		if !hasTopLevelComma {
			// Single item in parens: grouping like ((Int) -> String) or (Int)
			return inf.parseTypeAnnot(inner)
		}

		elems := inf.parseTupleElements(inner)
		isVariadic := false
		for _, r := range strings.Split(inner, ",") {
			if strings.TrimSpace(r) == "..." {
				isVariadic = true
				break
			}
		}
		return &TupleType{
			Elems:      elems,
			IsVariadic: isVariadic,
		}
	}

	// Dict annotation: {Key: Value}
	if strings.HasPrefix(annot, "{") && strings.HasSuffix(annot, "}") {
		inner := strings.TrimSpace(annot[1 : len(annot)-1])
		colonIdx := -1
		pDepth := 0
		bDepth := 0
		brDepth := 0
		innerRunes := []rune(inner)
		for i, r := range innerRunes {
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
			} else if r == ':' && pDepth == 0 && bDepth == 0 && brDepth == 0 {
				colonIdx = i
				break
			}
		}

		if colonIdx != -1 {
			keyStr := strings.TrimSpace(string(innerRunes[:colonIdx]))
			valStr := strings.TrimSpace(string(innerRunes[colonIdx+1:]))
			keyType := inf.parseTypeAnnot(keyStr)
			valType := inf.parseTypeAnnot(valStr)
			return &DictType{Key: keyType, Value: valType}
		}
		return &DictType{Key: TypeAny, Value: TypeAny}
	}

	if inf != nil {
		if strings.HasPrefix(annot, "@") {
			parts := strings.SplitN(annot[1:], ".", 2)
			if len(parts) == 2 {
				modName := parts[0]
				typeName := parts[1]
				if mod, ok := inf.modules[modName]; ok {
					if mod.Types != nil {
						if t, ok := mod.Types[typeName]; ok {
							return t
						}
					}
					if t, ok := mod.Exports[typeName]; ok {
						return t
					}
				}
			}
		}
		if inf.scope != nil {
			if t, ok := inf.scope.Lookup(annot); ok {
				return t
			}
		}
	}

	return TypeAny
}

func (inf *Inferencer) parseTupleElements(inner string) []Type {
	var parts []string
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	start := 0
	runes := []rune(inner)
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
		} else if r == '{' {
			braceDepth++
		} else if r == '}' {
			braceDepth--
		} else if r == ',' && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
			parts = append(parts, string(runes[start:i]))
			start = i + 1
		}
	}
	parts = append(parts, string(runes[start:]))

	var elems []Type
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" && trimmed != "..." {
			elems = append(elems, inf.parseTypeAnnot(trimmed))
		}
	}
	return elems
}
