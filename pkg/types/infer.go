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
	ExprTypes    map[ast.Expr]Type
	StmtTypes    map[ast.Stmt]Type
	filename     string
}

var BuiltinBytesFuncType = &FuncType{
	Arg: &TupleType{
		Elems: []Type{NewUnionType(TypeString, TypeBytes)},
	},
	MinArity: 0,
	MaxArity: 1,
	Ret:      TypeBytes,
}

var BuiltinExitFuncType = &FuncType{
	Arg: &TupleType{
		Elems: []Type{TypeInt},
	},
	MinArity: 0,
	MaxArity: 1,
	Ret:      TypeNil,
}

var BuiltinPrintFuncType = &FuncType{
	Arg: &TupleType{
		Elems:      []Type{BuiltinShowIface},
		IsVariadic: true,
	},
	Ret: &TupleType{Elems: []Type{}},
}

var BuiltinGetsFuncType = &FuncType{
	Arg: &TupleType{Elems: []Type{}},
	Ret: NewUnionType(TypeString, TypeNil),
}

var BuiltinEqualIface = &InterfaceType{
	Name: "Equal",
	Methods: map[string]*FuncType{
		"==": {
			Arg:      &TupleType{Elems: []Type{TypeUnknown}},
			MinArity: 1,
			MaxArity: 1,
			Ret:      TypeBool,
		},
	},
	Embedded: []*InterfaceType{},
}

var BuiltinComparableIface = &InterfaceType{
	Name: "Comparable",
	Methods: map[string]*FuncType{
		"==": {
			Arg:      &TupleType{Elems: []Type{TypeUnknown}},
			MinArity: 1,
			MaxArity: 1,
			Ret:      TypeBool,
		},
		"<": {
			Arg:      &TupleType{Elems: []Type{TypeUnknown}},
			MinArity: 1,
			MaxArity: 1,
			Ret:      TypeBool,
		},
	},
	Embedded: []*InterfaceType{BuiltinEqualIface},
}

var BuiltinShowIface = &InterfaceType{
	Name: "Show",
	Methods: map[string]*FuncType{
		"ToString": {
			Arg: &TupleType{Elems: []Type{}},
			Ret: TypeString,
		},
		"Type": {
			Arg: &TupleType{Elems: []Type{}},
			Ret: TypeString,
		},
	},
	Embedded: []*InterfaceType{},
}

var BuiltinErrorIface = &InterfaceType{
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

func NewInferencer() *Inferencer {
	rootScope := NewScope(nil)

	// Built-in Error interface: Message(): String, ToString(): String
	_ = rootScope.Declare("Error", BuiltinErrorIface)

	// Built-in Show interface: ToString(): String, Type(): String
	_ = rootScope.Declare("Show", BuiltinShowIface)

	// Built-in Equal & Comparable interfaces
	_ = rootScope.Declare("Equal", BuiltinEqualIface)
	_ = rootScope.Declare("Comparable", BuiltinComparableIface)

	// Built-in print: (*args: Show) -> ()
	_ = rootScope.Declare("print", BuiltinPrintFuncType)
	// Built-in exit: ([code: Int]) -> Nil
	_ = rootScope.Declare("exit", BuiltinExitFuncType)
	// Built-in Bytes: (String?) -> Bytes
	_ = rootScope.Declare("Bytes", BuiltinBytesFuncType)
	// Built-in gets: () -> String | Nil
	_ = rootScope.Declare("gets", BuiltinGetsFuncType)
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
		ExprTypes: make(map[ast.Expr]Type),
		StmtTypes: make(map[ast.Stmt]Type),
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
			"Flush": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: &TupleType{Elems: []Type{}},
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
			"Flush": {
				Arg: &TupleType{Elems: []Type{}},
				Ret: &TupleType{Elems: []Type{}},
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

func (inf *Inferencer) Scope() *Scope {
	return inf.scope
}

func (inf *Inferencer) SetFilename(filename string) {
	inf.filename = filename
}

func (inf *Inferencer) nodeErr(node ast.Node, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	// Avoid double-formatting if message already begins with location
	if strings.Contains(msg, " at col ") {
		return fmt.Errorf("%s", msg)
	}
	fn := inf.filename
	pos := ast.Position{}
	if node != nil {
		pos = node.Pos()
	}
	if pos.Line > 0 {
		if fn != "" {
			return fmt.Errorf("%s:%d: %s at col %d", fn, pos.Line, msg, pos.Col)
		}
		return fmt.Errorf("%d: %s at col %d", pos.Line, msg, pos.Col)
	}
	if fn != "" {
		return fmt.Errorf("%s: %s", fn, msg)
	}
	return fmt.Errorf("%s", msg)
}

func (inf *Inferencer) freshVar() *TypeVar {
	v := &TypeVar{ID: inf.nextVarID}
	inf.nextVarID++
	return v
}

func (inf *Inferencer) InferProgram(prog *ast.Program) error {
	if prog.Filename != "" {
		inf.filename = prog.Filename
	}
	for _, stmt := range prog.Body {
		if err := inf.inferStmt(stmt); err != nil {
			return inf.nodeErr(stmt, "%s", err.Error())
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
			expected, err := inf.parseTypeAnnot(s.TypeAnnot)
			if err != nil {
				return err
			}
			if !IsAssignableTo(valType, expected) {
				return inf.nodeErr(s, "type mismatch for variable '%s': expected %s, got %s", s.Name, expected, valType)
			}
			targetType = expected
		}
		// Enforce same-scope duplicate declaration prohibition
		if err := inf.scope.Declare(s.Name, targetType); err != nil {
			return inf.nodeErr(s, "%s", err.Error())
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
					var elemT Type = TypeUnknown
					if i < len(tt.Elems) {
						elemT = tt.Elems[i]
					} else if tt.IsVariadic && len(tt.Elems) > 0 {
						elemT = tt.Elems[len(tt.Elems)-1]
					}
					_ = inf.scope.Declare(name, elemT)
				}
			}
			if s.Pattern.RestName != "" && s.Pattern.RestName != "_" {
				var restElemT Type = TypeUnknown
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
					_ = inf.scope.Declare(name, TypeUnknown)
				}
			}
			if s.Pattern.RestName != "" && s.Pattern.RestName != "_" {
				_ = inf.scope.Declare(s.Pattern.RestName, &TupleType{Elems: []Type{TypeUnknown}, IsVariadic: true})
			}
		}
		return nil

	case *ast.FuncDef:
		paramTypes := make([]Type, len(s.Params))
		callerParamTypes := make([]Type, len(s.Params))
		isVariadic := false
		minArity := 0
		for i, p := range s.Params {
			var elemType Type = TypeUnknown
			if p.TypeAnnot != "" {
				t, err := inf.parseTypeAnnot(p.TypeAnnot)
				if err != nil {
					return err
				}
				elemType = t
			}
			if p.DefaultValue != nil {
				defT, err := inf.inferExpr(p.DefaultValue)
				if err != nil {
					return err
				}
				if elemType != TypeUnknown && !IsAssignableTo(defT, elemType) {
					return inf.nodeErr(p.DefaultValue, "type mismatch in default value for parameter '%s': expected %s, got %s", p.Name, elemType, defT)
				}
			} else if !p.IsVariadic {
				minArity++
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
		maxArity := len(s.Params)
		if isVariadic {
			maxArity = -1
		}
		var blockT Type
		blockOpt := false
		if s.BlockParam != nil {
			blockT = TypeUnknown
			if s.BlockParam.TypeAnnot != "" {
				t, err := inf.parseTypeAnnot(s.BlockParam.TypeAnnot)
				if err != nil {
					return err
				}
				blockT = t
			}
			blockOpt = (s.BlockParam.DefaultValue != nil)
			if s.BlockParam.DefaultValue != nil {
				defT, err := inf.inferExpr(s.BlockParam.DefaultValue)
				if err != nil {
					return err
				}
				if blockT != TypeUnknown && !IsAssignableTo(defT, blockT) {
					return inf.nodeErr(s.BlockParam.DefaultValue, "type mismatch in default value for block parameter '&%s': expected %s, got %s", s.BlockParam.Name, blockT, defT)
				}
			}
		}

		var declaredRetType Type = TypeUnknown
		if s.ReturnType != "" {
			t, err := inf.parseTypeAnnot(s.ReturnType)
			if err != nil {
				return err
			}
			declaredRetType = t
		}

		fnType := &FuncType{
			Arg: &TupleType{
				Elems:      callerParamTypes,
				IsVariadic: isVariadic,
			},
			MinArity:   minArity,
			MaxArity:   maxArity,
			BlockParam: blockT,
			BlockOpt:   blockOpt,
			Ret:        declaredRetType,
		}
		if inf.StmtTypes != nil {
			inf.StmtTypes[s] = fnType
		}
		// Function declaration in current scope
		if err := inf.scope.Declare(s.Name, fnType); err != nil {
			return inf.nodeErr(s, "%s", err.Error())
		}

		// Create child scope for function parameters and body (shadowing allowed)
		childScope := NewScope(inf.scope)
		for i, p := range s.Params {
			if err := childScope.Declare(p.Name, paramTypes[i]); err != nil {
				return inf.nodeErr(s, "duplicate parameter '%s' in function '%s'", p.Name, s.Name)
			}
		}
		if s.BlockParam != nil {
			if err := childScope.Declare(s.BlockParam.Name, blockT); err != nil {
				return inf.nodeErr(s, "duplicate parameter '&%s' in function '%s'", s.BlockParam.Name, s.Name)
			}
		}

		prevScope := inf.scope
		inf.scope = childScope
		var lastT Type = TypeUnknown
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
				return inf.nodeErr(s, "return type mismatch in function '%s': expected %s, got %s", s.Name, declaredRetType, lastT)
			}
		} else if lastT != TypeUnknown {
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

	case *ast.IfStmt:
		if s.Cond != nil {
			if _, err := inf.inferExpr(s.Cond); err != nil {
				return err
			}
		}
		thenScope := NewScope(inf.scope)
		prevScope := inf.scope
		inf.scope = thenScope
		for _, bStmt := range s.ThenBody {
			if err := inf.inferStmt(bStmt); err != nil {
				inf.scope = prevScope
				return err
			}
		}
		inf.scope = prevScope
		if len(s.ElseBody) > 0 {
			elseScope := NewScope(inf.scope)
			inf.scope = elseScope
			for _, bStmt := range s.ElseBody {
				if err := inf.inferStmt(bStmt); err != nil {
					inf.scope = prevScope
					return err
				}
			}
			inf.scope = prevScope
		}
		return nil

	case *ast.MemberAssignStmt:
		targetT, err := inf.inferExpr(s.Target)
		if err != nil {
			return err
		}
		st, ok := targetT.(*StructType)
		if !ok {
			return inf.nodeErr(s, "cannot assign member '%s' on non-struct type %s", s.Member, targetT)
		}
		expectedT, ok := st.Fields[s.Member]
		if !ok {
			return inf.nodeErr(s, "struct '%s' has no field '%s'", st.Name, s.Member)
		}
		valT, err := inf.inferExpr(s.Value)
		if err != nil {
			return err
		}
		if !IsAssignableTo(valT, expectedT) {
			return inf.nodeErr(s, "cannot assign type %s to field '%s' of type %s in struct '%s'", valT, s.Member, expectedT, st.Name)
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
		if inf.StmtTypes != nil {
			inf.StmtTypes[s] = st
		}

		// Declare struct type in current scope so methods/self can reference it
		if err := inf.scope.Declare(s.Name, st); err != nil {
			return inf.nodeErr(s, "%s", err.Error())
		}

		// Process fields
		for _, f := range s.Fields {
			if f.IsEmbedded {
				embType, ok := inf.scope.Lookup(f.TypeAnnot)
				if !ok {
					return inf.nodeErr(s, "undefined embedded struct '%s' in struct '%s'", f.TypeAnnot, s.Name)
				}
				embStruct, ok := embType.(*StructType)
				if !ok {
					return inf.nodeErr(s, "cannot embed non-struct type '%s' in struct '%s'", f.TypeAnnot, s.Name)
				}
				st.Embedded = append(st.Embedded, embStruct)
				st.Fields[f.Name] = embStruct
				st.FieldOrder = append(st.FieldOrder, f.Name)
			} else {
				fieldType, err := inf.parseTypeAnnot(f.TypeAnnot)
				if err != nil {
					return err
				}
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

			isVariadic := false
			callerParamTypes := make([]Type, 0, len(m.Params))
			for _, p := range m.Params {
				if p.Name == "self" {
					continue
				}
				var pType Type = TypeUnknown
				if p.TypeAnnot != "" {
					t, err := inf.parseTypeAnnot(p.TypeAnnot)
					if err != nil {
						return err
					}
					pType = t
				} else if m.Name == "init" {
					if fieldT, ok := st.Fields[p.Name]; ok {
						if p.IsVariadic {
							if tup, isTup := fieldT.(*TupleType); isTup && len(tup.Elems) > 0 {
								pType = tup.Elems[0]
							} else {
								pType = fieldT
							}
						} else {
							pType = fieldT
						}
					}
				}

				if p.IsVariadic {
					isVariadic = true
					callerParamTypes = append(callerParamTypes, pType)
					emptyTuple := &TupleType{Elems: []Type{}}
					streamTuple := &TupleType{Elems: []Type{pType}, IsVariadic: true}
					_ = methodScope.Declare(p.Name, NewUnionType(emptyTuple, streamTuple))
				} else {
					callerParamTypes = append(callerParamTypes, pType)
					_ = methodScope.Declare(p.Name, pType)
				}
			}

			minArity := 0
			for _, p := range m.Params {
				if p.Name == "self" {
					continue
				}
				if p.DefaultValue == nil && !p.IsVariadic {
					minArity++
				}
			}
			maxArity := len(callerParamTypes)
			if isVariadic {
				maxArity = -1
			}
			var blockT Type
			blockOpt := false
			if m.BlockParam != nil {
				blockT = TypeUnknown
				if m.BlockParam.TypeAnnot != "" {
					t, err := inf.parseTypeAnnot(m.BlockParam.TypeAnnot)
					if err != nil {
						return err
					}
					blockT = t
				}
				blockOpt = (m.BlockParam.DefaultValue != nil)
				_ = methodScope.Declare(m.BlockParam.Name, blockT)
			}

			prevScope := inf.scope
			inf.scope = methodScope
			var lastExprType Type = TypeUnknown
			for _, bStmt := range m.Body {
				if err := inf.inferStmt(bStmt); err != nil {
					inf.scope = prevScope
					return inf.nodeErr(bStmt, "%s", err.Error())
				}
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

			var retType Type = TypeUnknown
			if m.ReturnType != "" {
				t, err := inf.parseTypeAnnot(m.ReturnType)
				if err != nil {
					return inf.nodeErr(m, "%s", err.Error())
				}
				retType = t
			} else if m.Name == "init" {
				retType = st
			} else if lastExprType != nil {
				retType = lastExprType
			}


			mType := &FuncType{
				Arg: &TupleType{
					Elems:      callerParamTypes,
					IsVariadic: isVariadic,
				},
				MinArity:   minArity,
				MaxArity:   maxArity,
				BlockParam: blockT,
				BlockOpt:   blockOpt,
				Ret:        retType,
			}
			if existing, ok := st.Methods[m.Name]; ok && (m.Name == "+" || m.Name == "-") {
				var unaryFT, binaryFT *FuncType
				if mType.MaxArity == 0 && existing.MinArity >= 1 {
					unaryFT = mType
					binaryFT = existing
				} else if existing.MaxArity == 0 && mType.MinArity >= 1 {
					unaryFT = existing
					binaryFT = mType
				}
				if unaryFT != nil && binaryFT != nil {
					ret := binaryFT.Ret
					if !unaryFT.Ret.Equals(binaryFT.Ret) {
						ret = NewUnionType(unaryFT.Ret, binaryFT.Ret)
					}
					st.Methods[m.Name] = &FuncType{
						Arg:           binaryFT.Arg,
						OverloadUnion: fmt.Sprintf("%s | %s", unaryFT.String(), binaryFT.String()),
						Overloads:     []*FuncType{unaryFT, binaryFT},
						MinArity:      0,
						MaxArity:      binaryFT.MaxArity,
						Ret:           ret,
					}
				} else {
					st.Methods[m.Name] = mType
				}
			} else {
				st.Methods[m.Name] = mType
			}
		}

		if _, hasInit := st.Methods["init"]; !hasInit {
			embeddedSet := make(map[string]bool)
			for _, emb := range st.Embedded {
				embeddedSet[emb.Name] = true
			}
			var initParamTypes []Type
			minArity := 0
			for _, fName := range st.FieldOrder {
				if embeddedSet[fName] {
					continue
				}
				initParamTypes = append(initParamTypes, st.Fields[fName])
				// Check if field has default value
				hasDefault := false
				for _, f := range s.Fields {
					if f.Name == fName && f.DefaultValue != nil {
						hasDefault = true
						break
					}
				}
				if !hasDefault {
					minArity++
				}
			}
			st.Methods["init"] = &FuncType{
				Arg: &TupleType{
					Elems: initParamTypes,
				},
				MinArity: minArity,
				MaxArity: len(initParamTypes),
				Ret:      st,
			}
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
				return inf.nodeErr(s, "undefined embedded interface '%s' in interface '%s'", embName, s.Name)
			}
			embIface, ok := embType.(*InterfaceType)
			if !ok {
				return inf.nodeErr(s, "cannot embed non-interface type '%s' in interface '%s'", embName, s.Name)
			}
			iface.Embedded = append(iface.Embedded, embIface)

			for mName, mSig := range embIface.Methods {
				if existingSig, exists := iface.Methods[mName]; exists {
					if !existingSig.Equals(mSig) {
						return inf.nodeErr(s, "conflicting method signatures for '%s' in embedded interface '%s' of '%s'", mName, embName, s.Name)
					}
				} else {
					iface.Methods[mName] = mSig
				}
			}
		}

		if err := inf.scope.Declare(s.Name, iface); err != nil {
			return inf.nodeErr(s, "%s", err.Error())
		}

		// Next, process direct methods on interface
		for _, m := range s.Methods {
			paramTypes := make([]Type, len(m.Params))
			minArity := 0
			for i, p := range m.Params {
				var pType Type = TypeUnknown
				if p.TypeAnnot != "" {
					t, err := inf.parseTypeAnnot(p.TypeAnnot)
					if err != nil {
						return err
					}
					pType = t
				}
				paramTypes[i] = pType
				if p.DefaultValue == nil && !p.IsVariadic {
					minArity++
				}
			}
			maxArity := len(m.Params)
			var blockT Type
			blockOpt := false
			if m.BlockParam != nil {
				blockT = TypeUnknown
				if m.BlockParam.TypeAnnot != "" {
					t, err := inf.parseTypeAnnot(m.BlockParam.TypeAnnot)
					if err != nil {
						return err
					}
					blockT = t
				}
				blockOpt = (m.BlockParam.DefaultValue != nil)
			}
			var retType Type = TypeUnknown
			if m.ReturnType != "" {
				t, err := inf.parseTypeAnnot(m.ReturnType)
				if err != nil {
					return err
				}
				retType = t
			}
			mSig := &FuncType{
				Arg:        &TupleType{Elems: paramTypes},
				MinArity:   minArity,
				MaxArity:   maxArity,
				BlockParam: blockT,
				BlockOpt:   blockOpt,
				Ret:        retType,
			}

			if existingSig, exists := iface.Methods[m.Name]; exists {
				if !existingSig.Equals(mSig) {
					return inf.nodeErr(s, "method '%s' in interface '%s' conflicts with embedded method signature", m.Name, s.Name)
				}
			}
			iface.Methods[m.Name] = mSig
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
	t, err := inf.inferExprInternal(expr)
	if err == nil && t != nil && inf != nil && inf.ExprTypes != nil && expr != nil {
		inf.ExprTypes[expr] = t
	}
	return t, err
}

func (inf *Inferencer) inferExprInternal(expr ast.Expr) (Type, error) {
	if expr == nil {
		return TypeUnknown, nil
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
		return TypeUnknown, nil // forward reference or flexible variable
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
		if len(e.Entries) == 0 {
			return &DictType{Key: TypeUnknown, Value: TypeUnknown}, nil
		}
		var keyTypes []Type
		var valTypes []Type
		for _, entry := range e.Entries {
			k, err := inf.inferExpr(entry.Key)
			if err != nil {
				return nil, err
			}
			v, err := inf.inferExpr(entry.Value)
			if err != nil {
				return nil, err
			}
			keyTypes = append(keyTypes, k)
			valTypes = append(valTypes, v)
		}
		return &DictType{Key: NewUnionType(keyTypes...), Value: NewUnionType(valTypes...)}, nil
	case *ast.UnaryExpr:
		t, err := inf.inferExpr(e.Expr)
		if err != nil {
			return nil, err
		}
		if st, ok := t.(*StructType); ok {
			if m, err := st.LookupMember(e.Op); err == nil {
				if fnT, ok := m.(*FuncType); ok {
					if len(fnT.Overloads) > 0 {
						for _, ov := range fnT.Overloads {
							if ov.MinArity == 0 && ov.MaxArity == 0 {
								return ov.Ret, nil
							}
						}
					}
					return fnT.Ret, nil
				}
			}
		}
		if it, ok := t.(*InterfaceType); ok {
			if m, ok := it.LookupMethod(e.Op); ok {
				return m.Ret, nil
			}
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
			return TypeUnknown, nil
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
		if e.Op == "!=" {
			// Desugars to !(left == right)
			if st, ok := leftT.(*StructType); ok {
				if m, err := st.LookupMember("=="); err == nil {
					if fnT, ok := m.(*FuncType); ok {
						return fnT.Ret, nil
					}
				}
			}
			if it, ok := leftT.(*InterfaceType); ok {
				if m, ok := it.LookupMethod("=="); ok {
					return m.Ret, nil
				}
			}
			return TypeBool, nil
		}
		if st, ok := leftT.(*StructType); ok {
			if m, err := st.LookupMember(e.Op); err == nil {
				if fnT, ok := m.(*FuncType); ok {
					if len(fnT.Overloads) > 0 {
						for _, ov := range fnT.Overloads {
							if ov.MinArity >= 1 || ov.MaxArity >= 1 {
								return ov.Ret, nil
							}
						}
					}
					return fnT.Ret, nil
				}
			}
			if e.Op == "<=" || e.Op == ">" || e.Op == ">=" {
				if m, err := st.LookupMember("<"); err == nil {
					if fnT, ok := m.(*FuncType); ok {
						return fnT.Ret, nil
					}
				}
			}
		}
		if it, ok := leftT.(*InterfaceType); ok {
			if m, ok := it.LookupMethod(e.Op); ok {
				return m.Ret, nil
			}
			if e.Op == "<=" || e.Op == ">" || e.Op == ">=" {
				if m, ok := it.LookupMethod("<"); ok {
					return m.Ret, nil
				}
			}
		}
		switch e.Op {
		case "+":
			if leftT == TypeBytes && rightT == TypeBytes {
				return TypeBytes, nil
			}
			if leftT == TypeBytes || rightT == TypeBytes {
				return nil, inf.nodeErr(e, "cannot add %s and %s", leftT, rightT)
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
				return nil, inf.nodeErr(e, "cannot perform %s operation on %s and %s", e.Op, leftT, rightT)
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
			return TypeUnknown, nil
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
			isCopyUpdate := false
			if len(e.Args) > 0 {
				if _, isNamed := e.Args[0].(*ast.NamedArgExpr); !isNamed {
					if firstT, err := inf.inferExpr(e.Args[0]); err == nil && firstT.Equals(st) {
						isCopyUpdate = true
					}
				}
			}
			if !isCopyUpdate {
				if initFn, hasInit := st.Methods["init"]; hasInit {
					if argTuple, ok := initFn.Arg.(*TupleType); ok {
						var posArgs []ast.Expr
						hasNamed := false
						namedNames := make(map[string]bool)
						for _, arg := range e.Args {
							if named, isNamed := arg.(*ast.NamedArgExpr); isNamed {
								hasNamed = true
								namedNames[named.Name] = true
								// Validate named argument type if field exists
								if expectedFieldT, hasField := st.Fields[named.Name]; hasField {
									valT, err := inf.inferExpr(named.Value)
									if err != nil {
										return nil, err
									}
									if !IsAssignableTo(valT, expectedFieldT) {
										return nil, inf.nodeErr(named.Value, "type mismatch for field '%s' in struct '%s': expected %s, got %s", named.Name, st.Name, expectedFieldT, valT)
									}
								}
							} else {
								posArgs = append(posArgs, arg)
							}
						}

						minArgs := initFn.MinArgs()
						maxArgs := initFn.MaxArgs()
						if !hasNamed {
							if len(posArgs) < minArgs {
								return nil, inf.nodeErr(e, "struct '%s' constructor expects at least %d arguments, got %d", st.Name, minArgs, len(posArgs))
							}
							if maxArgs != -1 && len(posArgs) > maxArgs {
								return nil, inf.nodeErr(e, "struct '%s' constructor expects at most %d arguments, got %d", st.Name, maxArgs, len(posArgs))
							}
						} else {
							// When named arguments are present, positional arguments must not exceed maxArgs
							if maxArgs != -1 && len(posArgs) > maxArgs {
								return nil, inf.nodeErr(e, "struct '%s' constructor expects at most %d arguments, got %d", st.Name, maxArgs, len(posArgs))
							}
						}

						for i, arg := range posArgs {
							var expectedParamT Type
							if i < len(argTuple.Elems)-1 {
								expectedParamT = argTuple.Elems[i]
							} else if argTuple.IsVariadic && len(argTuple.Elems) > 0 {
								expectedParamT = argTuple.Elems[len(argTuple.Elems)-1]
							} else if i < len(argTuple.Elems) {
								expectedParamT = argTuple.Elems[i]
							}
							if expectedParamT != nil {
								argT, err := inf.inferExpr(arg)
								if err != nil {
									return nil, err
								}
								if !IsAssignableTo(argT, expectedParamT) {
									return nil, inf.nodeErr(arg, "type mismatch in argument %d of struct '%s' constructor: expected %s, got %s", i+1, st.Name, expectedParamT, argT)
								}
							}
						}
					}
				}
			}
			return st, nil
		}
		if iface, ok := calleeT.(*InterfaceType); ok && iface.Name == "Error" {
			for _, argExpr := range e.Args {
				if _, err := inf.inferExpr(argExpr); err != nil {
					return nil, err
				}
			}
			return iface, nil
		}
		if fnT, ok := calleeT.(*FuncType); ok {
			args := e.Args
			if fnT.BlockParam != nil {
				if e.Block != nil {
					bT, err := inf.inferExpr(e.Block)
					if err != nil {
						return nil, err
					}
					if fnT.BlockParam != TypeUnknown && !IsAssignableTo(bT, fnT.BlockParam) {
						return nil, inf.nodeErr(e.Block, "type mismatch for block argument: expected %s, got %s", fnT.BlockParam, bT)
					}
				} else if !fnT.BlockOpt {
					return nil, inf.nodeErr(e, "function requires a block argument (&block)")
				}
			} else {
				if e.Block != nil {
					args = append(append([]ast.Expr(nil), e.Args...), e.Block)
				}
			}

			if argTuple, ok := fnT.Arg.(*TupleType); ok {
				minArgs := fnT.MinArgs()
				maxArgs := fnT.MaxArgs()
				if len(args) < minArgs {
					return nil, inf.nodeErr(e, "function expects at least %d arguments, got %d", minArgs, len(args))
				}
				if maxArgs != -1 && len(args) > maxArgs {
					return nil, inf.nodeErr(e, "function expects at most %d arguments, got %d", maxArgs, len(args))
				}
				for i, argExpr := range args {
					argT, err := inf.inferExpr(argExpr)
					if err != nil {
						return nil, err
					}
					if i < len(argTuple.Elems) {
						expectedParamT := argTuple.Elems[i]
						if !IsAssignableTo(argT, expectedParamT) {
							return nil, inf.nodeErr(argExpr, "type mismatch in argument %d: expected %s, got %s", i+1, expectedParamT, argT)
						}
					} else if argTuple.IsVariadic && len(argTuple.Elems) > 0 {
						expectedParamT := argTuple.Elems[len(argTuple.Elems)-1]
						if !IsAssignableTo(argT, expectedParamT) {
							return nil, inf.nodeErr(argExpr, "type mismatch in variadic argument %d: expected %s, got %s", i+1, expectedParamT, argT)
						}
					}
				}
			} else if ut, ok := fnT.Arg.(*UnionType); ok {
				argTypes := make([]Type, len(args))
				for i, argExpr := range args {
					argT, err := inf.inferExpr(argExpr)
					if err != nil {
						return nil, err
					}
					argTypes[i] = argT
				}
				matched := false
				var lastMismatch error
				for _, cand := range ut.Members {
					if optTuple, ok := cand.(*TupleType); ok {
						if !optTuple.IsVariadic && len(args) != len(optTuple.Elems) {
							continue
						}
						if optTuple.IsVariadic && len(optTuple.Elems) > 0 && len(args) < len(optTuple.Elems)-1 {
							continue
						}
						typesMatch := true
						for i, argT := range argTypes {
							var expectedParamT Type
							if i < len(optTuple.Elems) {
								expectedParamT = optTuple.Elems[i]
							} else if optTuple.IsVariadic && len(optTuple.Elems) > 0 {
								expectedParamT = optTuple.Elems[len(optTuple.Elems)-1]
							}
							if expectedParamT != nil && !IsAssignableTo(argT, expectedParamT) {
								typesMatch = false
								lastMismatch = inf.nodeErr(args[i], "type mismatch in argument %d: expected %s, got %s", i+1, expectedParamT, argT)
								break
							}
						}
						if typesMatch {
							matched = true
							break
						}
					}
				}
				if !matched {
					if lastMismatch != nil {
						return nil, lastMismatch
					}
					return nil, inf.nodeErr(e, "no matching signature for function call with %d arguments", len(args))
				}
			} else {
				for _, argExpr := range args {
					if _, err := inf.inferExpr(argExpr); err != nil {
						return nil, err
					}
				}
			}
			return fnT.Ret, nil
		}
		for _, argExpr := range e.Args {
			if _, err := inf.inferExpr(argExpr); err != nil {
				return nil, err
			}
		}
		if e.Block != nil {
			if _, err := inf.inferExpr(e.Block); err != nil {
				return nil, err
			}
		}
		return TypeUnknown, nil
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
			return nil, inf.nodeErr(e, "interface %s has no method '%s'", it.Name, e.Member)
		}
		if targetT == TypeUnknown {
			return TypeUnknown, nil
		}
		if bSig, ok := LookupBuiltinTypeMethod(targetT, e.Member); ok {
			return bSig, nil
		}
		return nil, inf.nodeErr(e, "cannot access member '%s' on non-struct/interface type %s", e.Member, targetT)
	case *ast.NamedArgExpr:
		return inf.inferExpr(e.Value)
	case *ast.IndexExpr:
		targetT, err := inf.inferExpr(e.Target)
		if err != nil {
			return nil, err
		}
		if st, ok := targetT.(*StructType); ok {
			if m, err := st.LookupMember("[]"); err == nil {
				if fnT, ok := m.(*FuncType); ok {
					return fnT.Ret, nil
				}
			}
		}
		if it, ok := targetT.(*InterfaceType); ok {
			if m, ok := it.LookupMethod("[]"); ok {
				return m.Ret, nil
			}
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
		return TypeUnknown, nil
	case *ast.ModuleAccessExpr:
		if mod, ok := inf.modules[e.Module]; ok {
			if t, ok := mod.Exports[e.Member]; ok {
				return t, nil
			}
			return nil, inf.nodeErr(e, "undefined member '%s' in module '%s'", e.Member, e.Module)
		}
		return TypeUnknown, nil
	case *ast.IsExpr:
		_, err := inf.inferExpr(e.Expr)
		if err != nil {
			return nil, err
		}
		if e.TypeAnnot != "" {
			if _, err := inf.parseTypeAnnot(e.TypeAnnot); err != nil {
				return nil, err
			}
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
						_ = caseScope.Declare(name, TypeUnknown)
					}
				}
				if c.TuplePattern.RestName != "" && c.TuplePattern.RestName != "_" {
					_ = caseScope.Declare(c.TuplePattern.RestName, &TupleType{Elems: []Type{TypeUnknown}, IsVariadic: true})
				}
			} else {
				pat := strings.TrimSpace(c.Pattern)
				if pat != "" && pat != "_" && pat != "true" && pat != "false" {
					if _, err := inf.parseTypeAnnot(pat); err != nil {
						return nil, err
					}
				}
			}

			prevScope := inf.scope
			inf.scope = caseScope

			var lastT Type = TypeUnknown
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
			return TypeUnknown, nil
		}
		return NewUnionType(caseTypes...), nil
	case *ast.LambdaExpr:
		paramTypes := make([]Type, len(e.Params))
		callerParamTypes := make([]Type, len(e.Params))
		isVariadic := false
		minArity := 0
		for i, p := range e.Params {
			var elemType Type = TypeUnknown
			if p.TypeAnnot != "" {
				t, err := inf.parseTypeAnnot(p.TypeAnnot)
				if err != nil {
					return nil, err
				}
				elemType = t
			}
			if p.DefaultValue != nil {
				defT, err := inf.inferExpr(p.DefaultValue)
				if err != nil {
					return nil, err
				}
				if elemType != TypeUnknown && !IsAssignableTo(defT, elemType) {
					return nil, inf.nodeErr(p.DefaultValue, "type mismatch in default value for parameter '%s': expected %s, got %s", p.Name, elemType, defT)
				}
			} else if !p.IsVariadic {
				minArity++
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
		maxArity := len(e.Params)
		if isVariadic {
			maxArity = -1
		}

		childScope := NewScope(inf.scope)
		for i, p := range e.Params {
			_ = childScope.Declare(p.Name, paramTypes[i])
		}

		prevScope := inf.scope
		inf.scope = childScope
		var lastT Type = TypeUnknown
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
			Arg: &TupleType{
				Elems:      callerParamTypes,
				IsVariadic: isVariadic,
			},
			MinArity: minArity,
			MaxArity: maxArity,
			Ret:      lastT,
		}, nil
	case *ast.BlockLiteral:
		return inf.inferExpr(&ast.LambdaExpr{
			Params: e.Params,
			Body:   e.Body,
		})

	default:
		return TypeUnknown, nil
	}
}

func ParseTypeAnnot(annot string) (Type, error) {
	return (*Inferencer)(nil).parseTypeAnnot(annot)
}

func (inf *Inferencer) ParseTypeAnnot(annot string) (Type, error) {
	return inf.parseTypeAnnot(annot)
}

func parseTypeAnnot(annot string) (Type, error) {
	return (*Inferencer)(nil).parseTypeAnnot(annot)
}

func (inf *Inferencer) parseTypeAnnot(annot string) (Type, error) {
	annot = strings.TrimSpace(annot)
	if annot == "" {
		return TypeUnknown, nil
	}

	// 1. Check top-level Union: A | B
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
			pt := strings.TrimSpace(p)
			if pt == "" {
				return nil, fmt.Errorf("invalid union type: empty member in '%s'", annot)
			}
			mt, err := inf.parseTypeAnnot(pt)
			if err != nil {
				return nil, err
			}
			memberTypes = append(memberTypes, mt)
		}
		return NewUnionType(memberTypes...), nil
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
				elems, err := inf.parseTupleElements(inner)
				if err != nil {
					return nil, err
				}
				argType = &TupleType{Elems: elems}
			}
		} else if argStr == "" {
			argType = &TupleType{Elems: []Type{}}
		} else {
			elemType, err := inf.parseTypeAnnot(argStr)
			if err != nil {
				return nil, err
			}
			argType = &TupleType{Elems: []Type{elemType}}
		}

		retType, err := inf.parseTypeAnnot(retStr)
		if err != nil {
			return nil, err
		}
		return &FuncType{
			Arg: argType,
			Ret: retType,
		}, nil
	}

	switch annot {
	case "Int":
		return TypeInt, nil
	case "Float":
		return TypeFloat, nil
	case "String":
		return TypeString, nil
	case "Bytes":
		return TypeBytes, nil
	case "Bool":
		return TypeBool, nil
	case "Nil", "nil":
		return TypeNil, nil
	case "Tuple":

		return &TupleType{Elems: []Type{TypeUnknown}, IsVariadic: true}, nil
	case "Dict":
		return &DictType{Key: TypeUnknown, Value: TypeUnknown}, nil
	}

	// Tuple annotation or grouping: (Int, ...) or (Int, String) or ((Int) -> String)
	if strings.HasPrefix(annot, "(") && strings.HasSuffix(annot, ")") {
		inner := strings.TrimSpace(annot[1 : len(annot)-1])
		if inner == "" {
			return &TupleType{Elems: []Type{}}, nil
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

		elems, err := inf.parseTupleElements(inner)
		if err != nil {
			return nil, err
		}
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
		}, nil
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
			if keyStr != "" && valStr != "" {
				keyType, err := inf.parseTypeAnnot(keyStr)
				if err != nil {
					return nil, err
				}
				valType, err := inf.parseTypeAnnot(valStr)
				if err != nil {
					return nil, err
				}
				return &DictType{Key: keyType, Value: valType}, nil
			}
		}
		return nil, fmt.Errorf("unknown type '%s'", annot)
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
							return t, nil
						}
					}
					if t, ok := mod.Exports[typeName]; ok {
						return t, nil
					}
				}
			}
		}
		if inf.scope != nil {
			if t, ok := inf.scope.Lookup(annot); ok {
				return t, nil
			}
		}
	}

	return nil, fmt.Errorf("unknown type '%s'", annot)
}

func (inf *Inferencer) parseTupleElements(inner string) ([]Type, error) {
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
			elem, err := inf.parseTypeAnnot(trimmed)
			if err != nil {
				return nil, err
			}
			elems = append(elems, elem)
		}
	}
	return elems, nil
}
