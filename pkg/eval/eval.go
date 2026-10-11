package eval

import (
	"fmt"
	"strings"

	"les/pkg/ast"
)

// Evaluator runs Les AST nodes.
type Evaluator struct {
	globalEnv *Environment
}

func NewEvaluator() *Evaluator {
	env := NewEnvironment(nil)
	ev := &Evaluator{globalEnv: env}
	ev.registerBuiltins()
	return ev
}

func (ev *Evaluator) SetOutput(sb *strings.Builder) {
	ev.globalEnv.Output = sb
}

func (ev *Evaluator) SetModuleLoader(loader func(path string) (*ModuleValue, error)) {
	ev.globalEnv.Loader = loader
}

func (ev *Evaluator) registerBuiltins() {
	// print: built-in effect boundary. Demands evaluation of its arguments without automatic newline.
	ev.globalEnv.Set("print", &BuiltinFunc{
		Name: "print",
		Fn: func(args []Value) (Value, error) {
			parts := make([]string, len(args))
			for i, a := range args {
				val := Force(a)
				parts[i] = val.String()
			}
			out := strings.Join(parts, "")
			if ev.globalEnv.Output != nil {
				ev.globalEnv.Output.WriteString(out)
			} else {
				fmt.Print(out)
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	})

	// type: returns string representation of the runtime type of the value(s).
	ev.globalEnv.Set("type", &BuiltinFunc{
		Name: "type",
		Fn: func(args []Value) (Value, error) {
			if len(args) == 0 {
				return StringValue("()"), nil
			}
			if len(args) == 1 {
				val := Force(args[0])
				return StringValue(typeNameOf(val)), nil
			}
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = typeNameOf(Force(a))
			}
			return StringValue("(" + strings.Join(parts, ", ") + ")"), nil
		},
	})

	// len: returns integer length of tuple, string, or dict.
	ev.globalEnv.Set("len", &BuiltinFunc{
		Name: "len",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("len expects 1 argument, got %d", len(args))
			}
			val := Force(args[0])
			switch v := val.(type) {
			case StringValue:
				return IntValue(len([]rune(string(v)))), nil
			case *TupleValue:
				return IntValue(len(v.Elements)), nil
			case *DictValue:
				return IntValue(len(v.Entries)), nil
			default:
				return nil, fmt.Errorf("len expects String, Tuple, or Dict, got %v", val)
			}
		},
	})
}

func typeNameOf(val Value) string {
	return typeNameWithDepth(val, 0)
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
	case BoolValue:
		return "Bool"
	case NilValue:
		return "Nil"
	case *TupleValue:
		parts := make([]string, len(v.Elements))
		for i, el := range v.Elements {
			parts[i] = typeNameWithDepth(Force(el), depth+1)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *DictValue:
		return "Dict"
	case *Closure, *BuiltinFunc:
		return "Function"
	default:
		return "Any"
	}
}

func (ev *Evaluator) EvalProgram(prog *ast.Program) (val Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
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

func (env *Environment) evalStmt(stmt ast.Stmt) (Value, error) {
	switch s := stmt.(type) {
	case *ast.VarDecl:
		// Wrap RHS in lazy Thunk!
		thunk := NewThunk(s.Value, env)
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
			Params: s.Params,
			Body:   s.Body,
			Env:    env,
		}
		env.Set(s.Name, closure)
		return nil, nil

	case *ast.ReturnStmt:
		if s.Value == nil {
			return &TupleValue{Elements: []Value{}}, nil
		}
		return NewThunk(s.Value, env), nil

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
		// Expression statement: if it's a CallExpr to print/builtin effect, it will force demand.
		// Otherwise, statements can remain unforced or evaluated.
		return env.evalExpr(s.Value)

	case *ast.ModuleDecl:
		return nil, nil

	case *ast.ImportStmt:
		if env.Loader == nil {
			return nil, fmt.Errorf("module loader not configured")
		}
		modVal, err := env.Loader(s.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to import module '%s': %w", s.Path, err)
		}
		alias := s.Alias
		if alias == "" {
			alias = modVal.Name
		}
		// Register module with alias name
		modVal.Name = alias
		env.RegisterModule(modVal)
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

	case *ast.Identifier:
		v, ok := env.Get(e.Name)
		if !ok {
			return nil, fmt.Errorf("undefined identifier: %s", e.Name)
		}
		return v, nil

	case *ast.TupleLit:
		// Lazy tuple elements: wrapped in Thunks
		elems := make([]Value, len(e.Elements))
		for i, el := range e.Elements {
			elems[i] = NewThunk(el, env)
		}
		return &TupleValue{Elements: elems}, nil

	case *ast.DictLit:
		entries := make(map[string]Value)
		for _, entry := range e.Entries {
			kVal := Force(NewThunk(entry.Key, env))
			entries[kVal.String()] = NewThunk(entry.Value, env)
		}
		return &DictValue{Entries: entries}, nil

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
		case *DictValue:
			k := idxVal.String()
			if v, ok := t.Entries[k]; ok {
				return v, nil
			}
			return nil, fmt.Errorf("key not found in dict: %s", k)
		default:
			return nil, fmt.Errorf("cannot index non-tuple/non-string/non-dict value: %v", targetVal)
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

		// Prepare argument thunks (pass-by-need)
		argValues := make([]Value, len(e.Args))
		for i, a := range e.Args {
			argValues[i] = NewThunk(a, env)
		}

		// Trailing block as closure argument
		if e.Block != nil {
			blockClosure := &Closure{
				Params: e.Block.Params,
				Body:   e.Block.Body,
				Env:    env,
			}
			argValues = append(argValues, blockClosure)
		}

		switch fn := calleeVal.(type) {
		case *BuiltinFunc:
			return fn.Fn(argValues)

		case *Closure:
			callEnv := NewEnvironment(fn.Env)
			// Bind parameters to lazy argument thunks
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
				} else {
					callEnv.Set(p.Name, &TupleValue{Elements: []Value{}})
				}
			}
			return callEnv.evalBlock(fn.Body)

		default:
			return nil, fmt.Errorf("cannot call non-function value: %v", calleeVal)
		}

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

	case *ast.IsExpr:
		val := Force(NewThunk(e.Expr, env))
		matches := checkValueMatchesType(val, strings.TrimSpace(e.TypeAnnot))
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
			if pattern == "_" || checkValueMatchesType(val, pattern) {
				return env.evalBlock(c.Body)
			}
		}
		return &TupleValue{Elements: []Value{}}, nil

	default:
		return nil, fmt.Errorf("unknown expression: %T", expr)
	}
}

func evalBinary(op string, left, right Value) (Value, error) {
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

func checkValueMatchesType(val Value, typeAnnot string) bool {
	typeAnnot = strings.TrimSpace(typeAnnot)
	if typeAnnot == "" || typeAnnot == "Any" {
		return true
	}
	// Check if union: A | B
	if strings.Contains(typeAnnot, "|") {
		parts := strings.Split(typeAnnot, "|")
		for _, p := range parts {
			if checkValueMatchesType(val, strings.TrimSpace(p)) {
				return true
			}
		}
		return false
	}

	switch typeAnnot {
	case "Any":
		return true
	case "Nil":
		_, ok := val.(NilValue)
		return ok
	case "Int":
		_, ok := val.(IntValue)
		return ok
	case "Float":
		_, ok := val.(FloatValue)
		return ok
	case "String":
		_, ok := val.(StringValue)
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
	default:
		// Unknown or custom type
		return true
	}
}

