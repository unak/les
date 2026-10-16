package eval

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"les/pkg/ast"
	"les/pkg/types"
)

var callIDCounter uint64

func nextCallID() uint64 {
	return atomic.AddUint64(&callIDCounter, 1)
}

// Evaluator runs Les AST nodes.
type Evaluator struct {
	builtinsEnv      *Environment
	globalEnv        *Environment
	inferencer       *types.Inferencer
	fileModuleLoader *FileModuleLoader
}

func NewEvaluator() *Evaluator {
	bEnv := NewEnvironment(nil)
	ev := &Evaluator{builtinsEnv: bEnv, globalEnv: NewEnvironment(bEnv)}
	ev.initIOModule()
	ev.initBufioModule()
	ev.registerBuiltins()
	return ev
}

func (ev *Evaluator) SetInferencer(inf *types.Inferencer) {
	ev.inferencer = inf
	if ev.builtinsEnv != nil {
		ev.builtinsEnv.Inferencer = inf
	}
	if ev.globalEnv != nil {
		ev.globalEnv.Inferencer = inf
	}
}

func (ev *Evaluator) SetExprTypes(exprTypes map[ast.Expr]types.Type) {
	if ev.inferencer == nil {
		ev.inferencer = types.NewInferencer()
	}
	ev.inferencer.ExprTypes = exprTypes
	if ev.builtinsEnv != nil {
		ev.builtinsEnv.Inferencer = ev.inferencer
	}
	if ev.globalEnv != nil {
		ev.globalEnv.Inferencer = ev.inferencer
	}
}

func (ev *Evaluator) newThunk(expr ast.Expr, env *Environment) *Thunk {
	thunk := NewThunk(expr, env)
	if expr != nil && ev != nil && ev.inferencer != nil && ev.inferencer.ExprTypes != nil {
		if t, ok := ev.inferencer.ExprTypes[expr]; ok {
			thunk.InferredType = t
		}
	}
	return thunk
}

func (ev *Evaluator) SetOutput(w io.Writer) {
	if oldBW := ev.builtinsEnv.StdoutWriter; oldBW != nil {
		_ = oldBW.Flush()
		oldBW.Reset(io.Discard)
		ev.builtinsEnv.UnregisterBufioWriter(oldBW)
	}
	ev.builtinsEnv.RawStdout = w
	if w != nil {
		bw := bufio.NewWriter(w)
		ev.builtinsEnv.StdoutWriter = bw
		ev.builtinsEnv.RegisterBufioWriter(bw, w)
	} else {
		ev.builtinsEnv.StdoutWriter = nil
	}
}

func (ev *Evaluator) SetErrorOutput(w io.Writer) {
	ev.builtinsEnv.RawStderr = w
}

func (ev *Evaluator) SetInput(r io.Reader) {
	ev.builtinsEnv.RawStdin = r
	if r != nil {
		ev.builtinsEnv.StdinReader = bufio.NewReader(r)
	} else {
		ev.builtinsEnv.StdinReader = nil
	}
}

func (ev *Evaluator) callValue(calleeVal Value, args []Value) (Value, error) {
	forced := Force(calleeVal)
	switch fn := forced.(type) {
	case *BuiltinFunc:
		return fn.Fn(args)
	case *Closure:
		return ev.globalEnv.invokeClosure(fn, args)
	default:
		return nil, fmt.Errorf("cannot call non-function value: %v", forced)
	}
}

func (ev *Evaluator) SetModuleLoader(loader func(path string) (*ModuleValue, error)) {
	ev.builtinsEnv.Loader = loader
	ev.globalEnv.Loader = loader
}

func (ev *Evaluator) SetFileModuleLoader(fml *FileModuleLoader) {
	ev.fileModuleLoader = fml
	if fml != nil {
		fml.SetParentEnv(ev.builtinsEnv)
		ev.builtinsEnv.Loader = fml.Load
		ev.globalEnv.Loader = fml.Load
	}
}

func (ev *Evaluator) registerBuiltins() {
	// print: built-in effect boundary. Demands evaluation of its arguments without automatic newline.
	ev.builtinsEnv.Set("print", &BuiltinFunc{
		Name:     "print",
		FuncType: types.BuiltinPrintFuncType,
		Fn: func(args []Value) (Value, error) {
			parts := make([]string, len(args))
			for i, a := range args {
				val := Force(a)
				if !checkValueMatchesType(val, "Show", ev.globalEnv) {
					return nil, fmt.Errorf("type error in print: argument %d of type %s does not implement Show interface", i+1, typeNameOf(val))
				}
				parts[i] = val.String()
			}
			out := strings.Join(parts, "")
			w := ev.globalEnv.GetStdoutWriter()
			if _, err := w.WriteString(out); err != nil {
				return nil, err
			}
			if err := w.Flush(); err != nil {
				return nil, err
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	})

	// exit: terminates process with exit code (0 or 1 arg, Bool or Int).
	ev.builtinsEnv.Set("exit", &BuiltinFunc{
		Name:     "exit",
		FuncType: types.BuiltinExitFuncType,
		Fn: func(args []Value) (Value, error) {
			code := 0
			if len(args) == 0 {
				code = 0
			} else if len(args) == 1 {
				val := Force(args[0])
				switch v := val.(type) {
				case IntValue:
					code = int(v)
				case BoolValue:
					if bool(v) {
						code = 0
					} else {
						code = 1
					}
				default:
					return nil, fmt.Errorf("exit expects Int or Bool, got %v", val)
				}
			} else {
				return nil, fmt.Errorf("exit expects 0 or 1 argument, got %d", len(args))
			}
			panic(&ExitError{Code: code})
		},
	})

	// Bytes: creates Bytes from String or empty if 0 args
	ev.builtinsEnv.Set("Bytes", &BuiltinFunc{
		Name:     "Bytes",
		FuncType: types.BuiltinBytesFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) == 0 {
				return BytesValue(""), nil
			}
			if len(args) == 1 {
				forced := Force(args[0])
				switch v := forced.(type) {
				case StringValue:
					return BytesValue(string(v)), nil
				case BytesValue:
					return v, nil
				default:
					return nil, fmt.Errorf("Bytes constructor expects String or Bytes, got %s", typeNameOf(forced))
				}
			}
			return nil, fmt.Errorf("Bytes expects 0 or 1 arguments, got %d", len(args))
		},
	})

	// Global standard I/O streams
	bufioMod, _ := ev.builtinsEnv.GetModule("bufio")
	ioMod, _ := ev.builtinsEnv.GetModule("io")

	stdinConstructor := bufioMod.Exports["Reader"].(*BuiltinFunc)
	stdoutConstructor := bufioMod.Exports["Writer"].(*BuiltinFunc)

	stdinInst, _ := stdinConstructor.Fn([]Value{ioMod.Exports["Stdin"]})
	stdoutInst, _ := stdoutConstructor.Fn([]Value{ioMod.Exports["Stdout"]})
	stderrInst := ioMod.Exports["Stderr"]

	ev.builtinsEnv.Set("Stdin", stdinInst)
	ev.builtinsEnv.Set("Stdout", stdoutInst)
	ev.builtinsEnv.Set("Stderr", stderrInst)

	// gets: reads a single line from standard input, returning String or Nil on EOF.
	// Returns a lazy ActionThunk that only blocks when demanded.
	ev.builtinsEnv.Set("gets", &BuiltinFunc{
		Name:     "gets",
		FuncType: types.BuiltinGetsFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("gets expects 0 arguments, got %d", len(args))
			}
			si := stdinInst.(*StructInstance)
			rlFn := si.Fields["ReadLine"].(*BuiltinFunc)
			return rlFn.Fn([]Value{})
		},
	})

	// Default CLI arguments & program name
	ev.builtinsEnv.Set("Args", &TupleValue{Elements: []Value{}})
	ev.builtinsEnv.Set("ProgramName", StringValue(""))
	ev.builtinsEnv.Set("scriptName", StringValue(""))

	// Built-in Error constructor & default Error struct definition
	errorMethodFuncType := &types.FuncType{
		Arg: &types.TupleType{Elems: []types.Type{}},
		Ret: types.TypeString,
	}
	errorStructDef := &StructDefValue{
		Name:          "SimpleError",
		FieldOrder:    []string{"message"},
		FieldDefaults: make(map[string]ast.Expr),
		EmbeddedNames: []string{},
		Methods: map[string]*ast.FuncDef{
			"Message": {
				Name: "Message",
				Body: []ast.Stmt{
					&ast.ReturnStmt{Value: &ast.Identifier{Name: "message"}},
				},
			},
			"ToString": {
				Name: "ToString",
				Body: []ast.Stmt{
					&ast.ReturnStmt{
						Value: &ast.BinaryExpr{
							Op:    "+",
							Left:  &ast.StringLit{Value: "Error: "},
							Right: &ast.Identifier{Name: "message"},
						},
					},
				},
			},
			"Error": {
				Name: "Error",
				Body: []ast.Stmt{
					&ast.ReturnStmt{Value: &ast.Identifier{Name: "message"}},
				},
			},
		},
		Env: ev.builtinsEnv,
		StructType: &types.StructType{
			Name: "SimpleError",
			Fields: map[string]types.Type{
				"message": types.TypeString,
			},
			FieldOrder: []string{"message"},
			Methods: map[string]*types.FuncType{
				"Message":  errorMethodFuncType,
				"ToString": errorMethodFuncType,
				"Error":    errorMethodFuncType,
			},
		},
	}

	ev.builtinsEnv.Set("Error", &BuiltinFunc{
		Name: "Error",
		FuncType: &types.FuncType{
			Arg:      &types.TupleType{Elems: []types.Type{types.BuiltinShowIface}},
			MinArity: 0,
			MaxArity: 1,
			Ret:      types.BuiltinErrorIface,
		},
		Fn: func(args []Value) (Value, error) {
			msg := "error"
			if len(args) > 0 {
				msg = Force(args[0]).String()
			}
			fields := map[string]Value{
				"message": StringValue(msg),
			}
			inst := &StructInstance{
				Def:      errorStructDef,
				Fields:   fields,
				Embedded: make(map[string]*StructInstance),
			}
			return inst, nil
		},
	})

	// Built-in Show interface definition: ToString(): String, Type(): String
	ev.builtinsEnv.Set("Show", &InterfaceDefValue{
		Name: "Show",
		Methods: map[string]ast.MethodSig{
			"ToString": {
				Name:   "ToString",
				Params: []ast.FuncParam{},
			},
			"Type": {
				Name:   "Type",
				Params: []ast.FuncParam{},
			},
		},
	})
}

func (ev *Evaluator) SetArgs(args []string) {
	elems := make([]Value, len(args))
	for i, a := range args {
		elems[i] = StringValue(a)
	}
	ev.builtinsEnv.Set("Args", &TupleValue{Elements: elems})
}

func (ev *Evaluator) SetProgramName(name string) {
	ev.builtinsEnv.Set("ProgramName", StringValue(name))
}

func (ev *Evaluator) SetScriptName(name string) {
	ev.globalEnv.Set("scriptName", StringValue(name))
}

// ExitError indicates that exit() was called to terminate execution.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit with code %d", e.Code)
}

func typeNameOf(val Value) string {
	return typeNameWithDepth(val, 0)
}

func valueOrThunkTypeNameWithDepth(val Value, depth int) string {
	if evaluated, ok := PeekEvaluated(val); ok {
		return typeNameWithDepth(evaluated, depth)
	}
	if thunk, ok := val.(*Thunk); ok {
		if thunk.InferredType != nil {
			if t, ok := thunk.InferredType.(types.Type); ok && t != types.TypeUnknown {
				return t.String()
			}
		}
		return "Unknown"
	}
	return typeNameWithDepth(Force(val), depth)
}

func joinSortedTypeSet(typeSet map[string]bool) string {
	typesList := make([]string, 0, len(typeSet))
	for t := range typeSet {
		typesList = append(typesList, t)
	}
	sort.Strings(typesList)
	return strings.Join(typesList, " | ")
}

func typeNameWithDepth(val Value, depth int) string {
	if depth > 10 {
		return "..."
	}
	switch v := val.(type) {
	case IntValue:
		return "Int"
	case FloatValue:
		return "Float"
	case StringValue:
		return "String"
	case BytesValue:
		return "Bytes"
	case BoolValue:
		return "Bool"
	case NilValue:
		return "Nil"
	case *TupleValue:
		parts := make([]string, len(v.Elements))
		for i, el := range v.Elements {
			parts[i] = valueOrThunkTypeNameWithDepth(el, depth+1)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *DictValue:
		if len(v.Entries) == 0 {
			return "{Unknown: Unknown}"
		}
		keyTypesSet := make(map[string]bool)
		valTypesSet := make(map[string]bool)

		for dk, entryVal := range v.Entries {
			var kType string
			if v.KeyValues != nil {
				if actualKey, ok := v.KeyValues[dk]; ok && actualKey != nil {
					kType = valueOrThunkTypeNameWithDepth(actualKey, depth+1)
				}
			}
			if kType == "" {
				kType = dk.Type
			}
			keyTypesSet[kType] = true

			vType := valueOrThunkTypeNameWithDepth(entryVal, depth+1)
			valTypesSet[vType] = true
		}

		keyTypeStr := joinSortedTypeSet(keyTypesSet)
		valTypeStr := joinSortedTypeSet(valTypesSet)
		return fmt.Sprintf("{%s: %s}", keyTypeStr, valTypeStr)
	case *StructInstance:
		return v.Def.Name
	case *StructDefValue:
		if v.StructType != nil {
			if st, ok := v.StructType.(*types.StructType); ok {
				if initFn, ok := st.Methods["init"]; ok {
					return initFn.String()
				}
			}
		}
		return "() -> " + v.Name

	case *Closure:
		if v.FuncType != nil {
			if t, ok := v.FuncType.(*types.FuncType); ok {
				return t.String()
			}
		}
		return "Function"
	case *BuiltinFunc:
		if v.FuncType != nil {
			if t, ok := v.FuncType.(*types.FuncType); ok {
				return t.String()
			}
		}
		return "Function"
	default:
		return "Unknown"
	}
}

func (ev *Evaluator) EvalProgram(prog *ast.Program) (val Value, err error) {
	if ev.inferencer == nil {
		inf := types.NewInferencer()
		if ev.fileModuleLoader != nil {
			inf.SetModuleLoader(types.NewFileModuleLoader(ev.fileModuleLoader.baseDir).Load)
		}
		_ = inf.InferProgram(prog)
		ev.SetInferencer(inf)
	}
	defer func() {
		if r := recover(); r != nil {
			if exitErr, ok := r.(*ExitError); ok {
				// Normal termination via exit() or top-level return: flush all bufio writers
				flushErr := ev.globalEnv.FlushAllBufioWriters()
				if flushErr != nil {
					ev.globalEnv.ResetBufioWritersOnError()
					if exitErr.Code == 0 {
						err = flushErr
						return
					}
				}
				err = exitErr
				return
			}
			ev.globalEnv.ResetBufioWritersOnError()
			if _, ok := r.(*ReturnSignal); ok {
				err = fmt.Errorf("return outside of function")
				return
			}
			if _, ok := r.(*BreakSignal); ok {
				err = fmt.Errorf("break outside of closure")
				return
			}
			// Runtime error (panic): A3: do NOT flush on runtime error
			err = fmt.Errorf("%v", r)
			return
		}
		// Normal completion without panic/exit
		if err == nil {
			if flushErr := ev.globalEnv.FlushAllBufioWriters(); flushErr != nil {
				ev.globalEnv.ResetBufioWritersOnError()
				err = flushErr
			}
		} else {
			ev.globalEnv.ResetBufioWritersOnError()
		}
	}()

	var lastVal Value = &TupleValue{Elements: []Value{}}
	for _, stmt := range prog.Body {
		v, e := ev.globalEnv.evalStmt(stmt)
		if e != nil {
			return nil, e
		}
		if v != nil {
			lastVal = v
		}
	}
	return lastVal, nil
}

func (env *Environment) isForceDemand(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.ForceExpr:
		return true
	}
	return false
}

func evalLiteral(expr ast.Expr) (Value, bool) {
	if expr == nil {
		return nil, false
	}
	switch lit := expr.(type) {
	case *ast.IntLit:
		return IntValue(lit.Value), true
	case *ast.FloatLit:
		return FloatValue(lit.Value), true
	case *ast.StringLit:
		return StringValue(lit.Value), true
	case *ast.BoolLit:
		return BoolValue(lit.Value), true
	case *ast.NilLit:
		return NilValue{}, true
	case *ast.UnaryExpr:
		if lit.Op == "-" {
			if intLit, ok := lit.Expr.(*ast.IntLit); ok {
				return IntValue(-intLit.Value), true
			}
			if floatLit, ok := lit.Expr.(*ast.FloatLit); ok {
				return FloatValue(-floatLit.Value), true
			}
		}
		return nil, false
	default:
		return nil, false
	}
}

func (env *Environment) newThunk(expr ast.Expr) *Thunk {
	thunk := NewThunk(expr, env)
	if expr != nil && env != nil && env.Inferencer != nil {
		if inf, ok := env.Inferencer.(*types.Inferencer); ok && inf.ExprTypes != nil {
			if t, ok := inf.ExprTypes[expr]; ok {
				thunk.InferredType = t
			}
		}
	}
	return thunk
}

func (env *Environment) evalStmt(stmt ast.Stmt) (Value, error) {
	switch s := stmt.(type) {
	case *ast.VarDecl:
		if _, ok := env.store[s.Name]; ok {
			// Re-binding existing variable in current environment:
			// Force RHS immediately to avoid circular reference in lazy thunk
			newVal := Force(env.newThunk(s.Value))
			env.store[s.Name] = newVal
			return nil, nil
		}
		// If RHS is a constant literal, evaluate immediately without lazy thunk!
		if val, ok := evalLiteral(s.Value); ok {
			env.Set(s.Name, val)
			return nil, nil
		}
		// If RHS is a force demand point, force immediately instead of creating lazy thunk!
		if env.isForceDemand(s.Value) {
			val := Force(env.newThunk(s.Value))
			env.Set(s.Name, val)
			return nil, nil
		}
		// Wrap RHS in lazy Thunk!
		thunk := env.newThunk(s.Value)
		env.Set(s.Name, thunk)
		return nil, nil

	case *ast.UnpackDecl:
		val := Force(NewThunk(s.Value, env))
		tVal, ok := val.(*TupleValue)
		if !ok {
			return nil, fmt.Errorf("cannot unpack non-tuple value: %v", val)
		}
		reqCount := len(s.Pattern.Elements)
		if s.Pattern.RestName != "" {
			if len(tVal.Elements) < reqCount {
				return nil, fmt.Errorf("not enough elements to unpack into tuple pattern: got %d, need at least %d", len(tVal.Elements), reqCount)
			}
		} else {
			if len(tVal.Elements) != reqCount {
				return nil, fmt.Errorf("element count mismatch in unpack: expected %d, got %d", reqCount, len(tVal.Elements))
			}
		}
		for i, name := range s.Pattern.Elements {
			if name != "_" {
				env.Set(name, tVal.Elements[i])
			}
		}
		if s.Pattern.RestName != "" && s.Pattern.RestName != "_" {
			restElements := tVal.Elements[reqCount:]
			env.Set(s.Pattern.RestName, &TupleValue{Elements: restElements})
		}
		return nil, nil

	case *ast.FuncDef:
		closure := &Closure{
			Params:     s.Params,
			BlockParam: s.BlockParam,
			Body:       s.Body,
			Env:        env,
			IsFuncDef:  true,
		}
		if env != nil && env.Inferencer != nil {
			if inf, ok := env.Inferencer.(*types.Inferencer); ok {
				if inf.StmtTypes != nil {
					if fnType, ok := inf.StmtTypes[s]; ok {
						if ft, isFT := fnType.(*types.FuncType); isFT {
							closure.FuncType = ft
						}
					}
				}
				if closure.FuncType == nil {
					if fnType, ok := inf.Scope().Lookup(s.Name); ok {
						if ft, isFT := fnType.(*types.FuncType); isFT {
							closure.FuncType = ft
						}
					}
				}
			}
		}
		env.Set(s.Name, closure)
		return nil, nil

	case *ast.StructDef:
		sDef := &StructDefValue{
			Name:          s.Name,
			FieldOrder:    make([]string, 0),
			FieldDefaults: make(map[string]ast.Expr),
			EmbeddedNames: make([]string, 0),
			Methods:       make(map[string]*ast.FuncDef),
			Env:           env,
		}

		for _, f := range s.Fields {
			if f.IsEmbedded {
				sDef.EmbeddedNames = append(sDef.EmbeddedNames, f.Name)
				sDef.FieldOrder = append(sDef.FieldOrder, f.Name)
			} else {
				sDef.FieldOrder = append(sDef.FieldOrder, f.Name)
				if f.DefaultValue != nil {
					sDef.FieldDefaults[f.Name] = f.DefaultValue
				}
			}
		}

		for _, m := range s.Methods {
			sDef.Methods[m.Name] = m
		}

		if env != nil && env.Inferencer != nil {
			if inf, ok := env.Inferencer.(*types.Inferencer); ok {
				if inf.StmtTypes != nil {
					if st, ok := inf.StmtTypes[s]; ok {
						if structT, isST := st.(*types.StructType); isST {
							sDef.StructType = structT
						}
					}
				}
				if sDef.StructType == nil {
					if st, ok := inf.Scope().Lookup(s.Name); ok {
						if structT, isST := st.(*types.StructType); isST {
							sDef.StructType = structT
						}
					}
				}
			}
		}

		env.Set(s.Name, sDef)
		return nil, nil

	case *ast.InterfaceDef:
		iDef := &InterfaceDefValue{
			Name:     s.Name,
			Embedded: s.Embedded,
			Methods:  make(map[string]ast.MethodSig),
		}
		for _, m := range s.Methods {
			iDef.Methods[m.Name] = m
		}
		env.Set(s.Name, iDef)
		return nil, nil

	case *ast.MemberAssignStmt:
		targetVal := Force(NewThunk(s.Target, env))
		si, ok := targetVal.(*StructInstance)
		if !ok {
			return nil, fmt.Errorf("cannot assign member '%s' on non-struct %s", s.Member, targetVal.String())
		}
		if !si.IsInitializing {
			return nil, fmt.Errorf("cannot mutate field '%s' of immutable struct '%s'", s.Member, si.Def.Name)
		}
		found := false
		for _, fName := range si.Def.FieldOrder {
			if fName == s.Member {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("struct '%s' has no field '%s'", si.Def.Name, s.Member)
		}
		val, err := env.evalExpr(s.Value)
		if err != nil {
			return nil, err
		}
		si.Fields[s.Member] = val
		return nil, nil

	case *ast.ReturnStmt:
		var retVal Value = &TupleValue{Elements: []Value{}}
		if s.Value != nil {
			v, err := env.evalExpr(s.Value)
			if err != nil {
				return nil, err
			}
			retVal = v
		}

		if env.EnclosingFuncID == 0 {
			// Top-level return behaves like exit()
			code := 0
			forced := Force(retVal)
			switch v := forced.(type) {
			case *TupleValue:
				if len(v.Elements) == 0 {
					code = 0
				} else {
					return nil, fmt.Errorf("return at top level expects Int or Bool, got %v", forced)
				}
			case IntValue:
				code = int(v)
			case BoolValue:
				if bool(v) {
					code = 0
				} else {
					code = 1
				}
			default:
				return nil, fmt.Errorf("return at top level expects Int or Bool, got %v", forced)
			}
			panic(&ExitError{Code: code})
		}

		panic(&ReturnSignal{
			FuncID: env.EnclosingFuncID,
			Value:  retVal,
		})

	case *ast.BreakStmt:
		var brkVal Value = &TupleValue{Elements: []Value{}}
		if s.Value != nil {
			v, err := env.evalExpr(s.Value)
			if err != nil {
				return nil, err
			}
			brkVal = v
		}

		if env.ActiveLoopID != 0 {
			panic(&BreakSignal{
				LoopID: env.ActiveLoopID,
				Value:  brkVal,
			})
		}
		if env.ActiveClosureID != 0 {
			panic(&BreakSignal{
				ClosureID: env.ActiveClosureID,
				Value:     brkVal,
			})
		}
		return nil, fmt.Errorf("break outside of closure or loop")

	case *ast.ContinueStmt:
		if env.ActiveLoopID != 0 {
			panic(&ContinueSignal{
				LoopID: env.ActiveLoopID,
			})
		}
		if env.ActiveClosureID != 0 {
			panic(&ContinueSignal{
				ClosureID: env.ActiveClosureID,
			})
		}
		return nil, fmt.Errorf("continue outside of loop")

	case *ast.ForStmt:
		return env.evalForStmt(s)

	case *ast.IfStmt:
		condVal := Force(NewThunk(s.Cond, env))
		bVal, ok := condVal.(BoolValue)
		if !ok {
			return nil, fmt.Errorf("condition must be Bool, got %v", condVal)
		}
		if bool(bVal) {
			return env.evalBlock(s.ThenBody)
		} else if len(s.ElseBody) > 0 {
			return env.evalBlock(s.ElseBody)
		}
		return nil, nil

	case *ast.ExprStmt:
		val, err := env.evalExpr(s.Value)
		if err != nil {
			return nil, err
		}
		// If the statement is a bare identifier/member resolving to a function,
		// auto-invoke it with 0 arguments (e.g. bare 'puts' for newline).
		isCallableTarget := false
		switch s.Value.(type) {
		case *ast.Identifier, *ast.ModuleAccessExpr:
			isCallableTarget = true
		}
		if isCallableTarget {
			forced := Force(val)
			switch fn := forced.(type) {
			case *BuiltinFunc:
				return fn.Fn([]Value{})
			case *Closure:
				return env.invokeClosure(fn, []Value{})
			}
		}
		return val, nil

	case *ast.ModuleDecl:
		return nil, nil

	case *ast.ImportStmt:
		alias := s.Alias
		if existingMod, ok := env.GetModule(s.Path); ok {
			if alias != "" && alias != existingMod.Name {
				aliasedMod := &ModuleValue{
					Name:    alias,
					Exports: existingMod.Exports,
				}
				env.RegisterModule(aliasedMod)
			}
			return nil, nil
		}
		if env.Loader == nil {
			return nil, fmt.Errorf("module loader not configured")
		}
		modVal, err := env.Loader(s.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to import module '%s': %w", s.Path, err)
		}
		canonicalName := modVal.Name
		if alias == "" {
			alias = modVal.Name
		}
		// Register module with alias name
		modVal.Name = alias
		env.RegisterModule(modVal)
		if s.Path != alias {
			env.RegisterModule(&ModuleValue{Name: s.Path, Exports: modVal.Exports})
		}
		if canonicalName != "" && canonicalName != alias && canonicalName != s.Path {
			env.RegisterModule(&ModuleValue{Name: canonicalName, Exports: modVal.Exports})
		}
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown statement: %T", stmt)
	}
}

func (env *Environment) evalBlock(body []ast.Stmt) (Value, error) {
	var lastVal Value = &TupleValue{Elements: []Value{}}
	for _, stmt := range body {
		v, err := env.evalStmt(stmt)
		if err != nil {
			return nil, err
		}
		if v != nil {
			lastVal = v
		}
	}
	return lastVal, nil
}

func (env *Environment) invokeClosure(fn *Closure, argValues []Value, optBlock ...Value) (val Value, err error) {
	callEnv := NewEnvironment(fn.Env)
	var myCallID uint64

	if fn.IsFuncDef {
		// Regular function call (def)
		myCallID = nextCallID()
		callEnv.EnclosingFuncID = myCallID
		callEnv.ActiveClosureID = 0
	} else {
		// Closure / lambda / block call
		myCallID = nextCallID()
		callEnv.EnclosingFuncID = fn.EnclosingFuncID
		callEnv.ActiveClosureID = myCallID
	}

	for i, p := range fn.Params {
		if p.IsVariadic {
			var rest []Value
			if i < len(argValues) {
				rest = argValues[i:]
			}
			callEnv.Set(p.Name, &TupleValue{Elements: rest})
			break
		}
		if i < len(argValues) {
			callEnv.Set(p.Name, argValues[i])
		} else if p.DefaultValue != nil {
			callEnv.Set(p.Name, NewThunk(p.DefaultValue, callEnv))
		} else {
			callEnv.Set(p.Name, &TupleValue{Elements: []Value{}})
		}
	}

	if fn.BlockParam != nil {
		var blockVal Value
		if len(optBlock) > 0 && optBlock[0] != nil {
			blockVal = optBlock[0]
		}
		if blockVal != nil {
			callEnv.Set(fn.BlockParam.Name, blockVal)
		} else if fn.BlockParam.DefaultValue != nil {
			callEnv.Set(fn.BlockParam.Name, NewThunk(fn.BlockParam.DefaultValue, callEnv))
		} else {
			return nil, fmt.Errorf("missing required block argument &%s", fn.BlockParam.Name)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			if fn.IsFuncDef {
				if ret, ok := r.(*ReturnSignal); ok {
					if ret.FuncID == myCallID {
						val = ret.Value
						err = nil
						return
					}
					panic(ret)
				}
			} else {
				if brk, ok := r.(*BreakSignal); ok {
					if brk.ClosureID == myCallID {
						val = brk.Value
						err = nil
						return
					}
					panic(brk)
				}
				if cont, ok := r.(*ContinueSignal); ok {
					if cont.ClosureID == myCallID {
						val = &TupleValue{Elements: []Value{}}
						err = nil
						return
					}
					panic(cont)
				}
			}
			panic(r)
		}
	}()

	return callEnv.evalBlock(fn.Body)
}

func (env *Environment) evalForStmt(s *ast.ForStmt) (Value, error) {
	loopID := nextCallID()
	prevLoopID := env.ActiveLoopID
	env.ActiveLoopID = loopID
	defer func() {
		env.ActiveLoopID = prevLoopID
	}()

	var breakVal Value = &TupleValue{Elements: []Value{}}

	for {
		if s.Cond != nil {
			condVal := Force(NewThunk(s.Cond, env))
			bVal, ok := condVal.(BoolValue)
			if !ok {
				return nil, fmt.Errorf("for condition must be Bool, got %v", condVal)
			}
			if !bool(bVal) {
				break
			}
		}

		var broke bool
		var loopErr error

		func() {
			defer func() {
				if r := recover(); r != nil {
					if brk, ok := r.(*BreakSignal); ok {
						if brk.LoopID == loopID || brk.ClosureID == loopID {
							broke = true
							if brk.Value != nil {
								breakVal = brk.Value
							}
							return
						}
					}
					if cont, ok := r.(*ContinueSignal); ok {
						if cont.LoopID == loopID || cont.ClosureID == loopID {
							return // continue to next iteration
						}
					}
					// propagate return / outer break / panic
					panic(r)
				}
			}()

			for _, stmt := range s.Body {
				if _, err := env.evalStmt(stmt); err != nil {
					loopErr = err
					return
				}
			}
		}()

		if loopErr != nil {
			return nil, loopErr
		}
		if broke {
			break
		}
	}

	return breakVal, nil
}

func (env *Environment) evalExpr(expr ast.Expr) (Value, error) {
	if expr == nil {
		return &TupleValue{Elements: []Value{}}, nil
	}

	switch e := expr.(type) {
	case *ast.IntLit:
		return IntValue(e.Value), nil
	case *ast.FloatLit:
		return FloatValue(e.Value), nil
	case *ast.StringLit:
		return StringValue(e.Value), nil
	case *ast.BoolLit:
		return BoolValue(e.Value), nil
	case *ast.NilLit:
		return NilValue{}, nil
	case *ast.ForceExpr:
		return Force(NewThunk(e.Expr, env)), nil

	case *ast.Identifier:
		v, ok := env.Get(e.Name)
		if !ok {
			return nil, fmt.Errorf("undefined identifier: %s", e.Name)
		}
		return v, nil

	case *ast.TupleLit:
		// Lazy tuple elements: wrapped in Thunks unless force-demanded or constant literal
		elems := make([]Value, len(e.Elements))
		for i, el := range e.Elements {
			if env.isForceDemand(el) {
				elems[i] = Force(env.newThunk(el))
			} else if val, ok := evalLiteral(el); ok {
				elems[i] = val
			} else {
				elems[i] = env.newThunk(el)
			}
		}
		return &TupleValue{Elements: elems}, nil

	case *ast.DictLit:
		entries := make(map[DictKey]Value)
		keyValues := make(map[DictKey]Value)
		for _, entry := range e.Entries {
			kVal := Force(env.newThunk(entry.Key))
			dk := MakeDictKey(kVal)
			keyValues[dk] = kVal
			if env.isForceDemand(entry.Value) {
				entries[dk] = Force(env.newThunk(entry.Value))
			} else if val, ok := evalLiteral(entry.Value); ok {
				entries[dk] = val
			} else {
				entries[dk] = env.newThunk(entry.Value)
			}
		}
		return NewDictValue(entries, keyValues), nil

	case *ast.IndexExpr:
		targetVal := Force(NewThunk(e.Target, env))
		idxVal := Force(NewThunk(e.Index, env))

		switch t := targetVal.(type) {
		case *TupleValue:
			i, ok := idxVal.(IntValue)
			if !ok {
				return nil, fmt.Errorf("tuple index must be Int, got %v", idxVal)
			}
			idx := int(i)
			if idx < 0 {
				idx = len(t.Elements) + idx
			}
			if idx < 0 || idx >= len(t.Elements) {
				return nil, fmt.Errorf("tuple index out of bounds: %d (len %d)", int(i), len(t.Elements))
			}
			return t.Elements[idx], nil
		case StringValue:
			i, ok := idxVal.(IntValue)
			if !ok {
				return nil, fmt.Errorf("string index must be Int, got %v", idxVal)
			}
			runes := []rune(string(t))
			idx := int(i)
			if idx < 0 {
				idx = len(runes) + idx
			}
			if idx < 0 || idx >= len(runes) {
				return nil, fmt.Errorf("string index out of bounds: %d (len %d)", int(i), len(runes))
			}
			return StringValue(string(runes[idx])), nil
		case BytesValue:
			i, ok := idxVal.(IntValue)
			if !ok {
				return nil, fmt.Errorf("bytes index must be Int, got %v", idxVal)
			}
			data := []byte(t)
			idx := int(i)
			if idx < 0 {
				idx = len(data) + idx
			}
			if idx < 0 || idx >= len(data) {
				return nil, fmt.Errorf("bytes index out of bounds: %d (len %d)", int(i), len(data))
			}
			return IntValue(data[idx]), nil
		case *DictValue:
			dk := MakeDictKey(idxVal)
			if v, ok := t.Entries[dk]; ok {
				return v, nil
			}
			return nil, fmt.Errorf("key not found in dict: %s", idxVal.String())
		default:
			return nil, fmt.Errorf("cannot index non-tuple/non-string/non-bytes/non-dict value: %v", targetVal)
		}

	case *ast.BinaryExpr:
		if e.Op == "&&" {
			leftVal := Force(NewThunk(e.Left, env))
			lb, ok := leftVal.(BoolValue)
			if !ok {
				return nil, fmt.Errorf("operator && expects Bool, got %v", leftVal)
			}
			if !bool(lb) {
				return BoolValue(false), nil
			}
			rightVal := Force(NewThunk(e.Right, env))
			rb, ok := rightVal.(BoolValue)
			if !ok {
				return nil, fmt.Errorf("operator && expects Bool, got %v", rightVal)
			}
			return BoolValue(bool(rb)), nil
		}
		if e.Op == "||" {
			leftVal := Force(NewThunk(e.Left, env))
			lb, ok := leftVal.(BoolValue)
			if !ok {
				return nil, fmt.Errorf("operator || expects Bool, got %v", leftVal)
			}
			if bool(lb) {
				return BoolValue(true), nil
			}
			rightVal := Force(NewThunk(e.Right, env))
			rb, ok := rightVal.(BoolValue)
			if !ok {
				return nil, fmt.Errorf("operator || expects Bool, got %v", rightVal)
			}
			return BoolValue(bool(rb)), nil
		}
		leftVal := Force(NewThunk(e.Left, env))
		rightVal := Force(NewThunk(e.Right, env))
		return evalBinary(e.Op, leftVal, rightVal)

	case *ast.UnaryExpr:
		val := Force(NewThunk(e.Expr, env))
		switch e.Op {
		case "+":
			if iv, ok := val.(IntValue); ok {
				return iv, nil
			}
			if fv, ok := val.(FloatValue); ok {
				return fv, nil
			}
			return nil, fmt.Errorf("unary + expects number, got %v", val)
		case "-":
			if iv, ok := val.(IntValue); ok {
				return -iv, nil
			}
			if fv, ok := val.(FloatValue); ok {
				return -fv, nil
			}
			return nil, fmt.Errorf("unary - expects number, got %v", val)
		case "!":
			if bv, ok := val.(BoolValue); ok {
				return !bv, nil
			}
			return nil, fmt.Errorf("unary ! expects Bool, got %v", val)
		default:
			return nil, fmt.Errorf("unknown unary operator %s", e.Op)
		}

	case *ast.TernaryExpr:
		condVal := Force(NewThunk(e.Cond, env))
		bVal, ok := condVal.(BoolValue)
		if !ok {
			return nil, fmt.Errorf("ternary condition must be Bool, got %v", condVal)
		}
		if bool(bVal) {
			return env.evalExpr(e.Then)
		}
		return env.evalExpr(e.Else)

	case *ast.CallExpr:
		calleeVal := Force(NewThunk(e.Callee, env))

		// Prepare argument thunks (pass-by-need) unless force-demanded or constant literal
		argValues := make([]Value, len(e.Args))
		for i, a := range e.Args {
			if env.isForceDemand(a) {
				argValues[i] = Force(env.newThunk(a))
			} else if val, ok := evalLiteral(a); ok {
				argValues[i] = val
			} else {
				argValues[i] = env.newThunk(a)
			}
		}

		// Trailing block closure
		var blockClosure *Closure
		if e.Block != nil {
			blockClosure = &Closure{
				Params:          e.Block.Params,
				Body:            e.Block.Body,
				Env:             env,
				IsFuncDef:       false,
				EnclosingFuncID: env.EnclosingFuncID,
			}
			if env != nil && env.Inferencer != nil {
				if inf, ok := env.Inferencer.(*types.Inferencer); ok && inf.ExprTypes != nil {
					if fnType, ok := inf.ExprTypes[e.Block]; ok {
						blockClosure.FuncType = fnType
					}
				}
			}
		}

		switch fn := calleeVal.(type) {
		case *BuiltinFunc:
			if blockClosure != nil {
				argValues = append(argValues, blockClosure)
			}
			return fn.Fn(argValues)

		case *Closure:
			if fn.BlockParam != nil {
				if blockClosure != nil {
					return env.invokeClosure(fn, argValues, blockClosure)
				}
				return env.invokeClosure(fn, argValues)
			}
			if blockClosure != nil {
				argValues = append(argValues, blockClosure)
			}
			return env.invokeClosure(fn, argValues)

		case *StructDefValue:
			return env.instantiateStruct(fn, e.Args)

		default:
			return nil, fmt.Errorf("cannot call non-function value: %v", calleeVal)
		}

	case *ast.MemberAccessExpr:
		targetVal := Force(NewThunk(e.Target, env))
		if si, ok := targetVal.(*StructInstance); ok {
			memVal, err := si.LookupMember(e.Member)
			if err == nil {
				if closure, ok := memVal.(*Closure); ok && closure.FuncType == nil {
					if si.Def != nil && si.Def.StructType != nil {
						if st, ok := si.Def.StructType.(*types.StructType); ok {
							if mType, err := st.LookupMember(e.Member); err == nil {
								if ft, isFT := mType.(*types.FuncType); isFT {
									closure.FuncType = ft
								}
							}
						}
					}
				}
			}
			return memVal, err
		}
		return lookupBuiltinMember(targetVal, e.Member)

	case *ast.NamedArgExpr:
		return env.evalExpr(e.Value)

	case *ast.ModuleAccessExpr:
		mod, ok := env.GetModule(e.Module)
		if !ok {
			return nil, fmt.Errorf("module '@%s' not found or not imported", e.Module)
		}
		// Enforce Go-style visibility: member must start with Uppercase letter to be public
		firstChar := e.Member[0]
		if firstChar < 'A' || firstChar > 'Z' {
			return nil, fmt.Errorf("cannot access private member '%s' from module '@%s'", e.Member, e.Module)
		}
		val, ok := mod.Exports[e.Member]
		if !ok {
			return nil, fmt.Errorf("member '%s' not exported by module '@%s'", e.Member, e.Module)
		}
		return val, nil

	case *ast.LambdaExpr:
		closure := &Closure{
			Params:          e.Params,
			Body:            e.Body,
			Env:             env,
			IsFuncDef:       false,
			EnclosingFuncID: env.EnclosingFuncID,
		}
		if env != nil && env.Inferencer != nil {
			if inf, ok := env.Inferencer.(*types.Inferencer); ok && inf.ExprTypes != nil {
				if fnType, ok := inf.ExprTypes[e]; ok {
					closure.FuncType = fnType
				}
			}
		}
		return closure, nil

	case *ast.IsExpr:
		val := Force(NewThunk(e.Expr, env))
		matches := checkValueMatchesType(val, strings.TrimSpace(e.TypeAnnot), env)
		return BoolValue(matches), nil

	case *ast.MatchExpr:
		val := Force(NewThunk(e.Target, env))
		for _, c := range e.Cases {
			if c.TuplePattern != nil {
				tVal, ok := val.(*TupleValue)
				if !ok {
					continue
				}
				reqCount := len(c.TuplePattern.Elements)
				if c.TuplePattern.RestName != "" {
					if len(tVal.Elements) < reqCount {
						continue
					}
				} else {
					if len(tVal.Elements) != reqCount {
						continue
					}
				}

				// Matched! Bind unpacked elements in a new scope
				caseEnv := NewEnvironment(env)
				for i, name := range c.TuplePattern.Elements {
					if name != "_" {
						caseEnv.Set(name, tVal.Elements[i])
					}
				}
				if c.TuplePattern.RestName != "" && c.TuplePattern.RestName != "_" {
					restElements := tVal.Elements[reqCount:]
					caseEnv.Set(c.TuplePattern.RestName, &TupleValue{Elements: restElements})
				}
				return caseEnv.evalBlock(c.Body)
			}

			pattern := strings.TrimSpace(c.Pattern)
			if pattern == "_" || checkValueMatchesType(val, pattern, env) {
				return env.evalBlock(c.Body)
			}
		}
		return &TupleValue{Elements: []Value{}}, nil

	default:
		return nil, fmt.Errorf("unknown expression: %T", expr)
	}
}

func (env *Environment) instantiateStruct(sDef *StructDefValue, args []ast.Expr) (*StructInstance, error) {
	fields := make(map[string]Value)
	embedded := make(map[string]*StructInstance)

	var baseInstance *StructInstance
	argStartIdx := 0

	// Check if 1st argument is a copy-update base instance (e.g. Person(p, x: 30))
	if len(args) > 0 {
		if _, isNamed := args[0].(*ast.NamedArgExpr); !isNamed {
			val := Force(NewThunk(args[0], env))
			if si, ok := val.(*StructInstance); ok && si.Def.Name == sDef.Name {
				baseInstance = si
				argStartIdx = 1
			}
		}
	}

	if baseInstance != nil {
		for k, v := range baseInstance.Fields {
			fields[k] = v
		}
		for k, v := range baseInstance.Embedded {
			embedded[k] = v
		}
		posIdx := 0
		for i := argStartIdx; i < len(args); i++ {
			arg := args[i]
			if named, ok := arg.(*ast.NamedArgExpr); ok {
				fields[named.Name] = NewThunk(named.Value, env)
			} else {
				if posIdx < len(sDef.FieldOrder) {
					fName := sDef.FieldOrder[posIdx]
					fields[fName] = NewThunk(arg, env)
					posIdx++
				}
			}
		}
		for _, embName := range sDef.EmbeddedNames {
			if fVal, ok := fields[embName]; ok {
				forced := Force(fVal)
				if si, ok := forced.(*StructInstance); ok {
					embedded[embName] = si
				}
			}
		}
		return &StructInstance{
			Def:      sDef,
			Fields:   fields,
			Embedded: embedded,
		}, nil
	}

	if initDef, hasInit := sDef.Methods["init"]; hasInit {
		return env.instantiateStructWithInit(sDef, initDef, args)
	}

	// Positional index for non-named arguments
	posIdx := 0

	for i := argStartIdx; i < len(args); i++ {
		arg := args[i]
		if named, ok := arg.(*ast.NamedArgExpr); ok {
			fields[named.Name] = NewThunk(named.Value, env)
		} else {
			// Positional argument
			if posIdx < len(sDef.FieldOrder) {
				fName := sDef.FieldOrder[posIdx]
				fields[fName] = NewThunk(arg, env)
				posIdx++
			}
		}
	}

	isEmbeddedField := make(map[string]bool)
	for _, embName := range sDef.EmbeddedNames {
		isEmbeddedField[embName] = true
	}

	// For any fields not yet populated:
	for _, fName := range sDef.FieldOrder {
		if isEmbeddedField[fName] {
			continue
		}
		if _, ok := fields[fName]; !ok {
			if defExpr, hasDef := sDef.FieldDefaults[fName]; hasDef {
				fields[fName] = NewThunk(defExpr, sDef.Env)
			} else {
				fields[fName] = &TupleValue{Elements: []Value{}}
			}
		}
	}

	// Populate embedded struct instances
	for _, embName := range sDef.EmbeddedNames {
		if fVal, ok := fields[embName]; ok {
			forced := Force(fVal)
			if si, ok := forced.(*StructInstance); ok {
				embedded[embName] = si
			}
		} else {
			// Auto-instantiate embedded struct if defined in environment
			if embDefVal, ok := env.Get(embName); ok {
				if embDef, ok := Force(embDefVal).(*StructDefValue); ok {
					subInst, err := env.instantiateStruct(embDef, []ast.Expr{})
					if err == nil {
						embedded[embName] = subInst
						fields[embName] = subInst
					}
				}
			}
		}
	}

	return &StructInstance{
		Def:      sDef,
		Fields:   fields,
		Embedded: embedded,
	}, nil
}

func (env *Environment) instantiateStructWithInit(sDef *StructDefValue, initDef *ast.FuncDef, args []ast.Expr) (*StructInstance, error) {
	fields := make(map[string]Value)
	embedded := make(map[string]*StructInstance)

	isEmbeddedField := make(map[string]bool)
	for _, embName := range sDef.EmbeddedNames {
		isEmbeddedField[embName] = true
	}

	// 1. Initialize fields with default values
	for _, fName := range sDef.FieldOrder {
		if isEmbeddedField[fName] {
			continue
		}
		if defExpr, hasDef := sDef.FieldDefaults[fName]; hasDef {
			fields[fName] = NewThunk(defExpr, sDef.Env)
		} else {
			fields[fName] = &TupleValue{Elements: []Value{}}
		}
	}

	// 2. Auto-instantiate embedded struct instances if defined in environment
	for _, embName := range sDef.EmbeddedNames {
		if embDefVal, ok := env.Get(embName); ok {
			if embDef, ok := Force(embDefVal).(*StructDefValue); ok {
				subInst, err := env.instantiateStruct(embDef, []ast.Expr{})
				if err == nil {
					embedded[embName] = subInst
					fields[embName] = subInst
				}
			}
		}
	}

	// 3. Create initializing instance
	inst := &StructInstance{
		Def:            sDef,
		Fields:         fields,
		Embedded:       embedded,
		IsInitializing: true,
	}

	// 4. Determine init parameter list (skip explicit 'self' if present)
	initParams := initDef.Params
	if len(initParams) > 0 && initParams[0].Name == "self" {
		initParams = initParams[1:]
	}

	// 5. Match arguments to init parameters and perform auto-binding for matching fields
	argValues := make([]Value, len(initParams))
	posIdx := 0

	namedArgs := make(map[string]ast.Expr)
	var posArgs []ast.Expr
	for _, arg := range args {
		if named, ok := arg.(*ast.NamedArgExpr); ok {
			namedArgs[named.Name] = named.Value
		} else {
			posArgs = append(posArgs, arg)
		}
	}

	for i, param := range initParams {
		if param.IsVariadic {
			var rest []Value
			for p := posIdx; p < len(posArgs); p++ {
				rest = append(rest, NewThunk(posArgs[p], env))
			}
			tup := &TupleValue{Elements: rest}
			argValues[i] = tup
			for _, fName := range sDef.FieldOrder {
				if fName == param.Name {
					fields[param.Name] = tup
					break
				}
			}
			break
		}

		var argExpr ast.Expr
		if expr, ok := namedArgs[param.Name]; ok {
			argExpr = expr
		} else if posIdx < len(posArgs) {
			argExpr = posArgs[posIdx]
			posIdx++
		}

		if argExpr != nil {
			var val Value
			if env.isForceDemand(argExpr) {
				val = Force(env.newThunk(argExpr))
			} else if lit, ok := evalLiteral(argExpr); ok {
				val = lit
			} else {
				val = env.newThunk(argExpr)
			}
			argValues[i] = val

			// Auto-bind: if param name matches struct field, assign to fields
			for _, fName := range sDef.FieldOrder {
				if fName == param.Name {
					fields[param.Name] = val
					break
				}
			}
		} else if param.DefaultValue != nil {
			val := env.newThunk(param.DefaultValue)
			argValues[i] = val
			for _, fName := range sDef.FieldOrder {
				if fName == param.Name {
					fields[param.Name] = val
					break
				}
			}
		} else {
			argValues[i] = &TupleValue{Elements: []Value{}}
		}
	}

	// Also check if any named args directly target fields that weren't in initParams
	for name, expr := range namedArgs {
		foundInParams := false
		for _, param := range initParams {
			if param.Name == name {
				foundInParams = true
				break
			}
		}
		if !foundInParams {
			for _, fName := range sDef.FieldOrder {
				if fName == name {
					fields[name] = NewThunk(expr, env)
					break
				}
			}
		}
	}

	// 6. Bind and invoke init
	closure := inst.bindMethod(initDef)
	_, err := env.invokeClosure(closure, argValues)
	if err != nil {
		return nil, err
	}

	// 7. Freeze instance
	inst.IsInitializing = false
	return inst, nil
}

func evalBinary(op string, left, right Value) (Value, error) {
	// Nil comparison
	_, leftIsNil := left.(NilValue)
	_, rightIsNil := right.(NilValue)
	if leftIsNil || rightIsNil {
		if op == "==" {
			return BoolValue(leftIsNil && rightIsNil), nil
		}
		if op == "!=" {
			return BoolValue(leftIsNil != rightIsNil), nil
		}
	}

	// Numeric operations
	if li, ok := left.(IntValue); ok {
		if ri, ok := right.(IntValue); ok {
			switch op {
			case "+":
				return li + ri, nil
			case "-":
				return li - ri, nil
			case "*":
				return li * ri, nil
			case "/":
				if ri == 0 {
					return nil, fmt.Errorf("division by zero")
				}
				return li / ri, nil
			case "%":
				return li % ri, nil
			case "==":
				return BoolValue(li == ri), nil
			case "!=":
				return BoolValue(li != ri), nil
			case "<":
				return BoolValue(li < ri), nil
			case "<=":
				return BoolValue(li <= ri), nil
			case ">":
				return BoolValue(li > ri), nil
			case ">=":
				return BoolValue(li >= ri), nil
			}
		}
	}

	// String concatenation & comparison
	if ls, ok := left.(StringValue); ok {
		if rs, ok := right.(StringValue); ok {
			switch op {
			case "+":
				return ls + rs, nil
			case "==":
				return BoolValue(ls == rs), nil
			case "!=":
				return BoolValue(ls != rs), nil
			}
		}
	}

	// Bytes concatenation & comparison
	if lb, ok := left.(BytesValue); ok {
		if rb, ok := right.(BytesValue); ok {
			switch op {
			case "+":
				return lb + rb, nil
			case "==":
				return BoolValue(lb == rb), nil
			case "!=":
				return BoolValue(lb != rb), nil
			case "<":
				return BoolValue(lb < rb), nil
			case "<=":
				return BoolValue(lb <= rb), nil
			case ">":
				return BoolValue(lb > rb), nil
			case ">=":
				return BoolValue(lb >= rb), nil
			}
		}
	}

	// Boolean operations
	if lb, ok := left.(BoolValue); ok {
		if rb, ok := right.(BoolValue); ok {
			switch op {
			case "&&":
				return BoolValue(bool(lb) && bool(rb)), nil
			case "||":
				return BoolValue(bool(lb) || bool(rb)), nil
			case "==":
				return BoolValue(lb == rb), nil
			case "!=":
				return BoolValue(lb != rb), nil
			}
		}
	}

	return nil, fmt.Errorf("unsupported binary operation %v %s %v", left, op, right)
}

func lookupBuiltinMember(val Value, member string) (Value, error) {
	if member == "ToString" {
		return &BuiltinFunc{
			Name: "ToString",
			FuncType: &types.FuncType{
				Arg: &types.TupleType{Elems: []types.Type{}},
				Ret: types.TypeString,
			},
			Fn: func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("ToString() takes 0 arguments, got %d", len(args))
				}
				return StringValue(val.String()), nil
			},
		}, nil
	}
	if member == "Type" {
		return &BuiltinFunc{
			Name: "Type",
			FuncType: &types.FuncType{
				Arg: &types.TupleType{Elems: []types.Type{}},
				Ret: types.TypeString,
			},
			Fn: func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("Type() takes 0 arguments, got %d", len(args))
				}
				return StringValue(typeNameOf(val)), nil
			},
		}, nil
	}
	if member == "Len" {
		switch v := val.(type) {
		case StringValue:
			return &BuiltinFunc{
				Name: "Len",
				FuncType: &types.FuncType{
					Arg: &types.TupleType{Elems: []types.Type{}},
					Ret: types.TypeInt,
				},
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Len() takes 0 arguments, got %d", len(args))
					}
					return IntValue(len([]rune(string(v)))), nil
				},
			}, nil
		case BytesValue:
			return &BuiltinFunc{
				Name: "Len",
				FuncType: &types.FuncType{
					Arg: &types.TupleType{Elems: []types.Type{}},
					Ret: types.TypeInt,
				},
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Len() takes 0 arguments, got %d", len(args))
					}
					return IntValue(len(v)), nil
				},
			}, nil
		case *TupleValue:
			return &BuiltinFunc{
				Name: "Len",
				FuncType: &types.FuncType{
					Arg: &types.TupleType{Elems: []types.Type{}},
					Ret: types.TypeInt,
				},
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Len() takes 0 arguments, got %d", len(args))
					}
					return IntValue(len(v.Elements)), nil
				},
			}, nil
		case *DictValue:
			return &BuiltinFunc{
				Name: "Len",
				FuncType: &types.FuncType{
					Arg: &types.TupleType{Elems: []types.Type{}},
					Ret: types.TypeInt,
				},
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Len() takes 0 arguments, got %d", len(args))
					}
					return IntValue(len(v.Entries)), nil
				},
			}, nil
		}
	}
	if member == "Ord" {
		if sv, ok := val.(StringValue); ok {
			bf := &BuiltinFunc{
				Name: "Ord",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Ord() takes 0 arguments, got %d", len(args))
					}
					s := string(sv)
					if len(s) == 0 {
						return nil, fmt.Errorf("Ord() on empty string")
					}
					r, size := utf8.DecodeRuneInString(s)
					if r == utf8.RuneError && size <= 1 {
						return nil, fmt.Errorf("Ord() invalid UTF-8 string")
					}
					return IntValue(r), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeString, "Ord"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	if member == "Chr" {
		if iv, ok := val.(IntValue); ok {
			bf := &BuiltinFunc{
				Name: "Chr",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Chr() takes 0 arguments, got %d", len(args))
					}
					n := int64(iv)
					if n < 0 || n > 0x10FFFF || (n >= 0xD800 && n <= 0xDFFF) {
						return nil, fmt.Errorf("Chr() code point %d out of valid Unicode range", n)
					}
					return StringValue(string(rune(n))), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeInt, "Chr"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	if member == "Bytes" {
		if sv, ok := val.(StringValue); ok {
			bf := &BuiltinFunc{
				Name: "Bytes",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Bytes() takes 0 arguments, got %d", len(args))
					}
					return BytesValue(string(sv)), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeString, "Bytes"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
		if bv, ok := val.(BytesValue); ok {
			bf := &BuiltinFunc{
				Name: "Bytes",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("Bytes() takes 0 arguments, got %d", len(args))
					}
					return bv, nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeBytes, "Bytes"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	if member == "String" {
		if bv, ok := val.(BytesValue); ok {
			bf := &BuiltinFunc{
				Name: "String",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("String() takes 0 arguments, got %d", len(args))
					}
					b := []byte(bv)
					if !utf8.Valid(b) {
						return nil, fmt.Errorf("invalid UTF-8 byte sequence in Bytes.String()")
					}
					return StringValue(string(bv)), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeBytes, "String"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
		if iv, ok := val.(IntValue); ok {
			bf := &BuiltinFunc{
				Name: "String",
				Fn: func(args []Value) (Value, error) {
					radix := 10
					if len(args) == 1 {
						rVal := Force(args[0])
						rv, ok := rVal.(IntValue)
						if !ok {
							return nil, fmt.Errorf("String() radix must be an integer, got %s", typeNameOf(rVal))
						}
						radix = int(rv)
					} else if len(args) > 1 {
						return nil, fmt.Errorf("String() takes at most 1 argument, got %d", len(args))
					}

					if radix != 2 && radix != 8 && radix != 10 && radix != 16 {
						return nil, fmt.Errorf("unsupported radix %d (supported: 2, 8, 10, 16)", radix)
					}

					return StringValue(strconv.FormatInt(int64(iv), radix)), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeInt, "String"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	if member == "Int" {
		if sv, ok := val.(StringValue); ok {
			bf := &BuiltinFunc{
				Name: "Int",
				Fn: func(args []Value) (Value, error) {
					radix := 10
					if len(args) == 1 {
						rVal := Force(args[0])
						iv, ok := rVal.(IntValue)
						if !ok {
							return nil, fmt.Errorf("Int() radix must be an integer, got %s", typeNameOf(rVal))
						}
						radix = int(iv)
					} else if len(args) > 1 {
						return nil, fmt.Errorf("Int() takes at most 1 argument, got %d", len(args))
					}

					if radix != 2 && radix != 8 && radix != 10 && radix != 16 {
						return nil, fmt.Errorf("unsupported radix %d (supported: 2, 8, 10, 16)", radix)
					}

					parsed := parseStringToInt(string(sv), radix)
					return IntValue(parsed), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeString, "Int"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	if member == "IsValidString" {
		if bv, ok := val.(BytesValue); ok {
			bf := &BuiltinFunc{
				Name: "IsValidString",
				Fn: func(args []Value) (Value, error) {
					if len(args) != 0 {
						return nil, fmt.Errorf("IsValidString() takes 0 arguments, got %d", len(args))
					}
					return BoolValue(utf8.Valid([]byte(bv))), nil
				},
			}
			if sig, ok := types.LookupBuiltinTypeMethod(types.TypeBytes, "IsValidString"); ok {
				bf.FuncType = sig
			}
			return bf, nil
		}
	}
	return nil, fmt.Errorf("cannot access member '%s' on %s value: %v", member, typeNameOf(val), val)
}

func isValidDigitForRadix(c byte, radix int) bool {
	switch radix {
	case 2:
		return c == '0' || c == '1'
	case 8:
		return c >= '0' && c <= '7'
	case 10:
		return c >= '0' && c <= '9'
	case 16:
		return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	default:
		return false
	}
}

func parseStringToInt(s string, radix int) int64 {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0
	}
	start := 0
	if s[0] == '+' || s[0] == '-' {
		start = 1
	}
	end := start
	for end < len(s) {
		c := s[end]
		if !isValidDigitForRadix(c, radix) {
			break
		}
		end++
	}
	if end == start {
		return 0
	}
	sub := s[:end]
	v, err := strconv.ParseInt(sub, radix, 64)
	if err != nil {
		if strings.Contains(err.Error(), "value out of range") {
			if s[0] == '-' {
				return math.MinInt64
			}
			return math.MaxInt64
		}
		return 0
	}
	return v
}

func checkValueMatchesType(val Value, typeAnnot string, optEnv ...*Environment) bool {
	typeAnnot = strings.TrimSpace(typeAnnot)
	if typeAnnot == "" {
		return true
	}
	if thunk, ok := val.(*Thunk); ok {
		if evaluated, isEval := PeekEvaluated(thunk); isEval {
			val = evaluated
		} else {
			return checkThunkInferredType(thunk, typeAnnot, optEnv...)
		}
	}
	// Check top-level union: A | B (respecting parens, brackets, braces)
	var unionParts []string
	pDepth := 0
	bDepth := 0
	brDepth := 0
	start := 0
	runes := []rune(typeAnnot)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
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
		} else if r == '|' && pDepth == 0 && bDepth == 0 && brDepth == 0 {
			unionParts = append(unionParts, string(runes[start:i]))
			start = i + 1
		}
	}
	if len(unionParts) > 0 {
		unionParts = append(unionParts, string(runes[start:]))
		for _, p := range unionParts {
			if checkValueMatchesType(val, strings.TrimSpace(p), optEnv...) {
				return true
			}
		}
		return false
	}

	switch typeAnnot {
	case "Nil", "nil":
		_, ok := val.(NilValue)
		return ok
	case "true":
		b, ok := val.(BoolValue)
		return ok && bool(b)
	case "false":
		b, ok := val.(BoolValue)
		return ok && !bool(b)
	case "Int":
		_, ok := val.(IntValue)
		return ok
	case "Float":
		_, ok := val.(FloatValue)
		return ok
	case "String":
		_, ok := val.(StringValue)
		return ok
	case "Bytes":
		_, ok := val.(BytesValue)
		return ok
	case "Bool":
		_, ok := val.(BoolValue)
		return ok
	case "Tuple":
		_, ok := val.(*TupleValue)
		return ok
	case "Dict":
		_, ok := val.(*DictValue)
		return ok
	case "Function":
		switch val.(type) {
		case *Closure, *BuiltinFunc:
			return true
		default:
			return false
		}
	default:
		switch fnVal := val.(type) {
		case *Closure:
			if fnT, ok := fnVal.FuncType.(*types.FuncType); ok {
				var targetType types.Type
				if len(optEnv) > 0 && optEnv[0] != nil && optEnv[0].Inferencer != nil {
					if inf, ok := optEnv[0].Inferencer.(*types.Inferencer); ok {
						targetType, _ = inf.ParseTypeAnnot(typeAnnot)
					}
				}
				if targetType == nil {
					targetType, _ = types.ParseTypeAnnot(typeAnnot)
				}
				if targetFT, ok := targetType.(*types.FuncType); ok {
					return types.IsAssignableTo(fnT, targetFT)
				}
			}
		case *BuiltinFunc:
			if fnT, ok := fnVal.FuncType.(*types.FuncType); ok {
				var targetType types.Type
				if len(optEnv) > 0 && optEnv[0] != nil && optEnv[0].Inferencer != nil {
					if inf, ok := optEnv[0].Inferencer.(*types.Inferencer); ok {
						targetType, _ = inf.ParseTypeAnnot(typeAnnot)
					}
				}
				if targetType == nil {
					targetType, _ = types.ParseTypeAnnot(typeAnnot)
				}
				if targetFT, ok := targetType.(*types.FuncType); ok {
					return types.IsAssignableTo(fnT, targetFT)
				}
			}
		}
		if strings.HasPrefix(typeAnnot, "{") && strings.HasSuffix(typeAnnot, "}") {
			dVal, ok := val.(*DictValue)
			if !ok {
				return false
			}
			inner := strings.TrimSpace(typeAnnot[1 : len(typeAnnot)-1])
			if inner == "" {
				return false
			}
			colonIdx := -1
			pD := 0
			bD := 0
			brD := 0
			innerRunes := []rune(inner)
			for i, r := range innerRunes {
				if r == '(' {
					pD++
				} else if r == ')' {
					pD--
				} else if r == '[' {
					bD++
				} else if r == ']' {
					bD--
				} else if r == '{' {
					brD++
				} else if r == '}' {
					brD--
				} else if r == ':' && pD == 0 && bD == 0 && brD == 0 {
					if colonIdx != -1 {
						return false
					}
					colonIdx = i
				}
			}
			if colonIdx == -1 {
				return false
			}
			keyAnnot := strings.TrimSpace(string(innerRunes[:colonIdx]))
			valAnnot := strings.TrimSpace(string(innerRunes[colonIdx+1:]))
			if keyAnnot == "" || valAnnot == "" {
				return false
			}

			for dk, v := range dVal.Entries {
				var kVal Value
				if dVal.KeyValues != nil {
					if actualKey, exists := dVal.KeyValues[dk]; exists {
						kVal = actualKey
					}
				}
				if kVal == nil {
					kVal = StringValue(dk.Key)
				}
				if !checkValueMatchesType(kVal, keyAnnot, optEnv...) {
					return false
				}
				// Check value:
				if evaluatedVal, ok := PeekEvaluated(v); ok {
					if !checkValueMatchesType(evaluatedVal, valAnnot, optEnv...) {
						return false
					}
				} else if thunk, ok := v.(*Thunk); ok {
					if !checkThunkInferredType(thunk, valAnnot, optEnv...) {
						return false
					}
				}
			}
			return true
		}
		// Check tuple type annotation: (T1, T2, ...) or (T, ...)
		if strings.HasPrefix(typeAnnot, "(") && strings.HasSuffix(typeAnnot, ")") {
			tVal, ok := val.(*TupleValue)
			if !ok {
				return false
			}
			inner := strings.TrimSpace(typeAnnot[1 : len(typeAnnot)-1])
			if inner == "" {
				return len(tVal.Elements) == 0
			}
			elemAnnots := splitTopLevelCommas(inner)
			isVariadic := false
			if len(elemAnnots) > 0 && elemAnnots[len(elemAnnots)-1] == "..." {
				isVariadic = true
				elemAnnots = elemAnnots[:len(elemAnnots)-1]
			}
			if !isVariadic && len(tVal.Elements) != len(elemAnnots) {
				return false
			}
			lastIdx := len(elemAnnots) - 1
			for i, el := range tVal.Elements {
				expectedAnnot := "Unknown"
				if lastIdx >= 0 {
					if i < len(elemAnnots) {
						expectedAnnot = elemAnnots[i]
					} else if isVariadic {
						expectedAnnot = elemAnnots[lastIdx]
					} else {
						return false
					}
				}
				if evaluatedVal, ok := PeekEvaluated(el); ok {
					if !checkValueMatchesType(evaluatedVal, expectedAnnot, optEnv...) {
						return false
					}
				} else if thunk, ok := el.(*Thunk); ok {
					if !checkThunkInferredType(thunk, expectedAnnot, optEnv...) {
						return false
					}
				}
			}
			return true
		}
		// Check struct type or embedded struct type
		if si, ok := val.(*StructInstance); ok {
			if si.Def.Name == typeAnnot {
				return true
			}
			for _, emb := range si.Embedded {
				if checkValueMatchesType(emb, typeAnnot, optEnv...) {
					return true
				}
			}
			// Check if typeAnnot is "Error"
			if typeAnnot == "Error" {
				if _, err := si.LookupMember("Message"); err == nil {
					return true
				}
				if _, err := si.LookupMember("Error"); err == nil {
					return true
				}
				return false
			}
		}

		var searchEnv *Environment
		if len(optEnv) > 0 && optEnv[0] != nil {
			searchEnv = optEnv[0]
		} else if si, ok := val.(*StructInstance); ok && si.Def != nil {
			searchEnv = si.Def.Env
		}

		if searchEnv != nil {
			// Check if typeAnnot is module qualified: @io.Reader or @bufio.Reader
			if strings.HasPrefix(typeAnnot, "@") && strings.Contains(typeAnnot, ".") {
				parts := strings.SplitN(typeAnnot[1:], ".", 2)
				modName := parts[0]
				typeName := parts[1]
				if modVal, found := searchEnv.GetModule(modName); found {
					if expVal, ok := modVal.Exports[typeName]; ok {
						forcedExp := Force(expVal)
						// If the exported item is a constructor BuiltinFunc with TypeDef, unwrap it
						if bf, isBF := forcedExp.(*BuiltinFunc); isBF && bf.TypeDef != nil {
							forcedExp = Force(bf.TypeDef)
						}
						if ifaceDef, isIface := forcedExp.(*InterfaceDefValue); isIface {
							return valueImplementsInterfaceValue(val, ifaceDef, searchEnv)
						}
						if structDef, isStruct := forcedExp.(*StructDefValue); isStruct {
							if si, ok := val.(*StructInstance); ok && si.Def != nil {
								if si.Def == structDef {
									return true
								}
								for _, emb := range si.Embedded {
									if emb.Def == structDef {
										return true
									}
								}
							}
						}
					}
				}
			}

			// Check if typeAnnot is an Interface in searchEnv
			if ifaceVal, found := searchEnv.Get(typeAnnot); found {
				if ifaceDef, isIface := Force(ifaceVal).(*InterfaceDefValue); isIface {
					return valueImplementsInterfaceValue(val, ifaceDef, searchEnv)
				}
			}
		}

		return false
	}
}

func checkThunkInferredType(thunk *Thunk, targetAnnot string, optEnv ...*Environment) bool {
	if thunk == nil || thunk.InferredType == nil {
		return true // unknown inferred type, preserve laziness
	}
	infT, ok := thunk.InferredType.(types.Type)
	if !ok || infT == nil || infT == types.TypeUnknown {
		return true // TypeUnknown could be anything at runtime, preserve laziness
	}

	targetAnnot = strings.TrimSpace(targetAnnot)
	if targetAnnot == "" || targetAnnot == "Unknown" {
		return true
	}

	var targetType types.Type
	var err error
	if len(optEnv) > 0 && optEnv[0] != nil && optEnv[0].Inferencer != nil {
		if inf, ok := optEnv[0].Inferencer.(*types.Inferencer); ok {
			targetType, err = inf.ParseTypeAnnot(targetAnnot)
		}
	}
	if targetType == nil {
		targetType, err = types.ParseTypeAnnot(targetAnnot)
	}
	if err != nil {
		return true // target annot could not be parsed statically, preserve laziness
	}

	// If the statically inferred type is not assignable to targetType,
	// it can never match at runtime: return false without forcing the thunk!
	if !types.IsAssignableTo(infT, targetType) {
		return false
	}
	return true
}

func splitTopLevelCommas(s string) []string {
	var parts []string
	pDepth := 0
	bDepth := 0
	brDepth := 0
	start := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
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
			parts = append(parts, strings.TrimSpace(string(runes[start:i])))
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(string(runes[start:])))
	var result []string
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func valueImplementsInterfaceValue(val Value, iface *InterfaceDefValue, env *Environment) bool {
	// Check all direct methods required by the interface
	for mName, sig := range iface.Methods {
		var memberVal Value
		var err error
		if si, ok := val.(*StructInstance); ok {
			memberVal, err = si.LookupMember(mName)
		} else {
			memberVal, err = lookupBuiltinMember(val, mName)
		}
		if err != nil {
			return false
		}
		forced := Force(memberVal)
		switch fn := forced.(type) {
		case *Closure:
			// Check param count (excluding self)
			nonSelfCount := 0
			for _, p := range fn.Params {
				if p.Name != "self" {
					nonSelfCount++
				}
			}
			if nonSelfCount != len(sig.Params) {
				return false
			}
		case *BuiltinFunc:
			if mName == "ToString" || mName == "Type" {
				if len(sig.Params) != 0 || (sig.ReturnType != "" && sig.ReturnType != "String") {
					return false
				}
			} else if mName == "Len" || mName == "Ord" {
				if len(sig.Params) != 0 || (sig.ReturnType != "" && sig.ReturnType != "Int") {
					return false
				}
			} else if mName == "Chr" {
				if len(sig.Params) != 0 || (sig.ReturnType != "" && sig.ReturnType != "String") {
					return false
				}
			}
		default:

			return false
		}
	}
	// Check embedded interfaces
	for _, embName := range iface.Embedded {
		var embDef *InterfaceDefValue
		if env != nil {
			// If iface belongs to a specific module, resolve embedded interface from that owning module first
			if iface.ModuleName != "" {
				if mod, found := env.GetModule(iface.ModuleName); found {
					if expVal, exists := mod.Exports[embName]; exists {
						if ed, isIface := Force(expVal).(*InterfaceDefValue); isIface {
							embDef = ed
						}
					}
				}
			}
			if embDef == nil {
				if embVal, found := env.Get(embName); found {
					if ed, isIface := Force(embVal).(*InterfaceDefValue); isIface {
						embDef = ed
					}
				}
			}
			if embDef == nil {
				// Try finding in imported modules across environment hierarchy
				for cur := env; cur != nil; cur = cur.parent {
					for _, mod := range cur.modules {
						if expVal, exists := mod.Exports[embName]; exists {
							if ed, isIface := Force(expVal).(*InterfaceDefValue); isIface {
								embDef = ed
								break
							}
						}
					}
					if embDef != nil {
						break
					}
				}
			}
		}
		if embDef == nil || !valueImplementsInterfaceValue(val, embDef, env) {
			return false
		}
	}
	return true
}

func structImplementsInterfaceValue(si *StructInstance, iface *InterfaceDefValue) bool {
	var env *Environment
	if si.Def != nil {
		env = si.Def.Env
	}
	return valueImplementsInterfaceValue(si, iface, env)
}
