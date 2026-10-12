package eval

import (
	"bufio"
	"fmt"
	"io"

	"les/pkg/ast"
)

// customReaderAdapter adapts a Les value with a Read method into an io.Reader.
type customReaderAdapter struct {
	ev     *Evaluator
	target Value
}

func (a *customReaderAdapter) Read(p []byte) (int, error) {
	inst, ok := a.target.(*StructInstance)
	if !ok {
		return 0, fmt.Errorf("expected StructInstance for Reader, got %s", typeNameOf(a.target))
	}
	readVal, ok := inst.Fields["Read"]
	if !ok {
		return 0, fmt.Errorf("Reader has no Read method")
	}
	resVal, err := a.ev.callValue(readVal, []Value{IntValue(len(p))})
	if err != nil {
		return 0, err
	}
	forced := Force(resVal)
	switch v := forced.(type) {
	case StringValue:
		n := copy(p, []byte(string(v)))
		return n, nil
	case NilValue:
		return 0, io.EOF
	default:
		return 0, fmt.Errorf("Read returned unexpected type %s", typeNameOf(forced))
	}
}

// customWriterAdapter adapts a Les value with a Write method into an io.Writer.
type customWriterAdapter struct {
	ev     *Evaluator
	target Value
}

func (a *customWriterAdapter) Write(p []byte) (int, error) {
	inst, ok := a.target.(*StructInstance)
	if !ok {
		return 0, fmt.Errorf("expected StructInstance for Writer, got %s", typeNameOf(a.target))
	}
	writeVal, ok := inst.Fields["Write"]
	if !ok {
		return 0, fmt.Errorf("Writer has no Write method")
	}
	resVal, err := a.ev.callValue(writeVal, []Value{StringValue(string(p))})
	if err != nil {
		return 0, err
	}
	forced := Force(resVal)
	if iv, ok := forced.(IntValue); ok {
		return int(iv), nil
	}
	return len(p), nil
}

// initBufioModule registers the built-in @bufio module.
func (ev *Evaluator) initBufioModule() {
	bufioExports := make(map[string]Value)

	readerDef := &StructDefValue{
		Name:          "Reader",
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
			"ReadLine": {
				Name:       "ReadLine",
				Params:     []ast.FuncParam{{Name: "self"}},
				ReturnType: "String | Nil",
			},
			"ReadString": {
				Name: "ReadString",
				Params: []ast.FuncParam{
					{Name: "self"},
					{Name: "delim", TypeAnnot: "String"},
				},
				ReturnType: "String | Nil",
			},
			"Peek": {
				Name: "Peek",
				Params: []ast.FuncParam{
					{Name: "self"},
					{Name: "n", TypeAnnot: "Int"},
				},
				ReturnType: "String | Nil",
			},
		},
		Env: ev.globalEnv,
	}
	bufioExports["Reader"] = readerDef

	writerDef := &StructDefValue{
		Name:          "Writer",
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
			"Flush": {
				Name:       "Flush",
				Params:     []ast.FuncParam{{Name: "self"}},
				ReturnType: "()",
			},
		},
		Env: ev.globalEnv,
	}
	bufioExports["Writer"] = writerDef

	bufioExports["NewReader"] = &BuiltinFunc{
		Name: "NewReader",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("NewReader expects 1 argument, got %d", len(args))
			}
			target := Force(args[0])
			var getReader func() *bufio.Reader
			ioMod, _ := ev.globalEnv.GetModule("io")
			if ioMod != nil && target == ioMod.Exports["Stdin"] {
				getReader = func() *bufio.Reader { return ev.globalEnv.GetStdinReader() }
			} else {
				adapter := &customReaderAdapter{ev: ev, target: target}
				br := bufio.NewReader(adapter)
				getReader = func() *bufio.Reader { return br }
			}
			return ev.makeBufioReaderInstance(getReader, readerDef), nil
		},
	}

	bufioExports["NewWriter"] = &BuiltinFunc{
		Name: "NewWriter",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("NewWriter expects 1 argument, got %d", len(args))
			}
			target := Force(args[0])
			var getWriter func() *bufio.Writer
			ioMod, _ := ev.globalEnv.GetModule("io")
			if ioMod != nil && target == ioMod.Exports["Stdout"] {
				getWriter = func() *bufio.Writer { return ev.globalEnv.GetStdoutWriter() }
			} else {
				adapter := &customWriterAdapter{ev: ev, target: target}
				bw := bufio.NewWriter(adapter)
				getWriter = func() *bufio.Writer { return bw }
			}
			return ev.makeBufioWriterInstance(getWriter, writerDef), nil
		},
	}

	ev.globalEnv.RegisterModule(&ModuleValue{
		Name:    "bufio",
		Exports: bufioExports,
	})
}

// makeBufioReaderInstance wraps a *bufio.Reader getter into a Les StructInstance.
func (ev *Evaluator) makeBufioReaderInstance(getReader func() *bufio.Reader, def *StructDefValue) *StructInstance {
	inst := &StructInstance{
		Def:      def,
		Fields:   make(map[string]Value),
		Embedded: make(map[string]*StructInstance),
	}

	inst.Fields["Read"] = &BuiltinFunc{
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
			return NewActionThunk(func() (Value, error) {
				br := getReader()
				buf := make([]byte, numBytes)
				n, err := br.Read(buf)
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

	inst.Fields["ReadLine"] = &BuiltinFunc{
		Name: "ReadLine",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("ReadLine expects 0 arguments, got %d", len(args))
			}
			return NewActionThunk(func() (Value, error) {
				br := getReader()
				line, err := br.ReadString('\n')
				if err != nil && err != io.EOF {
					return nil, err
				}
				if len(line) > 0 {
					if line[len(line)-1] == '\n' {
						line = line[:len(line)-1]
						if len(line) > 0 && line[len(line)-1] == '\r' {
							line = line[:len(line)-1]
						}
					}
					return StringValue(line), nil
				}
				return NilValue{}, nil
			}), nil
		},
	}

	inst.Fields["ReadString"] = &BuiltinFunc{
		Name: "ReadString",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("ReadString expects 1 argument, got %d", len(args))
			}
			forced := Force(args[0])
			sv, ok := forced.(StringValue)
			if !ok {
				return nil, fmt.Errorf("ReadString expects String, got %s", typeNameOf(forced))
			}
			if len(sv) == 0 {
				return nil, fmt.Errorf("delimiter cannot be empty")
			}
			delimByte := sv[0]
			return NewActionThunk(func() (Value, error) {
				br := getReader()
				res, err := br.ReadString(delimByte)
				if err != nil && err != io.EOF {
					return nil, err
				}
				if len(res) > 0 {
					return StringValue(res), nil
				}
				return NilValue{}, nil
			}), nil
		},
	}

	inst.Fields["Peek"] = &BuiltinFunc{
		Name: "Peek",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("Peek expects 1 argument, got %d", len(args))
			}
			forced := Force(args[0])
			iv, ok := forced.(IntValue)
			if !ok {
				return nil, fmt.Errorf("Peek expects Int, got %s", typeNameOf(forced))
			}
			numBytes := int(iv)
			return NewActionThunk(func() (Value, error) {
				br := getReader()
				bytes, err := br.Peek(numBytes)
				if err != nil && err != io.EOF && len(bytes) == 0 {
					return nil, err
				}
				if len(bytes) > 0 {
					return StringValue(string(bytes)), nil
				}
				return NilValue{}, nil
			}), nil
		},
	}

	return inst
}

// makeBufioWriterInstance wraps a *bufio.Writer getter into a Les StructInstance.
func (ev *Evaluator) makeBufioWriterInstance(getWriter func() *bufio.Writer, def *StructDefValue) *StructInstance {
	inst := &StructInstance{
		Def:      def,
		Fields:   make(map[string]Value),
		Embedded: make(map[string]*StructInstance),
	}

	inst.Fields["Write"] = &BuiltinFunc{
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
			bw := getWriter()
			n, err := bw.WriteString(string(sv))
			if err != nil {
				return nil, err
			}
			return IntValue(n), nil
		},
	}

	inst.Fields["Flush"] = &BuiltinFunc{
		Name: "Flush",
		Fn: func(args []Value) (Value, error) {
			if len(args) != 0 {
				return nil, fmt.Errorf("Flush expects 0 arguments, got %d", len(args))
			}
			bw := getWriter()
			if err := bw.Flush(); err != nil {
				return nil, err
			}
			return &TupleValue{Elements: []Value{}}, nil
		},
	}

	return inst
}
