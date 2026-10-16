package eval

import (
	"fmt"
	"io"

	"les/pkg/ast"
	"les/pkg/types"
)

// initIOModule registers the built-in @io module.
func (ev *Evaluator) initIOModule() {
	ioExports := make(map[string]Value)

	readFuncType := &types.FuncType{
		Arg: &types.TupleType{Elems: []types.Type{types.TypeInt}},
		Ret: types.NewUnionType(types.TypeBytes, types.TypeNil),
	}
	writeFuncType := &types.FuncType{
		Arg: &types.TupleType{Elems: []types.Type{types.TypeBytes}},
		Ret: types.TypeInt,
	}
	voidFuncType := &types.FuncType{
		Arg: &types.TupleType{Elems: []types.Type{}},
		Ret: &types.TupleType{Elems: []types.Type{}},
	}

	// Reader interface definition: Read(n: Int): Bytes | Nil
	readerIface := &InterfaceDefValue{
		Name:       "Reader",
		ModuleName: "io",
		Methods: map[string]ast.MethodSig{
			"Read": {
				Name:       "Read",
				Params:     []ast.FuncParam{{Name: "n", TypeAnnot: "Int"}},
				ReturnType: "Bytes | Nil",
			},
		},
	}
	ioExports["Reader"] = readerIface

	// Writer interface definition: Write(b: Bytes): Int, Flush(): ()
	writerIface := &InterfaceDefValue{
		Name:       "Writer",
		ModuleName: "io",
		Methods: map[string]ast.MethodSig{
			"Write": {
				Name:       "Write",
				Params:     []ast.FuncParam{{Name: "b", TypeAnnot: "Bytes"}},
				ReturnType: "Int",
			},
			"Flush": {
				Name:       "Flush",
				Params:     []ast.FuncParam{},
				ReturnType: "()",
			},
		},
	}
	ioExports["Writer"] = writerIface

	// Closer interface definition: Close(): ()
	closerIface := &InterfaceDefValue{
		Name:       "Closer",
		ModuleName: "io",
		Methods: map[string]ast.MethodSig{
			"Close": {
				Name:       "Close",
				Params:     []ast.FuncParam{},
				ReturnType: "()",
			},
		},
	}
	ioExports["Closer"] = closerIface

	// ReadCloser interface definition: Reader, Closer
	readCloserIface := &InterfaceDefValue{
		Name:       "ReadCloser",
		ModuleName: "io",
		Embedded:   []string{"Reader", "Closer"},
		Methods:    make(map[string]ast.MethodSig),
	}
	ioExports["ReadCloser"] = readCloserIface

	// WriteCloser interface definition: Writer, Closer
	writeCloserIface := &InterfaceDefValue{
		Name:       "WriteCloser",
		ModuleName: "io",
		Embedded:   []string{"Writer", "Closer"},
		Methods:    make(map[string]ast.MethodSig),
	}
	ioExports["WriteCloser"] = writeCloserIface

	// Stdin struct definition implementing Reader & Closer
	stdinStructDef := &StructDefValue{
		Name:          "StdinType",
		FieldOrder:    []string{},
		FieldDefaults: make(map[string]ast.Expr),
		EmbeddedNames: []string{},
		Methods: map[string]*ast.FuncDef{
			"Read": {
				Name: "Read",
				Params: []ast.FuncParam{
					{Name: "self"},
					{Name: "n", TypeAnnot: "Int"},
				},
				ReturnType: "String | Nil",
			},
			"Close": {
				Name:       "Close",
				Params:     []ast.FuncParam{{Name: "self"}},
				ReturnType: "()",
			},
		},
		Env: ev.globalEnv,
	}

	stdinInstance := &StructInstance{
		Def:      stdinStructDef,
		Fields:   make(map[string]Value),
		Embedded: make(map[string]*StructInstance),
	}

	stdinInstance.Fields["Read"] = &BuiltinFunc{
		Name:     "Read",
		FuncType: readFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("Read expects 1 argument, got %d", len(args))
			}
			forced := Force(args[0])
			iv, ok := forced.(IntValue)
			if !ok {
				return nil, fmt.Errorf("Read expects Int, got %s", typeNameOf(forced))
			}
			numBytes := int64(iv)
			if numBytes < 0 {
				return nil, fmt.Errorf("Read: negative byte count %d", numBytes)
			}
			if numBytes == 0 {
				return BytesValue(""), nil
			}
			return NewActionThunk(func() (Value, error) {
				r := ev.globalEnv.GetStdinReader()
				allocSize := numBytes
				if allocSize > 64*1024 {
					allocSize = 64 * 1024
				}
				buf := make([]byte, allocSize)
				n, err := r.Read(buf)
				if n > 0 {
					return BytesValue(string(buf[:n])), nil
				}
				if err == io.EOF || err == io.ErrNoProgress {
					return NilValue{}, nil
				}
				if err != nil {
					return nil, err
				}
				return BytesValue(""), nil
			}), nil
		},
	}
	stdinInstance.Fields["Close"] = &BuiltinFunc{
		Name:     "Close",
		FuncType: voidFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("Close expects 0 arguments, got %d", len(args))
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	}
	ioExports["Stdin"] = stdinInstance

	// Stdout struct definition implementing WriteCloser
	stdoutStructDef := &StructDefValue{
		Name:          "StdoutType",
		FieldOrder:    []string{},
		FieldDefaults: make(map[string]ast.Expr),
		EmbeddedNames: []string{},
		Methods: map[string]*ast.FuncDef{
			"Write": {
				Name: "Write",
				Params: []ast.FuncParam{
					{Name: "self"},
					{Name: "b", TypeAnnot: "Bytes"},
				},
				ReturnType: "Int",
			},
			"Close": {
				Name:       "Close",
				Params:     []ast.FuncParam{{Name: "self"}},
				ReturnType: "()",
			},
			"Flush": {
				Name:       "Flush",
				Params:     []ast.FuncParam{{Name: "self"}},
				ReturnType: "()",
			},
		},
		Env: ev.globalEnv,
	}

	stdoutInstance := &StructInstance{
		Def:      stdoutStructDef,
		Fields:   make(map[string]Value),
		Embedded: make(map[string]*StructInstance),
	}
	stdoutInstance.Fields["Write"] = &BuiltinFunc{
		Name:     "Write",
		FuncType: writeFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("Write expects 1 argument, got %d", len(args))
			}
			forced := Force(args[0])
			bv, ok := forced.(BytesValue)
			if !ok {
				return nil, fmt.Errorf("Write expects Bytes, got %s", typeNameOf(forced))
			}
			// C/Ruby behavior: flush buffered stdout before writing directly
			if stdoutWriter := ev.globalEnv.GetStdoutWriter(); stdoutWriter != nil {
				if err := stdoutWriter.Flush(); err != nil {
					return nil, err
				}
			}
			w := ev.globalEnv.GetRawStdout()
			n, err := w.Write([]byte(string(bv)))
			if err != nil {
				return nil, err
			}
			return IntValue(n), nil
		},
	}
	stdoutInstance.Fields["Close"] = &BuiltinFunc{
		Name:     "Close",
		FuncType: voidFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("Close expects 0 arguments, got %d", len(args))
			}
			if stdoutWriter := ev.globalEnv.GetStdoutWriter(); stdoutWriter != nil {
				if err := stdoutWriter.Flush(); err != nil {
					return nil, err
				}
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	}
	stdoutInstance.Fields["Flush"] = &BuiltinFunc{
		Name:     "Flush",
		FuncType: voidFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("Flush expects 0 arguments, got %d", len(args))
			}
			if stdoutWriter := ev.globalEnv.GetStdoutWriter(); stdoutWriter != nil {
				if err := stdoutWriter.Flush(); err != nil {
					return nil, err
				}
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	}
	ioExports["Stdout"] = stdoutInstance

	// Stderr struct definition implementing WriteCloser (Unbuffered)
	stderrStructDef := &StructDefValue{
		Name:          "StderrType",
		FieldOrder:    []string{},
		FieldDefaults: make(map[string]ast.Expr),
		EmbeddedNames: []string{},
		Methods: map[string]*ast.FuncDef{
			"Write": {
				Name: "Write",
				Params: []ast.FuncParam{
					{Name: "self"},
					{Name: "b", TypeAnnot: "Bytes"},
				},
				ReturnType: "Int",
			},
			"Close": {
				Name:       "Close",
				Params:     []ast.FuncParam{{Name: "self"}},
				ReturnType: "()",
			},
			"Flush": {
				Name:       "Flush",
				Params:     []ast.FuncParam{{Name: "self"}},
				ReturnType: "()",
			},
		},
		Env: ev.globalEnv,
	}

	stderrInstance := &StructInstance{
		Def:      stderrStructDef,
		Fields:   make(map[string]Value),
		Embedded: make(map[string]*StructInstance),
	}
	stderrInstance.Fields["Write"] = &BuiltinFunc{
		Name:     "Write",
		FuncType: writeFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("Write expects 1 argument, got %d", len(args))
			}
			forced := Force(args[0])
			bv, ok := forced.(BytesValue)
			if !ok {
				return nil, fmt.Errorf("Write expects Bytes, got %s", typeNameOf(forced))
			}
			// C/Ruby behavior: flush buffered stdout before writing to stderr
			if stdoutWriter := ev.globalEnv.GetStdoutWriter(); stdoutWriter != nil {
				if err := stdoutWriter.Flush(); err != nil {
					return nil, err
				}
			}
			w := ev.globalEnv.GetRawStderr()
			n, err := w.Write([]byte(string(bv)))
			if err != nil {
				return nil, err
			}
			return IntValue(n), nil
		},
	}
	stderrInstance.Fields["Close"] = &BuiltinFunc{
		Name:     "Close",
		FuncType: voidFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("Close expects 0 arguments, got %d", len(args))
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	}
	stderrInstance.Fields["Flush"] = &BuiltinFunc{
		Name:     "Flush",
		FuncType: voidFuncType,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("Flush expects 0 arguments, got %d", len(args))
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	}
	ioExports["Stderr"] = stderrInstance

	// Register module in built-in environment
	ev.builtinsEnv.RegisterModule(&ModuleValue{
		Name:    "io",
		Exports: ioExports,
	})
}
