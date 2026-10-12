package eval

import (
	"fmt"
	"io"

	"les/pkg/ast"
)

// initIOModule registers the built-in @io module.
func (ev *Evaluator) initIOModule() {
	ioExports := make(map[string]Value)

	// Reader interface definition: Read(n: Int): String | Nil
	readerIface := &InterfaceDefValue{
		Name: "Reader",
		Methods: map[string]ast.MethodSig{
			"Read": {
				Name:       "Read",
				Params:     []ast.FuncParam{{Name: "n", TypeAnnot: "Int"}},
				ReturnType: "String | Nil",
			},
		},
	}
	ioExports["Reader"] = readerIface

	// Writer interface definition: Write(s: String): Int
	writerIface := &InterfaceDefValue{
		Name: "Writer",
		Methods: map[string]ast.MethodSig{
			"Write": {
				Name:       "Write",
				Params:     []ast.FuncParam{{Name: "s", TypeAnnot: "String"}},
				ReturnType: "Int",
			},
		},
	}
	ioExports["Writer"] = writerIface

	// Closer interface definition: Close(): ()
	closerIface := &InterfaceDefValue{
		Name: "Closer",
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
		Name:     "ReadCloser",
		Embedded: []string{"Reader", "Closer"},
		Methods:  make(map[string]ast.MethodSig),
	}
	ioExports["ReadCloser"] = readCloserIface

	// WriteCloser interface definition: Writer, Closer
	writeCloserIface := &InterfaceDefValue{
		Name:     "WriteCloser",
		Embedded: []string{"Writer", "Closer"},
		Methods:  make(map[string]ast.MethodSig),
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
		Name: "Read",
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
				return StringValue(""), nil
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
					return StringValue(string(buf[:n])), nil
				}
				if err == io.EOF {
					return NilValue{}, nil
				}
				if err != nil {
					return nil, err
				}
				return NilValue{}, nil
			}), nil
		},
	}
	stdinInstance.Fields["Close"] = &BuiltinFunc{
		Name: "Close",
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
					{Name: "s", TypeAnnot: "String"},
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
		Name: "Write",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("Write expects 1 argument, got %d", len(args))
			}
			forced := Force(args[0])
			sv, ok := forced.(StringValue)
			if !ok {
				return nil, fmt.Errorf("Write expects String, got %s", typeNameOf(forced))
			}
			// C/Ruby behavior: flush buffered stdout before writing directly
			if stdoutWriter := ev.globalEnv.GetStdoutWriter(); stdoutWriter != nil {
				if err := stdoutWriter.Flush(); err != nil {
					return nil, err
				}
			}
			w := ev.globalEnv.GetRawStdout()
			n, err := w.Write([]byte(string(sv)))
			if err != nil {
				return nil, err
			}
			return IntValue(n), nil
		},
	}
	stdoutInstance.Fields["Close"] = &BuiltinFunc{
		Name: "Close",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("Close expects 0 arguments, got %d", len(args))
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	}
	stdoutInstance.Fields["Flush"] = &BuiltinFunc{
		Name: "Flush",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("Flush expects 0 arguments, got %d", len(args))
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
					{Name: "s", TypeAnnot: "String"},
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
		Name: "Write",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("Write expects 1 argument, got %d", len(args))
			}
			forced := Force(args[0])
			sv, ok := forced.(StringValue)
			if !ok {
				return nil, fmt.Errorf("Write expects String, got %s", typeNameOf(forced))
			}
			// C/Ruby behavior: flush buffered stdout before writing to stderr
			if stdoutWriter := ev.globalEnv.GetStdoutWriter(); stdoutWriter != nil {
				if err := stdoutWriter.Flush(); err != nil {
					return nil, err
				}
			}
			w := ev.globalEnv.GetRawStderr()
			n, err := w.Write([]byte(string(sv)))
			if err != nil {
				return nil, err
			}
			return IntValue(n), nil
		},
	}
	stderrInstance.Fields["Close"] = &BuiltinFunc{
		Name: "Close",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("Close expects 0 arguments, got %d", len(args))
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	}
	stderrInstance.Fields["Flush"] = &BuiltinFunc{
		Name: "Flush",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("Flush expects 0 arguments, got %d", len(args))
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	}
	ioExports["Stderr"] = stderrInstance

	// Register module in global environment
	ev.globalEnv.RegisterModule(&ModuleValue{
		Name:    "io",
		Exports: ioExports,
	})
}
