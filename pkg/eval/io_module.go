package eval

import (
	"io"

	"les/pkg/ast"
)

// initIOModule registers the built-in @io module.
func (ev *Evaluator) initIOModule() {
	ioExports := make(map[string]Value)

	// Reader interface definition: read(n: Int): String | Nil
	readerIface := &InterfaceDefValue{
		Name: "Reader",
		Methods: map[string]ast.MethodSig{
			"read": {
				Name:       "read",
				Params:     []ast.FuncParam{{Name: "n", TypeAnnot: "Int"}},
				ReturnType: "String | Nil",
			},
		},
	}
	ioExports["Reader"] = readerIface

	// Writer interface definition: write(s: String): Int
	writerIface := &InterfaceDefValue{
		Name: "Writer",
		Methods: map[string]ast.MethodSig{
			"write": {
				Name:       "write",
				Params:     []ast.FuncParam{{Name: "s", TypeAnnot: "String"}},
				ReturnType: "Int",
			},
		},
	}
	ioExports["Writer"] = writerIface

	// Closer interface definition: close(): ()
	closerIface := &InterfaceDefValue{
		Name: "Closer",
		Methods: map[string]ast.MethodSig{
			"close": {
				Name:       "close",
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
			"read": {
				Name: "read",
				Params: []ast.FuncParam{
					{Name: "self"},
					{Name: "n", TypeAnnot: "Int"},
				},
				ReturnType: "String | Nil",
			},
			"close": {
				Name:       "close",
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

	// Bind custom built-in method closures for stdin
	stdinInstance.Fields["read"] = &BuiltinFunc{
		Name: "read",
		Fn: func(args []Value) (Value, error) {
			numBytes := int64(1024)
			if len(args) > 0 {
				forced := Force(args[0])
				if iv, ok := forced.(IntValue); ok {
					numBytes = int64(iv)
				}
			}
			return NewActionThunk(func() (Value, error) {
				r := ev.globalEnv.GetStdinReader()
				buf := make([]byte, numBytes)
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
	stdinInstance.Fields["close"] = &BuiltinFunc{
		Name: "close",
		Fn: func(args []Value) (Value, error) {
			return &TupleValue{Elements: []Value{}}, nil
		},
	}

	ioExports["Stdin"] = stdinInstance

	// Register module in global environment
	ev.globalEnv.RegisterModule(&ModuleValue{
		Name:    "io",
		Exports: ioExports,
	})
}
