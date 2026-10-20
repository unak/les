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

func (s *Scope) Clone() *Scope {
	c := &Scope{
		parent:   s.parent,
		bindings: make(map[string]Type, len(s.bindings)),
		declared: make(map[string]bool, len(s.declared)),
	}
	for k, v := range s.bindings {
		c.bindings[k] = v
	}
	for k, v := range s.declared {
		c.declared[k] = v
	}
	return c
}

// TypeWarning represents a warning emitted during type inference.
type TypeWarning struct {
	Filename string
	Pos      ast.Position
	Message  string
}

func (w TypeWarning) String() string {
	if w.Filename != "" {
		return fmt.Sprintf("%s:%d: %s at col %d", w.Filename, w.Pos.Line, w.Message, w.Pos.Col)
	}
	return fmt.Sprintf("%d: %s at col %d", w.Pos.Line, w.Message, w.Pos.Col)
}

type funcReturnCtx struct {
	fnType      *FuncType
	returnTypes []Type
	selfCalled  bool
}

type breakTargetCtx struct {
	parent     *breakTargetCtx
	isLoop     bool
	breakTypes []Type
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
	warnUnknown  bool
	warnings     []TypeWarning
	funcCtx      *funcReturnCtx
	breakCtx     *breakTargetCtx
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
		nextVarID:   0,
		scope:       fileScope,
		modules:     make(map[string]*ModuleType),
		ExprTypes:   make(map[ast.Expr]Type),
		StmtTypes:   make(map[ast.Stmt]Type),
		warnUnknown: true,
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

func (inf *Inferencer) SetWarnUnknown(warn bool) {
	inf.warnUnknown = warn
}

func (inf *Inferencer) Warnings() []TypeWarning {
	return inf.warnings
}

func (inf *Inferencer) addWarning(pos ast.Position, format string, args ...any) {
	if !inf.warnUnknown {
		return
	}
	msg := fmt.Sprintf(format, args...)
	inf.warnings = append(inf.warnings, TypeWarning{
		Filename: inf.filename,
		Pos:      pos,
		Message:  msg,
	})
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

func isBareCallableAutoInvocation(expr ast.Expr, t Type) (*FuncType, bool) {
	if fnT, ok := t.(*FuncType); ok {
		switch expr.(type) {
		case *ast.Identifier, *ast.ModuleAccessExpr:
			return fnT, true
		}
	}
	return nil, false
}

func isExitCall(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Identifier:
		return e.Name == "exit"
	case *ast.CallExpr:
		if id, ok := e.Callee.(*ast.Identifier); ok && id.Name == "exit" {
			return true
		}
	}
	return false
}

func (inf *Inferencer) inferBlock(stmts []ast.Stmt) (fallthroughTypes []Type, canFallthrough bool, err error) {
	emptyTuple := &TupleType{Elems: []Type{}}
	if len(stmts) == 0 {
		return []Type{emptyTuple}, true, nil
	}

	canFallthrough = true
	currentValTypes := []Type{emptyTuple}

	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			var retT Type = emptyTuple
			if s.Value != nil {
				t, err := inf.inferExpr(s.Value)
				if err != nil {
					return nil, false, err
				}
				retT = t
			}
			if inf.funcCtx != nil {
				inf.funcCtx.returnTypes = append(inf.funcCtx.returnTypes, retT)
			}
			canFallthrough = false
			currentValTypes = nil

		case *ast.BreakStmt:
			var brkT Type = emptyTuple
			if s.Value != nil {
				t, err := inf.inferExpr(s.Value)
				if err != nil {
					return nil, false, err
				}
				brkT = t
			}
			if inf.breakCtx != nil {
				inf.breakCtx.breakTypes = append(inf.breakCtx.breakTypes, brkT)
			}
			canFallthrough = false
			currentValTypes = nil

		case *ast.ContinueStmt:
			if inf.breakCtx != nil && !inf.breakCtx.isLoop {
				inf.breakCtx.breakTypes = append(inf.breakCtx.breakTypes, emptyTuple)
			}
			canFallthrough = false
			currentValTypes = nil

		case *ast.ExprStmt:
			if me, isMatch := s.Value.(*ast.MatchExpr); isMatch {
				met, matchCanFT, err := inf.inferMatchExpr(me)
				if err != nil {
					return nil, false, err
				}
				if inf.ExprTypes != nil {
					inf.ExprTypes[me] = met
				}
				if !matchCanFT {
					canFallthrough = false
					currentValTypes = nil
				} else if canFallthrough {
					currentValTypes = []Type{met}
				}
				continue
			}

			et, err := inf.inferExpr(s.Value)
			if err != nil {
				return nil, false, err
			}
			if isExitCall(s.Value) {
				canFallthrough = false
				currentValTypes = nil
			} else if fnT, ok := isBareCallableAutoInvocation(s.Value, et); ok {
				if inf.funcCtx != nil && inf.funcCtx.fnType == fnT {
					inf.funcCtx.selfCalled = true
				}
				if canFallthrough {
					currentValTypes = []Type{fnT.Ret}
				}
			} else {
				if canFallthrough {
					currentValTypes = []Type{et}
				}
			}

		case *ast.IfStmt:
			if s.Cond != nil {
				if _, err := inf.inferExpr(s.Cond); err != nil {
					return nil, false, err
				}
			}
			thenScope := NewScope(inf.scope)
			prevScope := inf.scope
			inf.scope = thenScope
			thenFT, thenCanFT, err := inf.inferBlock(s.ThenBody)
			inf.scope = prevScope
			if err != nil {
				return nil, false, err
			}

			if len(s.ElseBody) > 0 {
				elseScope := NewScope(inf.scope)
				inf.scope = elseScope
				elseFT, elseCanFT, err := inf.inferBlock(s.ElseBody)
				inf.scope = prevScope
				if err != nil {
					return nil, false, err
				}

				if canFallthrough {
					if !thenCanFT && !elseCanFT {
						canFallthrough = false
						currentValTypes = nil
					} else {
						var branchTypes []Type
						if thenCanFT {
							branchTypes = append(branchTypes, thenFT...)
						}
						if elseCanFT {
							branchTypes = append(branchTypes, elseFT...)
						}
						canFallthrough = true
						currentValTypes = branchTypes
					}
				}
			} else {
				// No else body: when condition is false at runtime, evalStmt returns nil, nil,
				// so lastVal remains currentValTypes if canFallthrough was true.
				if canFallthrough && thenCanFT {
					currentValTypes = append(currentValTypes, thenFT...)
				}
			}

		case *ast.ForStmt:
			if s.Cond != nil {
				if _, err := inf.inferExpr(s.Cond); err != nil {
					return nil, false, err
				}
			}
			loopCtx := &breakTargetCtx{
				parent: inf.breakCtx,
				isLoop: true,
			}
			prevBreakCtx := inf.breakCtx
			inf.breakCtx = loopCtx
			_, _, err := inf.inferBlock(s.Body)
			inf.breakCtx = prevBreakCtx
			if err != nil {
				return nil, false, err
			}

			if canFallthrough {
				if s.Cond == nil {
					// Infinite for: loop: only exits if a break occurred
					if len(loopCtx.breakTypes) == 0 {
						canFallthrough = false
						currentValTypes = nil
					} else {
						canFallthrough = true
						currentValTypes = loopCtx.breakTypes
					}
				} else {
					// Conditional for loop: exits normally with () or with break
					canFallthrough = true
					currentValTypes = append([]Type{emptyTuple}, loopCtx.breakTypes...)
				}
			}

		default:
			if err := inf.inferStmt(stmt); err != nil {
				return nil, false, err
			}
			// Declarations, assignments, imports, etc. do not update lastVal in evalBlock
		}
	}

	return currentValTypes, canFallthrough, nil
}

func (inf *Inferencer) inferFuncBody(body []ast.Stmt, fnType *FuncType, declaredRetType Type, pos ast.Position, entityName string, isMethod bool, structName string) error {
	savedWarnIdx := len(inf.warnings)

	funcCtx := &funcReturnCtx{
		fnType: fnType,
	}
	prevFuncCtx := inf.funcCtx
	inf.funcCtx = funcCtx
	prevBreakCtx := inf.breakCtx
	inf.breakCtx = nil
	defer func() {
		inf.funcCtx = prevFuncCtx
		inf.breakCtx = prevBreakCtx
	}()

	cleanScope := inf.scope.Clone()

	ftTypes, canFT, err := inf.inferBlock(body)
	if err != nil {
		return err
	}

	// 2-pass resolution for self-recursion when declaredRetType is unspecified
	if declaredRetType == TypeUnknown && funcCtx.selfCalled {
		var candidateTypes []Type
		for _, t := range funcCtx.returnTypes {
			if t != TypeUnknown {
				candidateTypes = append(candidateTypes, t)
			}
		}
		if canFT {
			for _, t := range ftTypes {
				if t != TypeUnknown {
					candidateTypes = append(candidateTypes, t)
				}
			}
		}

		// Prefer completely known types (not containing any nested Unknown)
		var knownTypes []Type
		for _, t := range candidateTypes {
			if !containsUnknown(t) {
				knownTypes = append(knownTypes, t)
			}
		}
		if len(knownTypes) == 0 {
			knownTypes = candidateTypes
		}

		if len(knownTypes) > 0 {
			tentativeRet := NewUnionType(knownTypes...)
			fnType.Ret = tentativeRet
			inf.warnings = inf.warnings[:savedWarnIdx]

			funcCtx.returnTypes = nil
			funcCtx.selfCalled = false
			inf.scope = cleanScope
			ftTypes, canFT, err = inf.inferBlock(body)
			if err != nil {
				return err
			}
		}
	}

	var allCollected []Type
	allCollected = append(allCollected, funcCtx.returnTypes...)
	if canFT {
		allCollected = append(allCollected, ftTypes...)
	}

	var unifiedType Type = TypeUnknown
	if len(allCollected) > 0 {
		unifiedType = NewUnionType(allCollected...)
	} else if canFT {
		unifiedType = &TupleType{Elems: []Type{}}
	}

	if declaredRetType != TypeUnknown {
		if !IsAssignableTo(unifiedType, declaredRetType) {
			if isMethod {
				return inf.posErr(pos, "return type mismatch in method '%s' of struct '%s': expected %s, got %s", entityName, structName, declaredRetType, unifiedType)
			}
			return inf.posErr(pos, "return type mismatch in function '%s': expected %s, got %s", entityName, declaredRetType, unifiedType)
		}
	} else if unifiedType != TypeUnknown {
		fnType.Ret = unifiedType
	} else {
		if isMethod {
			inf.addWarning(pos, "Unknown type of return value in method '%s' of struct '%s'", entityName, structName)
		} else {
			inf.addWarning(pos, "Unknown type of return value in function '%s'", entityName)
		}
	}

	return nil
}

func (inf *Inferencer) posErr(pos ast.Position, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if strings.Contains(msg, " at col ") {
		return fmt.Errorf("%s", msg)
	}
	fn := inf.filename
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
		} else if targetType == TypeUnknown && !isEmptyCollection(s.Value) {
			inf.addWarning(s.Pos(), "Unknown type of variable '%s'", s.Name)
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
		var origScope *Scope
		var typeParamDefs []TypeParamDef
		if len(s.TypeParams) > 0 {
			origScope = inf.scope
			tpScope := NewScope(inf.scope)
			tpTypes := make([]*TypeParamType, len(s.TypeParams))
			for i, tp := range s.TypeParams {
				tpType := &TypeParamType{Name: tp.Name}
				tpTypes[i] = tpType
				_ = tpScope.Declare(tp.Name, tpType)
			}
			inf.scope = tpScope
			typeParamDefs = make([]TypeParamDef, len(s.TypeParams))
			for i, tp := range s.TypeParams {
				if tp.Constraint != "" {
					cType, err := inf.parseTypeAnnot(tp.Constraint)
					if err != nil {
						inf.scope = origScope
						return err
					}
					tpTypes[i].Constraint = cType
				}
				typeParamDefs[i] = TypeParamDef{Name: tp.Name, Constraint: tpTypes[i].Constraint}
			}
		}

		paramTypes := make([]Type, len(s.Params))
		callerParamTypes := make([]Type, len(s.Params))
		isVariadic := false
		minArity := 0
		for i, p := range s.Params {
			var elemType Type = TypeUnknown
			if p.TypeAnnot != "" {
				t, err := inf.parseTypeAnnot(p.TypeAnnot)
				if err != nil {
					if origScope != nil {
						inf.scope = origScope
					}
					return err
				}
				elemType = t
			} else {
				inf.addWarning(p.Pos(), "Unknown type of parameter '%s' in function '%s'", p.Name, s.Name)
			}
			if p.DefaultValue != nil {
				defT, err := inf.inferExpr(p.DefaultValue)
				if err != nil {
					if origScope != nil {
						inf.scope = origScope
					}
					return err
				}
				if elemType != TypeUnknown && !IsAssignableTo(defT, elemType) {
					if origScope != nil {
						inf.scope = origScope
					}
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
					if origScope != nil {
						inf.scope = origScope
					}
					return err
				}
				blockT = t
			} else {
				inf.addWarning(s.BlockParam.Pos(), "Unknown type of block parameter '&%s' in function '%s'", s.BlockParam.Name, s.Name)
			}
			blockOpt = (s.BlockParam.DefaultValue != nil)
			if s.BlockParam.DefaultValue != nil {
				defT, err := inf.inferExpr(s.BlockParam.DefaultValue)
				if err != nil {
					if origScope != nil {
						inf.scope = origScope
					}
					return err
				}
				if blockT != TypeUnknown && !IsAssignableTo(defT, blockT) {
					if origScope != nil {
						inf.scope = origScope
					}
					return inf.nodeErr(s.BlockParam.DefaultValue, "type mismatch in default value for block parameter '&%s': expected %s, got %s", s.BlockParam.Name, blockT, defT)
				}
			}
		}

		var declaredRetType Type = TypeUnknown
		if s.ReturnType != "" {
			t, err := inf.parseTypeAnnot(s.ReturnType)
			if err != nil {
				if origScope != nil {
					inf.scope = origScope
				}
				return err
			}
			declaredRetType = t
		}

		fnType := &FuncType{
			TypeParams: typeParamDefs,
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
		// Function declaration in enclosing scope
		declScope := inf.scope
		if origScope != nil {
			declScope = origScope
		}
		if err := declScope.Declare(s.Name, fnType); err != nil {
			if origScope != nil {
				inf.scope = origScope
			}
			return inf.nodeErr(s, "%s", err.Error())
		}

		// Create child scope for function parameters and body (shadowing allowed)
		childScope := NewScope(inf.scope)
		for i, p := range s.Params {
			if err := childScope.Declare(p.Name, paramTypes[i]); err != nil {
				if origScope != nil {
					inf.scope = origScope
				}
				return inf.nodeErr(s, "duplicate parameter '%s' in function '%s'", p.Name, s.Name)
			}
		}
		if s.BlockParam != nil {
			if err := childScope.Declare(s.BlockParam.Name, blockT); err != nil {
				if origScope != nil {
					inf.scope = origScope
				}
				return inf.nodeErr(s, "duplicate parameter '&%s' in function '%s'", s.BlockParam.Name, s.Name)
			}
		}

		prevScope := inf.scope
		inf.scope = childScope
		err := inf.inferFuncBody(s.Body, fnType, declaredRetType, s.Pos(), s.Name, false, "")
		inf.scope = prevScope
		if origScope != nil {
			inf.scope = origScope
		}
		if err != nil {
			return err
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
		effTargetT := targetT
		if tp, ok := targetT.(*TypeParamType); ok && tp.Constraint != nil {
			effTargetT = tp.Constraint
		}
		st, ok := effTargetT.(*StructType)
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
		var origScope *Scope
		var typeParamDefs []TypeParamDef
		if len(s.TypeParams) > 0 {
			origScope = inf.scope
			tpScope := NewScope(inf.scope)
			tpTypes := make([]*TypeParamType, len(s.TypeParams))
			for i, tp := range s.TypeParams {
				tpType := &TypeParamType{Name: tp.Name}
				tpTypes[i] = tpType
				_ = tpScope.Declare(tp.Name, tpType)
			}
			inf.scope = tpScope
			typeParamDefs = make([]TypeParamDef, len(s.TypeParams))
			for i, tp := range s.TypeParams {
				if tp.Constraint != "" {
					cType, err := inf.parseTypeAnnot(tp.Constraint)
					if err != nil {
						inf.scope = origScope
						return err
					}
					tpTypes[i].Constraint = cType
				}
				typeParamDefs[i] = TypeParamDef{Name: tp.Name, Constraint: tpTypes[i].Constraint}
			}
		}

		st := &StructType{
			Name:       s.Name,
			TypeParams: typeParamDefs,
			Fields:     make(map[string]Type),
			FieldOrder: make([]string, 0),
			Embedded:   make([]*StructType, 0),
			Methods:    make(map[string]*FuncType),
		}
		if inf.StmtTypes != nil {
			inf.StmtTypes[s] = st
		}

		// Declare struct type in enclosing scope so methods/self can reference it
		declScope := inf.scope
		if origScope != nil {
			declScope = origScope
		}
		if err := declScope.Declare(s.Name, st); err != nil {
			if origScope != nil {
				inf.scope = origScope
			}
			return inf.nodeErr(s, "%s", err.Error())
		}

		// Process fields
		for _, f := range s.Fields {
			if f.IsEmbedded {
				embType, err := inf.parseTypeAnnot(f.TypeAnnot)
				if err != nil {
					if origScope != nil {
						inf.scope = origScope
					}
					return inf.nodeErr(s, "undefined embedded struct '%s' in struct '%s'", f.TypeAnnot, s.Name)
				}
				embStruct, ok := embType.(*StructType)
				if !ok {
					if origScope != nil {
						inf.scope = origScope
					}
					return inf.nodeErr(s, "cannot embed non-struct type '%s' in struct '%s'", f.TypeAnnot, s.Name)
				}
				st.Embedded = append(st.Embedded, embStruct)
				st.Fields[f.Name] = embStruct
				st.FieldOrder = append(st.FieldOrder, f.Name)
			} else {
				fieldType, err := inf.parseTypeAnnot(f.TypeAnnot)
				if err != nil {
					if origScope != nil {
						inf.scope = origScope
					}
					return err
				}
				st.Fields[f.Name] = fieldType
				st.FieldOrder = append(st.FieldOrder, f.Name)
			}
		}

		// Process methods in two passes:
		// Pass A: Build and register method signatures on st.Methods so self-calls and forward calls work
		type methodWorkItem struct {
			methodDef        *ast.FuncDef
			methodType       *FuncType
			methodScope      *Scope
			declaredRetType  Type
		}
		var methodItems []*methodWorkItem

		for _, m := range s.Methods {
			methodScope := NewScope(inf.scope)
			_ = methodScope.Declare("self", st)
			for fName, fType := range st.Fields {
				_ = methodScope.Declare(fName, fType)
			}

			var mTypeParams []TypeParamDef
			prevInfScope := inf.scope
			if len(m.TypeParams) > 0 {
				mScope := NewScope(methodScope)
				tpTypes := make([]*TypeParamType, len(m.TypeParams))
				for i, tp := range m.TypeParams {
					tpTypes[i] = &TypeParamType{Name: tp.Name}
					_ = mScope.Declare(tp.Name, tpTypes[i])
				}
				inf.scope = mScope
				mTypeParams = make([]TypeParamDef, len(m.TypeParams))
				for i, tp := range m.TypeParams {
					if tp.Constraint != "" {
						cType, err := inf.parseTypeAnnot(tp.Constraint)
						if err != nil {
							inf.scope = prevInfScope
							if origScope != nil {
								inf.scope = origScope
							}
							return err
						}
						tpTypes[i].Constraint = cType
					}
					mTypeParams[i] = TypeParamDef{Name: tp.Name, Constraint: tpTypes[i].Constraint}
				}
				methodScope = mScope
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
						if len(m.TypeParams) > 0 {
							inf.scope = prevInfScope
						}
						if origScope != nil {
							inf.scope = origScope
						}
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
				if pType == TypeUnknown {
					inf.addWarning(p.Pos(), "Unknown type of parameter '%s' in method '%s' of struct '%s'", p.Name, m.Name, s.Name)
				}

				if p.DefaultValue != nil {
					defT, err := inf.inferExpr(p.DefaultValue)
					if err != nil {
						if len(m.TypeParams) > 0 {
							inf.scope = prevInfScope
						}
						if origScope != nil {
							inf.scope = origScope
						}
						return err
					}
					if pType != TypeUnknown && !IsAssignableTo(defT, pType) {
						if len(m.TypeParams) > 0 {
							inf.scope = prevInfScope
						}
						if origScope != nil {
							inf.scope = origScope
						}
						return inf.nodeErr(p.DefaultValue, "type mismatch in default value for parameter '%s': expected %s, got %s", p.Name, pType, defT)
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
						if len(m.TypeParams) > 0 {
							inf.scope = prevInfScope
						}
						if origScope != nil {
							inf.scope = origScope
						}
						return err
					}
					blockT = t
				} else {
					inf.addWarning(m.BlockParam.Pos(), "Unknown type of block parameter '&%s' in method '%s' of struct '%s'", m.BlockParam.Name, m.Name, s.Name)
				}
				blockOpt = (m.BlockParam.DefaultValue != nil)
				if m.BlockParam.DefaultValue != nil {
					defT, err := inf.inferExpr(m.BlockParam.DefaultValue)
					if err != nil {
						if len(m.TypeParams) > 0 {
							inf.scope = prevInfScope
						}
						if origScope != nil {
							inf.scope = origScope
						}
						return err
					}
					if blockT != TypeUnknown && !IsAssignableTo(defT, blockT) {
						if len(m.TypeParams) > 0 {
							inf.scope = prevInfScope
						}
						if origScope != nil {
							inf.scope = origScope
						}
						return inf.nodeErr(m.BlockParam.DefaultValue, "type mismatch in default value for block parameter '&%s': expected %s, got %s", m.BlockParam.Name, blockT, defT)
					}
				}
				_ = methodScope.Declare(m.BlockParam.Name, blockT)
			}

			var declaredRetType Type = TypeUnknown
			if m.ReturnType != "" {
				t, err := inf.parseTypeAnnot(m.ReturnType)
				if err != nil {
					if len(m.TypeParams) > 0 {
						inf.scope = prevInfScope
					}
					if origScope != nil {
						inf.scope = origScope
					}
					return inf.nodeErr(m, "%s", err.Error())
				}
				declaredRetType = t
			} else if m.Name == "init" {
				declaredRetType = st
			}

			if len(m.TypeParams) > 0 {
				inf.scope = prevInfScope
			}

			mType := &FuncType{
				TypeParams: mTypeParams,
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

			methodItems = append(methodItems, &methodWorkItem{
				methodDef:       m,
				methodType:      mType,
				methodScope:     methodScope,
				declaredRetType: declaredRetType,
			})

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

		// Pass B: Infer method bodies using inferFuncBody
		for _, item := range methodItems {
			m := item.methodDef
			prevScope := inf.scope
			inf.scope = item.methodScope
			checkDeclaredRet := item.declaredRetType
			if m.Name == "init" {
				checkDeclaredRet = TypeUnknown
			}
			err := inf.inferFuncBody(m.Body, item.methodType, checkDeclaredRet, m.Pos(), m.Name, true, s.Name)
			inf.scope = prevScope
			if err != nil {
				if origScope != nil {
					inf.scope = origScope
				}
				return err
			}
			if m.Name == "init" {
				item.methodType.Ret = st
			}
		}

		// Rebuild overloaded operator method types (+ / -) now that method bodies have inferred Ret
		for _, opName := range []string{"+", "-"} {
			if existing, ok := st.Methods[opName]; ok && len(existing.Overloads) == 2 {
				unaryFT := existing.Overloads[0]
				binaryFT := existing.Overloads[1]
				ret := binaryFT.Ret
				if !unaryFT.Ret.Equals(binaryFT.Ret) {
					ret = NewUnionType(unaryFT.Ret, binaryFT.Ret)
				}
				existing.Ret = ret
				existing.OverloadUnion = fmt.Sprintf("%s | %s", unaryFT.String(), binaryFT.String())
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

		st.ClearInstCache()
		if origScope != nil {
			inf.scope = origScope
		}
		return nil

	case *ast.InterfaceDef:
		var origScope *Scope
		var typeParamDefs []TypeParamDef
		if len(s.TypeParams) > 0 {
			origScope = inf.scope
			tpScope := NewScope(inf.scope)
			tpTypes := make([]*TypeParamType, len(s.TypeParams))
			for i, tp := range s.TypeParams {
				tpType := &TypeParamType{Name: tp.Name}
				tpTypes[i] = tpType
				_ = tpScope.Declare(tp.Name, tpType)
			}
			inf.scope = tpScope
			typeParamDefs = make([]TypeParamDef, len(s.TypeParams))
			for i, tp := range s.TypeParams {
				if tp.Constraint != "" {
					cType, err := inf.parseTypeAnnot(tp.Constraint)
					if err != nil {
						inf.scope = origScope
						return err
					}
					tpTypes[i].Constraint = cType
				}
				typeParamDefs[i] = TypeParamDef{Name: tp.Name, Constraint: tpTypes[i].Constraint}
			}
		}

		iface := &InterfaceType{
			Name:       s.Name,
			TypeParams: typeParamDefs,
			Methods:    make(map[string]*FuncType),
			Embedded:   make([]*InterfaceType, 0),
		}

		// First, collect methods and check for conflicts from embedded interfaces
		for _, embName := range s.Embedded {
			embType, err := inf.parseTypeAnnot(embName)
			if err != nil {
				if origScope != nil {
					inf.scope = origScope
				}
				return inf.nodeErr(s, "undefined embedded interface '%s' in interface '%s'", embName, s.Name)
			}
			embIface, ok := embType.(*InterfaceType)
			if !ok {
				if origScope != nil {
					inf.scope = origScope
				}
				return inf.nodeErr(s, "cannot embed non-interface type '%s' in interface '%s'", embName, s.Name)
			}
			iface.Embedded = append(iface.Embedded, embIface)

			for mName, mSig := range embIface.Methods {
				if existingSig, exists := iface.Methods[mName]; exists {
					if !existingSig.Equals(mSig) {
						if origScope != nil {
							inf.scope = origScope
						}
						return inf.nodeErr(s, "conflicting method signatures for '%s' in embedded interface '%s' of '%s'", mName, embName, s.Name)
					}
				} else {
					iface.Methods[mName] = mSig
				}
			}
		}

		declScope := inf.scope
		if origScope != nil {
			declScope = origScope
		}
		if err := declScope.Declare(s.Name, iface); err != nil {
			if origScope != nil {
				inf.scope = origScope
			}
			return inf.nodeErr(s, "%s", err.Error())
		}

		// Next, process direct methods on interface
		for _, m := range s.Methods {
			var mTypeParams []TypeParamDef
			prevInfScope := inf.scope
			if len(m.TypeParams) > 0 {
				mScope := NewScope(inf.scope)
				tpTypes := make([]*TypeParamType, len(m.TypeParams))
				for i, tp := range m.TypeParams {
					tpTypes[i] = &TypeParamType{Name: tp.Name}
					_ = mScope.Declare(tp.Name, tpTypes[i])
				}
				inf.scope = mScope
				mTypeParams = make([]TypeParamDef, len(m.TypeParams))
				for i, tp := range m.TypeParams {
					if tp.Constraint != "" {
						cType, err := inf.parseTypeAnnot(tp.Constraint)
						if err != nil {
							inf.scope = prevInfScope
							if origScope != nil {
								inf.scope = origScope
							}
							return err
						}
						tpTypes[i].Constraint = cType
					}
					mTypeParams[i] = TypeParamDef{Name: tp.Name, Constraint: tpTypes[i].Constraint}
				}
			}

			paramTypes := make([]Type, 0, len(m.Params))
			minArity := 0
			isVariadic := false
			for _, p := range m.Params {
				if p.Name == "self" {
					continue
				}
				if p.IsVariadic {
					isVariadic = true
				}
				var pType Type = TypeUnknown
				if p.TypeAnnot != "" {
					t, err := inf.parseTypeAnnot(p.TypeAnnot)
					if err != nil {
						if len(m.TypeParams) > 0 {
							inf.scope = prevInfScope
						}
						if origScope != nil {
							inf.scope = origScope
						}
						return err
					}
					pType = t
				} else {
					inf.addWarning(p.Pos(), "Unknown type of parameter '%s' in method '%s' of interface '%s'", p.Name, m.Name, s.Name)
				}
				paramTypes = append(paramTypes, pType)
				if p.DefaultValue == nil && !p.IsVariadic {
					minArity++
				}
			}
			maxArity := len(paramTypes)
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
						if len(m.TypeParams) > 0 {
							inf.scope = prevInfScope
						}
						if origScope != nil {
							inf.scope = origScope
						}
						return err
					}
					blockT = t
				} else {
					inf.addWarning(m.BlockParam.Pos(), "Unknown type of block parameter '&%s' in method '%s' of interface '%s'", m.BlockParam.Name, m.Name, s.Name)
				}
				blockOpt = (m.BlockParam.DefaultValue != nil)
			}
			var retType Type = TypeUnknown
			if m.ReturnType != "" {
				t, err := inf.parseTypeAnnot(m.ReturnType)
				if err != nil {
					if len(m.TypeParams) > 0 {
						inf.scope = prevInfScope
					}
					if origScope != nil {
						inf.scope = origScope
					}
					return err
				}
				retType = t
			} else {
				inf.addWarning(m.Pos(), "Unknown type of return value in method '%s' of interface '%s'", m.Name, s.Name)
			}

			if len(m.TypeParams) > 0 {
				inf.scope = prevInfScope
			}

			mSig := &FuncType{
				TypeParams: mTypeParams,
				Arg:        &TupleType{Elems: paramTypes, IsVariadic: isVariadic},
				MinArity:   minArity,
				MaxArity:   maxArity,
				BlockParam: blockT,
				BlockOpt:   blockOpt,
				Ret:        retType,
			}

			if existingSig, exists := iface.Methods[m.Name]; exists {
				if !existingSig.Equals(mSig) {
					if origScope != nil {
						inf.scope = origScope
					}
					return inf.nodeErr(s, "method '%s' in interface '%s' conflicts with embedded method signature", m.Name, s.Name)
				}
			}
			iface.Methods[m.Name] = mSig
		}

		iface.ClearInstCache()
		if origScope != nil {
			inf.scope = origScope
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
		elems := make([]Type, 0, len(e.Elements))
		isVariadic := false
		for _, el := range e.Elements {
			if isVariadic {
				return nil, inf.nodeErr(el, "elements cannot follow a spread infinite/variadic tuple")
			}
			if sp, ok := el.(*ast.SpreadExpr); ok {
				spT, err := inf.inferExpr(sp.Expr)
				if err != nil {
					return nil, err
				}
				if spT == TypeUnknown {
					isVariadic = true
					elems = append(elems, TypeUnknown)
					continue
				}
				spTuple, ok := spT.(*TupleType)
				if !ok {
					return nil, inf.nodeErr(sp.Expr, "cannot spread non-tuple value of type %s", spT)
				}
				if spTuple.IsVariadic {
					isVariadic = true
				}
				elems = append(elems, spTuple.Elems...)
			} else {
				if isVariadic {
					return nil, inf.nodeErr(el, "elements cannot follow a spread infinite/variadic tuple")
				}
				t, err := inf.inferExpr(el)
				if err != nil {
					return nil, err
				}
				elems = append(elems, t)
			}
		}
		return &TupleType{Elems: elems, IsVariadic: isVariadic}, nil
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
		effT := t
		if tp, ok := t.(*TypeParamType); ok && tp.Constraint != nil {
			effT = tp.Constraint
		}
		if st, ok := effT.(*StructType); ok {
			if m, err := st.LookupMember(e.Op); err == nil {
				if fnT, ok := m.(*FuncType); ok {
					if len(fnT.Overloads) > 0 {
						for _, ov := range fnT.Overloads {
							if ov.MinArity == 0 && ov.MaxArity == 0 {
								return ov.Ret, nil
							}
						}
						return nil, inf.nodeErr(e, "operator %s requires 0 arguments", e.Op)
					}
					if fnT.MinArity > 0 {
						return nil, inf.nodeErr(e, "operator %s requires 0 arguments, got %d", e.Op, fnT.MinArity)
					}
					return fnT.Ret, nil
				}
			}
		}
		if it, ok := effT.(*InterfaceType); ok {
			if m, ok := it.LookupMethod(e.Op); ok {
				if m.MinArity > 0 {
					return nil, inf.nodeErr(e, "operator %s requires 0 arguments, got %d", e.Op, m.MinArity)
				}
				return m.Ret, nil
			}
		}
		if tp, ok := t.(*TypeParamType); ok && tp.Constraint != nil {
			if bSig, ok := LookupBuiltinTypeMethod(tp.Constraint, e.Op); ok {
				return bSig.Ret, nil
			}
			switch e.Op {
			case "+", "-":
				if IsAssignableTo(tp.Constraint, NewUnionType(TypeInt, TypeFloat)) {
					return tp, nil
				}
			case "!":
				return TypeBool, nil
			case "~":
				if IsAssignableTo(tp.Constraint, TypeInt) {
					return tp, nil
				}
			}
		}
		switch e.Op {
		case "+", "-":
			if effT == TypeUnknown {
				return TypeUnknown, nil
			}
			if effT == TypeFloat {
				return TypeFloat, nil
			}
			return TypeInt, nil
		case "!":
			return TypeBool, nil
		case "~":
			if effT == TypeUnknown {
				return TypeUnknown, nil
			}
			if effT == TypeInt {
				return TypeInt, nil
			}
			return nil, inf.nodeErr(e, "cannot perform ~ operation on %s", t)
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
						if pT, ok := getFirstParamType(fnT); ok && !IsAssignableTo(rightT, pT) {
							return nil, inf.nodeErr(e.Right, "type mismatch in operator '!=': expected %s, got %s", pT, rightT)
						}
						return fnT.Ret, nil
					}
				}
			}
			if it, ok := leftT.(*InterfaceType); ok {
				if m, ok := it.LookupMethod("=="); ok {
					if pT, ok := getFirstParamType(m); ok && !IsAssignableTo(rightT, pT) {
						return nil, inf.nodeErr(e.Right, "type mismatch in operator '!=': expected %s, got %s", pT, rightT)
					}
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
							if ov.MinArity <= 1 && (ov.MaxArity < 0 || ov.MaxArity >= 1) {
								if pT, ok := getFirstParamType(ov); ok && !IsAssignableTo(rightT, pT) {
									return nil, inf.nodeErr(e.Right, "type mismatch in operator '%s': expected %s, got %s", e.Op, pT, rightT)
								}
								return ov.Ret, nil
							}
						}
						return nil, inf.nodeErr(e, "no matching overload for operator '%s' with 1 argument", e.Op)
					}
					if fnT.MinArity > 1 || (fnT.MaxArity >= 0 && fnT.MaxArity < 1) {
						return nil, inf.nodeErr(e, "operator %s requires 1 argument, but method has arity %d..%d", e.Op, fnT.MinArity, fnT.MaxArity)
					}
					if pT, ok := getFirstParamType(fnT); ok && !IsAssignableTo(rightT, pT) {
						return nil, inf.nodeErr(e.Right, "type mismatch in operator '%s': expected %s, got %s", e.Op, pT, rightT)
					}
					return fnT.Ret, nil
				}
			}
			if e.Op == "<=" || e.Op == ">" || e.Op == ">=" {
				if m, err := st.LookupMember("<"); err == nil {
					if fnT, ok := m.(*FuncType); ok {
						if pT, ok := getFirstParamType(fnT); ok && !IsAssignableTo(rightT, pT) {
							return nil, inf.nodeErr(e.Right, "type mismatch in operator '%s': expected %s, got %s", e.Op, pT, rightT)
						}
						return fnT.Ret, nil
					}
				}
			}
		}
		if it, ok := leftT.(*InterfaceType); ok {
			if m, ok := it.LookupMethod(e.Op); ok {
				if m.MinArity > 1 || (m.MaxArity >= 0 && m.MaxArity < 1) {
					return nil, inf.nodeErr(e, "operator %s requires 1 argument, but interface method has arity %d..%d", e.Op, m.MinArity, m.MaxArity)
				}
				if pT, ok := getFirstParamType(m); ok && !IsAssignableTo(rightT, pT) {
					return nil, inf.nodeErr(e.Right, "type mismatch in operator '%s': expected %s, got %s", e.Op, pT, rightT)
				}
				return m.Ret, nil
			}
			if e.Op == "<=" || e.Op == ">" || e.Op == ">=" {
				if m, ok := it.LookupMethod("<"); ok {
					if pT, ok := getFirstParamType(m); ok && !IsAssignableTo(rightT, pT) {
						return nil, inf.nodeErr(e.Right, "type mismatch in operator '%s': expected %s, got %s", e.Op, pT, rightT)
					}
					return m.Ret, nil
				}
			}
		}
		if tp, ok := leftT.(*TypeParamType); ok && tp.Constraint != nil {
			if it, ok := tp.Constraint.(*InterfaceType); ok {
				if m, ok := it.LookupMethod(e.Op); ok {
					if pT, ok := getFirstParamType(m); ok && !IsAssignableTo(rightT, pT) {
						return nil, inf.nodeErr(e.Right, "type mismatch in operator '%s': expected %s, got %s", e.Op, pT, rightT)
					}
					return m.Ret, nil
				}
			}
			switch e.Op {
			case "==", "!=", "<", "<=", ">", ">=":
				return TypeBool, nil
			case "+", "-", "*", "/", "%":
				if IsAssignableTo(tp.Constraint, NewUnionType(TypeInt, TypeFloat)) {
					if rightT.Equals(tp) {
						return tp, nil
					}
				}
				if e.Op == "+" && (IsAssignableTo(tp.Constraint, TypeString) || IsAssignableTo(tp.Constraint, TypeBytes)) {
					if rightT.Equals(tp) {
						return tp, nil
					}
				}
			case "&", "|", "^", "<<", ">>":
				if IsAssignableTo(tp.Constraint, TypeInt) {
					if rightT.Equals(tp) {
						return tp, nil
					}
				}
			}
			if bSig, ok := LookupBuiltinTypeMethod(tp.Constraint, e.Op); ok {
				if bSig.Ret == TypeBool {
					return TypeBool, nil
				}
				if rightT.Equals(tp) {
					return tp, nil
				}
				return bSig.Ret, nil
			}
		}
		effLeftT := leftT
		if tp, ok := leftT.(*TypeParamType); ok && tp.Constraint != nil {
			effLeftT = tp.Constraint
		}
		effRightT := rightT
		if tp, ok := rightT.(*TypeParamType); ok && tp.Constraint != nil {
			effRightT = tp.Constraint
		}
		switch e.Op {
		case "+":
			if leftTuple, ok := effLeftT.(*TupleType); ok {
				if rightT == TypeUnknown {
					return &TupleType{Elems: append([]Type(nil), leftTuple.Elems...), IsVariadic: true}, nil
				}
				rightTuple, ok := rightT.(*TupleType)
				if !ok {
					return nil, inf.nodeErr(e.Right, "type mismatch in operator '+': expected tuple, got %s", rightT)
				}
				merged := append(append([]Type(nil), leftTuple.Elems...), rightTuple.Elems...)
				return &TupleType{Elems: merged, IsVariadic: leftTuple.IsVariadic || rightTuple.IsVariadic}, nil
			}
			if _, ok := effRightT.(*TupleType); ok {
				return nil, inf.nodeErr(e.Left, "type mismatch in operator '+': expected tuple, got %s", leftT)
			}
			if leftT == TypeUnknown || rightT == TypeUnknown {
				return TypeUnknown, nil
			}
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
			if leftT == TypeUnknown || rightT == TypeUnknown {
				return TypeUnknown, nil
			}
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
		case "<<":
			if leftTuple, ok := effLeftT.(*TupleType); ok {
				if leftTuple.IsVariadic {
					return leftTuple, nil
				}
				newElems := append(append([]Type(nil), leftTuple.Elems...), rightT)
				return &TupleType{Elems: newElems}, nil
			}
			if leftT == TypeUnknown || rightT == TypeUnknown {
				if leftT != TypeUnknown && leftT != TypeInt {
					return nil, inf.nodeErr(e, "cannot perform << operation on %s and %s", leftT, rightT)
				}
				if rightT != TypeUnknown && rightT != TypeInt {
					return nil, inf.nodeErr(e, "cannot perform << operation on %s and %s", leftT, rightT)
				}
				return TypeUnknown, nil
			}
			if leftT == TypeInt && rightT == TypeInt {
				return TypeInt, nil
			}
			return nil, inf.nodeErr(e, "cannot perform << operation on %s and %s", leftT, rightT)
		case "&", "|", "^", ">>":
			if leftT == TypeUnknown || rightT == TypeUnknown {
				if leftT != TypeUnknown && leftT != TypeInt {
					return nil, inf.nodeErr(e, "cannot perform %s operation on %s and %s", e.Op, leftT, rightT)
				}
				if rightT != TypeUnknown && rightT != TypeInt {
					return nil, inf.nodeErr(e, "cannot perform %s operation on %s and %s", e.Op, leftT, rightT)
				}
				return TypeUnknown, nil
			}
			if leftT == TypeInt && rightT == TypeInt {
				return TypeInt, nil
			}
			return nil, inf.nodeErr(e, "cannot perform %s operation on %s and %s", e.Op, leftT, rightT)
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
		if mem, ok := e.Callee.(*ast.MemberAccessExpr); ok {
			targetT, err := inf.inferExpr(mem.Target)
			if err != nil {
				return nil, err
			}
			effTargetT := targetT
			if tp, ok := targetT.(*TypeParamType); ok && tp.Constraint != nil {
				effTargetT = tp.Constraint
			}
			if targetTuple, ok := effTargetT.(*TupleType); ok {
				switch mem.Member {
				case "Append":
					if len(e.Args) != 1 {
						return nil, inf.nodeErr(e, "Append expects 1 argument, got %d", len(e.Args))
					}
					argT, err := inf.inferExpr(e.Args[0])
					if err != nil {
						return nil, err
					}
					if targetTuple.IsVariadic {
						return targetTuple, nil
					}
					newElems := append(append([]Type(nil), targetTuple.Elems...), argT)
					return &TupleType{Elems: newElems}, nil
				case "Prepend":
					if len(e.Args) != 1 {
						return nil, inf.nodeErr(e, "Prepend expects 1 argument, got %d", len(e.Args))
					}
					argT, err := inf.inferExpr(e.Args[0])
					if err != nil {
						return nil, err
					}
					newElems := append([]Type{argT}, targetTuple.Elems...)
					return &TupleType{Elems: newElems, IsVariadic: targetTuple.IsVariadic}, nil
				case "Concat":
					if len(e.Args) < 1 {
						return nil, inf.nodeErr(e, "Concat expects at least 1 argument, got 0")
					}
					merged := append([]Type(nil), targetTuple.Elems...)
					isVar := targetTuple.IsVariadic
					for i, arg := range e.Args {
						argT, err := inf.inferExpr(arg)
						if err != nil {
							return nil, err
						}
						if argTuple, ok := argT.(*TupleType); ok {
							merged = append(merged, argTuple.Elems...)
							if argTuple.IsVariadic {
								isVar = true
							}
						} else if argT == TypeUnknown {
							isVar = true
						} else {
							return nil, inf.nodeErr(arg, "Concat argument %d must be a tuple, got %s", i+1, argT)
						}
					}
					return &TupleType{Elems: merged, IsVariadic: isVar}, nil
				case "Slice":
					if len(e.Args) < 1 || len(e.Args) > 2 {
						return nil, inf.nodeErr(e, "Slice expects 1 or 2 arguments, got %d", len(e.Args))
					}
					for _, arg := range e.Args {
						argT, err := inf.inferExpr(arg)
						if err != nil {
							return nil, err
						}
						if argT != TypeInt && argT != TypeUnknown {
							return nil, inf.nodeErr(arg, "Slice arguments must be Int, got %s", argT)
						}
					}
					if len(e.Args) == 1 {
						if intLit, ok := e.Args[0].(*ast.IntLit); ok && intLit.Value >= 0 && int(intLit.Value) <= len(targetTuple.Elems) && !targetTuple.IsVariadic {
							return &TupleType{Elems: append([]Type(nil), targetTuple.Elems[int(intLit.Value):]...)}, nil
						}
					} else if len(e.Args) == 2 {
						sLit, sOK := e.Args[0].(*ast.IntLit)
						eLit, eOK := e.Args[1].(*ast.IntLit)
						if sOK && eOK && sLit.Value >= 0 && eLit.Value >= sLit.Value && int(eLit.Value) <= len(targetTuple.Elems) && !targetTuple.IsVariadic {
							return &TupleType{Elems: append([]Type(nil), targetTuple.Elems[int(sLit.Value):int(eLit.Value)]...)}, nil
						}
					}
					var elemUnion []Type
					for _, el := range targetTuple.Elems {
						elemUnion = append(elemUnion, el)
					}
					var retElem Type = TypeUnknown
					if len(elemUnion) > 0 {
						retElem = NewUnionType(elemUnion...)
					}
					return &TupleType{Elems: []Type{retElem}, IsVariadic: true}, nil
				}
			}
		}
		calleeT, err := inf.inferExpr(e.Callee)
		if err != nil {
			return nil, err
		}
		effCalleeT := calleeT
		if tp, ok := calleeT.(*TypeParamType); ok && tp.Constraint != nil {
			effCalleeT = tp.Constraint
		}
		if st, ok := effCalleeT.(*StructType); ok {
			if len(st.TypeParams) > 0 && len(st.TypeArgs) == 0 {
				bindings := make(map[string]Type)
				argStartIdx := 0
				if len(e.Args) > 0 {
					if _, isNamed := e.Args[0].(*ast.NamedArgExpr); !isNamed {
						if firstT, err := inf.inferExpr(e.Args[0]); err == nil {
							if firstSt, ok := firstT.(*StructType); ok && firstSt.Name == st.Name && len(firstSt.TypeArgs) == len(st.TypeParams) {
								for i, tp := range st.TypeParams {
									bindings[tp.Name] = firstSt.TypeArgs[i]
								}
								argStartIdx = 1
							}
						}
					}
				}
				var initTuple *TupleType
				if initFn, hasInit := st.Methods["init"]; hasInit {
					if tup, ok := initFn.Arg.(*TupleType); ok {
						initTuple = tup
					}
				}
				posIdx := 0
				for i := argStartIdx; i < len(e.Args); i++ {
					arg := e.Args[i]
					if named, isNamed := arg.(*ast.NamedArgExpr); isNamed {
						if expectedFieldT, hasField := st.Fields[named.Name]; hasField {
							valT, err := inf.inferExpr(named.Value)
							if err == nil {
								unifyTypeParams(expectedFieldT, valT, bindings)
							}
						}
					} else {
						var expectedParamT Type
						if initTuple != nil {
							if posIdx < len(initTuple.Elems) {
								expectedParamT = initTuple.Elems[posIdx]
							}
						} else if posIdx < len(st.FieldOrder) {
							fName := st.FieldOrder[posIdx]
							expectedParamT = st.Fields[fName]
						}
						if expectedParamT != nil {
							valT, err := inf.inferExpr(arg)
							if err == nil {
								unifyTypeParams(expectedParamT, valT, bindings)
							}
						}
						posIdx++
					}
				}
				typeArgs := make([]Type, len(st.TypeParams))
				for i, tp := range st.TypeParams {
					if bound, ok := bindings[tp.Name]; ok {
						typeArgs[i] = bound
					} else {
						typeArgs[i] = TypeUnknown
					}
				}
				if err := ValidateTypeConstraints(st.TypeParams, typeArgs); err != nil {
					return nil, inf.nodeErr(e, "%s", err.Error())
				}
				st = InstantiateStruct(st, typeArgs)
				calleeT = st
			}
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
		if iface, ok := effCalleeT.(*InterfaceType); ok && iface.Name == "Error" {
			for _, argExpr := range e.Args {
				if _, err := inf.inferExpr(argExpr); err != nil {
					return nil, err
				}
			}
			return iface, nil
		}
		if fnT, ok := effCalleeT.(*FuncType); ok {
			if len(fnT.TypeParams) > 0 {
				bindings := make(map[string]Type)
				argTuple, isTuple := fnT.Arg.(*TupleType)
				if isTuple {
					callArgs := e.Args
					if fnT.BlockParam == nil && e.Block != nil {
						callArgs = append(append([]ast.Expr(nil), e.Args...), e.Block)
					}
					// Phase A: Non-lambda arguments
					for i, argExpr := range callArgs {
						if _, isLambda := argExpr.(*ast.LambdaExpr); !isLambda {
							argT, err := inf.inferExpr(argExpr)
							if err == nil {
								var paramT Type
								if i < len(argTuple.Elems) {
									paramT = argTuple.Elems[i]
								} else if argTuple.IsVariadic && len(argTuple.Elems) > 0 {
									paramT = argTuple.Elems[len(argTuple.Elems)-1]
								}
								if paramT != nil {
									unifyTypeParams(paramT, argT, bindings)
								}
							}
						}
					}
					// Phase B: Lambda arguments with propagated parameter types
					for i, argExpr := range callArgs {
						if lam, isLambda := argExpr.(*ast.LambdaExpr); isLambda {
							var paramT Type
							if i < len(argTuple.Elems) {
								paramT = argTuple.Elems[i]
							} else if argTuple.IsVariadic && len(argTuple.Elems) > 0 {
								paramT = argTuple.Elems[len(argTuple.Elems)-1]
							}
							if paramT != nil {
								subParamT := Substitute(paramT, bindings)
								if lamFnT, ok := subParamT.(*FuncType); ok {
									if lamArgTup, isTup := lamFnT.Arg.(*TupleType); isTup {
										for idx := range lam.Params {
											if lam.Params[idx].TypeAnnot == "" && idx < len(lamArgTup.Elems) && lamArgTup.Elems[idx] != TypeUnknown {
												lam.Params[idx].TypeAnnot = lamArgTup.Elems[idx].String()
											}
										}
									}
								}
								lamT, err := inf.inferExpr(lam)
								if err == nil {
									unifyTypeParams(paramT, lamT, bindings)
								}
							}
						}
					}
					// Phase B2: e.Block if fnT.BlockParam != nil
					if fnT.BlockParam != nil && e.Block != nil {
						subBp := Substitute(fnT.BlockParam, bindings)
						if bpFunc, isFunc := subBp.(*FuncType); isFunc {
							if argTup, isTup := bpFunc.Arg.(*TupleType); isTup {
								for idx := range e.Block.Params {
									if e.Block.Params[idx].TypeAnnot == "" && idx < len(argTup.Elems) && argTup.Elems[idx] != TypeUnknown {
										e.Block.Params[idx].TypeAnnot = argTup.Elems[idx].String()
									}
								}
							}
						}
						bT, err := inf.inferExpr(e.Block)
						if err == nil {
							unifyTypeParams(fnT.BlockParam, bT, bindings)
						}
					}
				}
				typeArgs := make([]Type, len(fnT.TypeParams))
				for i, tp := range fnT.TypeParams {
					if bound, ok := bindings[tp.Name]; ok {
						typeArgs[i] = bound
					} else {
						typeArgs[i] = TypeUnknown
					}
				}
				if err := ValidateTypeConstraints(fnT.TypeParams, typeArgs); err != nil {
					return nil, inf.nodeErr(e, "%s", err.Error())
				}
				origFnT := fnT
				instFn := Substitute(fnT, bindings).(*FuncType)
				instFn.TypeParams = nil
				instFn.TypeArgs = typeArgs
				fnT = instFn
				calleeT = instFn
				if inf.funcCtx != nil && (inf.funcCtx.fnType == origFnT || inf.funcCtx.fnType == fnT) {
					inf.funcCtx.selfCalled = true
				}
			}
			if inf.funcCtx != nil && inf.funcCtx.fnType == fnT {
				inf.funcCtx.selfCalled = true
			}
			args := e.Args
			if fnT.BlockParam != nil {
				if e.Block != nil {
					// If the block parameter is a function type with known argument types,
					// propagate them to unannotated parameters of the block literal so they don't warn as Unknown.
					if bpFunc, isFunc := fnT.BlockParam.(*FuncType); isFunc {
						if argTup, isTup := bpFunc.Arg.(*TupleType); isTup {
							for idx := range e.Block.Params {
								if e.Block.Params[idx].TypeAnnot == "" {
									if idx < len(argTup.Elems) && argTup.Elems[idx] != TypeUnknown {
										e.Block.Params[idx].TypeAnnot = argTup.Elems[idx].String()
									}
								}
							}
						}
					}
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
		effTargetT := targetT
		if tp, ok := targetT.(*TypeParamType); ok && tp.Constraint != nil {
			effTargetT = tp.Constraint
		}
		if st, ok := effTargetT.(*StructType); ok {
			return st.LookupMember(e.Member)
		}
		if it, ok := effTargetT.(*InterfaceType); ok {
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
	case *ast.TypeInstExpr:
		targetT, err := inf.inferExpr(e.Target)
		if err != nil {
			return nil, err
		}
		args := make([]Type, len(e.TypeArgs))
		for i, aStr := range e.TypeArgs {
			t, err := inf.parseTypeAnnot(aStr)
			if err != nil {
				return nil, err
			}
			args[i] = t
		}
		if st, ok := targetT.(*StructType); ok {
			if err := ValidateTypeConstraints(st.TypeParams, args); err != nil {
				return nil, inf.nodeErr(e, "%s", err.Error())
			}
			return InstantiateStruct(st, args), nil
		}
		if fnT, ok := targetT.(*FuncType); ok {
			if err := ValidateTypeConstraints(fnT.TypeParams, args); err != nil {
				return nil, inf.nodeErr(e, "%s", err.Error())
			}
			bindings := make(map[string]Type)
			for i, tp := range fnT.TypeParams {
				bindings[tp.Name] = args[i]
			}
			instFn := Substitute(fnT, bindings).(*FuncType)
			instFn.TypeParams = nil
			instFn.TypeArgs = args
			return instFn, nil
		}
		if iface, ok := targetT.(*InterfaceType); ok {
			if err := ValidateTypeConstraints(iface.TypeParams, args); err != nil {
				return nil, inf.nodeErr(e, "%s", err.Error())
			}
			return InstantiateInterface(iface, args), nil
		}
		return nil, inf.nodeErr(e, "cannot instantiate type arguments on %s", targetT.String())

	case *ast.IndexExpr:
		targetT, err := inf.inferExpr(e.Target)
		if err != nil {
			return nil, err
		}
		if e.TypeArg != "" {
			if st, ok := targetT.(*StructType); ok && len(st.TypeParams) > 0 && len(st.TypeArgs) == 0 {
				if argT, err := inf.parseTypeAnnot(e.TypeArg); err == nil {
					args := []Type{argT}
					if err := ValidateTypeConstraints(st.TypeParams, args); err != nil {
						return nil, inf.nodeErr(e, "%s", err.Error())
					}
					return InstantiateStruct(st, args), nil
				}
			}
			if fnT, ok := targetT.(*FuncType); ok && len(fnT.TypeParams) > 0 && len(fnT.TypeArgs) == 0 {
				if argT, err := inf.parseTypeAnnot(e.TypeArg); err == nil {
					args := []Type{argT}
					if err := ValidateTypeConstraints(fnT.TypeParams, args); err != nil {
						return nil, inf.nodeErr(e, "%s", err.Error())
					}
					bindings := map[string]Type{fnT.TypeParams[0].Name: argT}
					instFn := Substitute(fnT, bindings).(*FuncType)
					instFn.TypeParams = nil
					instFn.TypeArgs = args
					return instFn, nil
				}
			}
			if iface, ok := targetT.(*InterfaceType); ok && len(iface.TypeParams) > 0 && len(iface.TypeArgs) == 0 {
				if argT, err := inf.parseTypeAnnot(e.TypeArg); err == nil {
					args := []Type{argT}
					if err := ValidateTypeConstraints(iface.TypeParams, args); err != nil {
						return nil, inf.nodeErr(e, "%s", err.Error())
					}
					return InstantiateInterface(iface, args), nil
				}
			}
		}
		if _, err := inf.inferExpr(e.Index); err != nil {
			return nil, err
		}
		effTargetT := targetT
		if tp, ok := targetT.(*TypeParamType); ok && tp.Constraint != nil {
			effTargetT = tp.Constraint
		}
		if st, ok := effTargetT.(*StructType); ok {
			if m, err := st.LookupMember("[]"); err == nil {
				if fnT, ok := m.(*FuncType); ok {
					return fnT.Ret, nil
				}
			}
		}
		if it, ok := effTargetT.(*InterfaceType); ok {
			if m, ok := it.LookupMethod("[]"); ok {
				return m.Ret, nil
			}
		}
		if tt, ok := effTargetT.(*TupleType); ok {
			if len(tt.Elems) > 0 {
				if !tt.IsVariadic {
					if intLit, ok := e.Index.(*ast.IntLit); ok && intLit.BigValue == nil {
						idx := int(intLit.Value)
						if idx >= 0 && idx < len(tt.Elems) {
							return tt.Elems[idx], nil
						}
					} else if uExpr, ok := e.Index.(*ast.UnaryExpr); ok && uExpr.Op == "-" {
						if intLit, ok := uExpr.Expr.(*ast.IntLit); ok && intLit.BigValue == nil {
							idx := len(tt.Elems) - int(intLit.Value)
							if idx >= 0 && idx < len(tt.Elems) {
								return tt.Elems[idx], nil
							}
						}
					}
					return NewUnionType(tt.Elems...), nil
				}
				return NewUnionType(tt.Elems...), nil
			}
		} else if dt, ok := effTargetT.(*DictType); ok {
			return dt.Value, nil
		} else if effTargetT == TypeString {
			return TypeString, nil
		} else if effTargetT == TypeBytes {
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
		t, _, err := inf.inferMatchExpr(e)
		return t, err

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
			} else {
				inf.addWarning(p.Pos(), "Unknown type of parameter '%s' in lambda", p.Name)
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

		closureCtx := &breakTargetCtx{
			parent: inf.breakCtx,
			isLoop: false,
		}
		prevBreakCtx := inf.breakCtx
		inf.breakCtx = closureCtx

		prevScope := inf.scope
		inf.scope = childScope

		ftTypes, canFT, err := inf.inferBlock(e.Body)

		inf.scope = prevScope
		inf.breakCtx = prevBreakCtx

		if err != nil {
			return nil, err
		}

		var allRetTypes []Type
		allRetTypes = append(allRetTypes, closureCtx.breakTypes...)
		if canFT {
			allRetTypes = append(allRetTypes, ftTypes...)
		}

		var retT Type = TypeUnknown
		if len(allRetTypes) > 0 {
			retT = NewUnionType(allRetTypes...)
		} else if canFT {
			retT = &TupleType{Elems: []Type{}}
		}

		return &FuncType{
			Arg: &TupleType{
				Elems:      callerParamTypes,
				IsVariadic: isVariadic,
			},
			MinArity: minArity,
			MaxArity: maxArity,
			Ret:      retT,
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

	// Tuple annotation or grouping: (...) or (Int, ...) or (Int, String) or ((Int) -> String)
	if strings.HasPrefix(annot, "(") && strings.HasSuffix(annot, ")") {
		inner := strings.TrimSpace(annot[1 : len(annot)-1])
		if inner == "" {
			return &TupleType{Elems: []Type{}}, nil
		}
		if inner == "..." {
			return &TupleType{Elems: []Type{TypeUnknown}, IsVariadic: true}, nil
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

	// Dict annotation: {...} or {Key: Value}
	if strings.HasPrefix(annot, "{") && strings.HasSuffix(annot, "}") {
		inner := strings.TrimSpace(annot[1 : len(annot)-1])
		if inner == "..." {
			return &DictType{Key: TypeUnknown, Value: TypeUnknown}, nil
		}
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

	// Parameterized type annotation: Name[T1, T2] or @mod.Name[T1, T2]
	if strings.HasSuffix(annot, "]") {
		depth := 0
		bracketIdx := -1
		for i := len(runes) - 1; i >= 0; i-- {
			if runes[i] == ']' {
				depth++
			} else if runes[i] == '[' {
				depth--
				if depth == 0 {
					bracketIdx = i
					break
				}
			}
		}
		if bracketIdx > 0 {
			baseStr := strings.TrimSpace(string(runes[:bracketIdx]))
			argsStr := strings.TrimSpace(string(runes[bracketIdx+1 : len(runes)-1]))
			baseType, err := inf.parseTypeAnnot(baseStr)
			if err != nil {
				return nil, err
			}
			argTypes, err := inf.parseTupleElements(argsStr)
			if err != nil {
				return nil, err
			}
			if st, ok := baseType.(*StructType); ok {
				if err := ValidateTypeConstraints(st.TypeParams, argTypes); err != nil {
					return nil, err
				}
				return InstantiateStruct(st, argTypes), nil
			}
			if iface, ok := baseType.(*InterfaceType); ok {
				if err := ValidateTypeConstraints(iface.TypeParams, argTypes); err != nil {
					return nil, err
				}
				return InstantiateInterface(iface, argTypes), nil
			}
			return nil, fmt.Errorf("type '%s' does not take type arguments", baseStr)
		}
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

func unifyTypeParams(paramType, argType Type, bindings map[string]Type) {
	if paramType == nil || argType == nil || argType == TypeUnknown {
		return
	}
	switch pt := paramType.(type) {
	case *TypeParamType:
		if existing, ok := bindings[pt.Name]; ok {
			if !existing.Equals(argType) {
				bindings[pt.Name] = NewUnionType(existing, argType)
			}
		} else {
			bindings[pt.Name] = argType
		}
	case *TupleType:
		if at, ok := argType.(*TupleType); ok {
			for i := 0; i < len(pt.Elems) && i < len(at.Elems); i++ {
				unifyTypeParams(pt.Elems[i], at.Elems[i], bindings)
			}
			if pt.IsVariadic && len(pt.Elems) > 0 {
				varElem := pt.Elems[len(pt.Elems)-1]
				for i := len(pt.Elems) - 1; i < len(at.Elems); i++ {
					unifyTypeParams(varElem, at.Elems[i], bindings)
				}
			}
		}
	case *DictType:
		if at, ok := argType.(*DictType); ok {
			unifyTypeParams(pt.Key, at.Key, bindings)
			unifyTypeParams(pt.Value, at.Value, bindings)
		}
	case *FuncType:
		if at, ok := argType.(*FuncType); ok {
			unifyTypeParams(pt.effectiveArgType(), at.effectiveArgType(), bindings)
			unifyTypeParams(pt.Ret, at.Ret, bindings)
		}
	case *UnionType:
		if at, ok := argType.(*UnionType); ok {
			for i := 0; i < len(pt.Members) && i < len(at.Members); i++ {
				unifyTypeParams(pt.Members[i], at.Members[i], bindings)
			}
		} else {
			for _, m := range pt.Members {
				if m != TypeNil {
					unifyTypeParams(m, argType, bindings)
				}
			}
		}
	case *StructType:
		if at, ok := argType.(*StructType); ok && pt.Name == at.Name {
			for i := 0; i < len(pt.TypeArgs) && i < len(at.TypeArgs); i++ {
				unifyTypeParams(pt.TypeArgs[i], at.TypeArgs[i], bindings)
			}
		}
	case *InterfaceType:
		if at, ok := argType.(*InterfaceType); ok && pt.Name == at.Name {
			for i := 0; i < len(pt.TypeArgs) && i < len(at.TypeArgs); i++ {
				unifyTypeParams(pt.TypeArgs[i], at.TypeArgs[i], bindings)
			}
		}
	}
}

func isEmptyCollection(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.DictLit:
		return len(e.Entries) == 0
	case *ast.TupleLit:
		return len(e.Elements) == 0
	default:
		return false
	}
}

func containsUnknown(t Type) bool {
	if t == nil || t == TypeUnknown {
		return true
	}
	switch v := t.(type) {
	case *TupleType:
		for _, el := range v.Elems {
			if containsUnknown(el) {
				return true
			}
		}
	case *DictType:
		return containsUnknown(v.Key) || containsUnknown(v.Value)
	case *UnionType:
		for _, m := range v.Members {
			if containsUnknown(m) {
				return true
			}
		}
	case *FuncType:
		return containsUnknown(v.Arg) || containsUnknown(v.Ret)
	}
	return false
}

func (inf *Inferencer) inferMatchExpr(e *ast.MatchExpr) (Type, bool, error) {
	emptyTuple := &TupleType{Elems: []Type{}}
	targetT, err := inf.inferExpr(e.Target)
	if err != nil {
		return nil, false, err
	}
	var caseTypes []Type
	allCasesTerminated := (len(e.Cases) > 0)
	hasWildcard := false
	hasTrue := false
	hasFalse := false
	var coveredTypes []Type

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
			if pat == "_" {
				hasWildcard = true
			} else if pat == "true" {
				hasTrue = true
			} else if pat == "false" {
				hasFalse = true
			} else if pat != "" {
				patT, err := inf.parseTypeAnnot(pat)
				if err != nil {
					return nil, false, err
				}
				coveredTypes = append(coveredTypes, patT)
				// If target is an identifier, narrow its type inside this case scope
				if id, isId := e.Target.(*ast.Identifier); isId {
					_ = caseScope.Declare(id.Name, patT)
				}
			}
		}

		prevScope := inf.scope
		inf.scope = caseScope

		ftTypes, canFT, err := inf.inferBlock(c.Body)
		inf.scope = prevScope
		if err != nil {
			return nil, false, err
		}

		if canFT {
			allCasesTerminated = false
			caseTypes = append(caseTypes, ftTypes...)
		}
	}

	isExhaustive := hasWildcard
	if !isExhaustive && targetT != nil && targetT != TypeUnknown {
		if targetT == TypeBool && hasTrue && hasFalse {
			isExhaustive = true
		} else if len(coveredTypes) > 0 && IsAssignableTo(targetT, NewUnionType(coveredTypes...)) {
			isExhaustive = true
		}
	}

	if allCasesTerminated && isExhaustive {
		return TypeUnknown, false, nil
	}

	if len(caseTypes) == 0 {
		return emptyTuple, true, nil
	}

	if !isExhaustive {
		caseTypes = append(caseTypes, emptyTuple)
	}
	return NewUnionType(caseTypes...), true, nil
}

func getFirstParamType(fnT *FuncType) (Type, bool) {
	if fnT == nil {
		return nil, false
	}
	if tup, ok := fnT.Arg.(*TupleType); ok && len(tup.Elems) > 0 {
		return tup.Elems[0], true
	}
	return nil, false
}

