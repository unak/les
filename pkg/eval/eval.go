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

func (ev *Evaluator) registerBuiltins() {
	// print: built-in effect boundary. Demands evaluation of its arguments.
	ev.globalEnv.Set("print", &BuiltinFunc{
		Name: "print",
		Fn: func(args []Value) (Value, error) {
			parts := make([]string, len(args))
			for i, a := range args {
				val := Force(a)
				parts[i] = val.String()
			}
			out := strings.Join(parts, " ")
			if ev.globalEnv.Output != nil {
				ev.globalEnv.Output.WriteString(out + "\n")
			} else {
				fmt.Println(out)
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	})
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
			if idx < 0 || idx >= len(t.Elements) {
				return nil, fmt.Errorf("tuple index out of bounds: %d (len %d)", idx, len(t.Elements))
			}
			return t.Elements[idx], nil
		case *DictValue:
			k := idxVal.String()
			if v, ok := t.Entries[k]; ok {
				return v, nil
			}
			return nil, fmt.Errorf("key not found in dict: %s", k)
		default:
			return nil, fmt.Errorf("cannot index non-tuple/non-dict value: %v", targetVal)
		}

	case *ast.BinaryExpr:
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
