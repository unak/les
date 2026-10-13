package eval

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"unicode/utf8"

	"les/pkg/ast"
)

// customReaderAdapter adapts a Les value with a Read method into an io.Reader.
type customReaderAdapter struct {
	ev       *Evaluator
	target   Value
	leftover []byte
}

func (a *customReaderAdapter) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	defer func() {
		if r := recover(); r != nil {
			if exitErr, ok := r.(*ExitError); ok {
				panic(exitErr)
			}
			if retSig, ok := r.(*ReturnSignal); ok {
				panic(retSig)
			}
			if brkSig, ok := r.(*BreakSignal); ok {
				panic(brkSig)
			}
			if contSig, ok := r.(*ContinueSignal); ok {
				panic(contSig)
			}
			err = fmt.Errorf("runtime error in Reader.Read: %v", r)
		}
	}()
	// Drain leftover bytes if any
	if len(a.leftover) > 0 {
		copied := copy(p, a.leftover)
		a.leftover = a.leftover[copied:]
		if len(a.leftover) == 0 {
			a.leftover = nil
		}
		return copied, nil
	}

	inst, ok := a.target.(*StructInstance)
	if !ok {
		return 0, fmt.Errorf("expected StructInstance for Reader, got %s", typeNameOf(a.target))
	}
	readVal, err := inst.LookupMember("Read")
	if err != nil {
		return 0, fmt.Errorf("Reader has no Read method: %w", err)
	}
	resVal, err := a.ev.callValue(readVal, []Value{IntValue(len(p))})
	if err != nil {
		return 0, err
	}
	forced := Force(resVal)
	switch v := forced.(type) {
	case BytesValue:
		data := []byte(string(v))
		if len(data) == 0 {
			// A12: Empty byte sequence means no data yet / progress made, not EOF.
			return 0, nil
		}
		copied := copy(p, data)
		if copied < len(data) {
			a.leftover = append(a.leftover, data[copied:]...)
		}
		return copied, nil
	case NilValue:
		return 0, io.EOF
	default:
		return 0, fmt.Errorf("Read returned unexpected type %s (expected Bytes or Nil)", typeNameOf(forced))
	}
}

// customWriterAdapter adapts a Les value with a Write method into an io.Writer.
type customWriterAdapter struct {
	ev     *Evaluator
	target Value
}

func (a *customWriterAdapter) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	defer func() {
		if r := recover(); r != nil {
			if exitErr, ok := r.(*ExitError); ok {
				panic(exitErr)
			}
			if retSig, ok := r.(*ReturnSignal); ok {
				panic(retSig)
			}
			if brkSig, ok := r.(*BreakSignal); ok {
				panic(brkSig)
			}
			if contSig, ok := r.(*ContinueSignal); ok {
				panic(contSig)
			}
			err = fmt.Errorf("runtime error in Writer.Write: %v", r)
		}
	}()
	inst, ok := a.target.(*StructInstance)
	if !ok {
		return 0, fmt.Errorf("expected StructInstance for Writer, got %s", typeNameOf(a.target))
	}
	writeVal, err := inst.LookupMember("Write")
	if err != nil {
		return 0, fmt.Errorf("Writer has no Write method: %w", err)
	}
	resVal, err := a.ev.callValue(writeVal, []Value{BytesValue(string(p))})
	if err != nil {
		return 0, err
	}
	forced := Force(resVal)
	if iv, ok := forced.(IntValue); ok {
		nVal := int(iv)
		if nVal < 0 {
			return 0, fmt.Errorf("Write returned negative count %d", nVal)
		}
		if nVal > len(p) {
			return 0, fmt.Errorf("Write returned count %d exceeding buffer length %d", nVal, len(p))
		}
		if nVal < len(p) {
			return nVal, io.ErrShortWrite
		}
		return nVal, nil
	}
	return 0, fmt.Errorf("Write returned unexpected type %s", typeNameOf(forced))
}

// initBufioModule registers the built-in @bufio module.
func (ev *Evaluator) initBufioModule() {
	bufioExports := make(map[string]Value)

	// Reader struct definition
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

	// Writer struct definition
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

	// Reader constructor: @bufio.Reader(r)
	readerConstructor := &BuiltinFunc{
		Name:    "Reader",
		TypeDef: readerDef,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("Reader constructor expects 1 argument, got %d", len(args))
			}
			target := Force(args[0])

			// A7: If target is already a @bufio.Reader instance, return it directly.
			if si, ok := target.(*StructInstance); ok && si.Def == readerDef {
				return si, nil
			}

			var getReader func() *bufio.Reader
			ioMod, _ := ev.globalEnv.GetModule("io")
			if ioMod != nil && target == ioMod.Exports["Stdin"] {
				getReader = func() *bufio.Reader { return ev.globalEnv.GetStdinReader() }
			} else {
				// Eager check: verify target has callable Read
				if inst, ok := target.(*StructInstance); ok {
					if _, err := inst.LookupMember("Read"); err != nil {
						return nil, fmt.Errorf("Reader requires an object with a Read method: %w", err)
					}
				} else {
					return nil, fmt.Errorf("Reader requires an object with a Read method, got %s", typeNameOf(target))
				}
				adapter := &customReaderAdapter{ev: ev, target: target}
				br := bufio.NewReader(adapter)
				getReader = func() *bufio.Reader { return br }
			}
			return ev.makeBufioReaderInstance(getReader, readerDef), nil
		},
	}
	bufioExports["Reader"] = readerConstructor

	// Writer constructor: @bufio.Writer(w)
	writerConstructor := &BuiltinFunc{
		Name:    "Writer",
		TypeDef: writerDef,
		Fn: func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("Writer constructor expects 1 argument, got %d", len(args))
			}
			target := Force(args[0])

			// A7: If target is already a @bufio.Writer instance, return it directly.
			if si, ok := target.(*StructInstance); ok && si.Def == writerDef {
				return si, nil
			}

			var getWriter func() *bufio.Writer
			var getUnderlying func() io.Writer
			ioMod, _ := ev.globalEnv.GetModule("io")
			if ioMod != nil && target == ioMod.Exports["Stdout"] {
				getWriter = func() *bufio.Writer { return ev.globalEnv.GetStdoutWriter() }
				getUnderlying = func() io.Writer { return ev.globalEnv.GetRawStdout() }
			} else {
				// Eager check: verify target has callable Write
				if inst, ok := target.(*StructInstance); ok {
					if _, err := inst.LookupMember("Write"); err != nil {
						return nil, fmt.Errorf("Writer requires an object with a Write method: %w", err)
					}
				} else {
					return nil, fmt.Errorf("Writer requires an object with a Write method, got %s", typeNameOf(target))
				}
				adapter := &customWriterAdapter{ev: ev, target: target}
				bw := bufio.NewWriter(adapter)
				ev.globalEnv.RegisterBufioWriter(bw, adapter)
				getWriter = func() *bufio.Writer { return bw }
				getUnderlying = func() io.Writer { return adapter }
			}
			return ev.makeBufioWriterInstance(getWriter, getUnderlying, writerDef), nil
		},
	}
	bufioExports["Writer"] = writerConstructor

	ev.builtinsEnv.RegisterModule(&ModuleValue{
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
			// A6: n < 0 is error; n == 0 returns empty Bytes
			if numBytes < 0 {
				return nil, fmt.Errorf("Read: negative byte count %d", numBytes)
			}
			if numBytes == 0 {
				return BytesValue(""), nil
			}

			return NewActionThunk(func() (Value, error) {
				br := getReader()
				// Cap allocation to avoid huge allocations
				allocSize := numBytes
				if allocSize > 64*1024 {
					allocSize = 64 * 1024
				}
				buf := make([]byte, allocSize)
				n, err := br.Read(buf)
				if n > 0 {
					return BytesValue(string(buf[:n])), nil
				}
				if err == io.EOF || err == io.ErrNoProgress {
					return NilValue{}, nil
				}
				if err != nil {
					return nil, err
				}
				// 0 bytes read without error: empty read
				return BytesValue(""), nil
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
				if err != nil && err != io.EOF && err != io.ErrNoProgress {
					return nil, err
				}
				if len(line) > 0 {
					if line[len(line)-1] == '\n' {
						line = line[:len(line)-1]
						if len(line) > 0 && line[len(line)-1] == '\r' {
							line = line[:len(line)-1]
						}
					}
					if !utf8.Valid([]byte(line)) {
						return nil, fmt.Errorf("invalid UTF-8 byte sequence in bufio.Reader.ReadLine()")
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
				return nil, fmt.Errorf("ReadString: delimiter cannot be empty")
			}
			delimStr := string(sv)

			return NewActionThunk(func() (Value, error) {
				br := getReader()
				// A5: Support arbitrary multi-character delimiter
				delimBytes := []byte(delimStr)
				lastByte := delimBytes[len(delimBytes)-1]
				var accumulated []byte

				for {
					chunk, err := br.ReadBytes(lastByte)
					if len(chunk) > 0 {
						accumulated = append(accumulated, chunk...)
						if bytes.HasSuffix(accumulated, delimBytes) {
							if !utf8.Valid(accumulated) {
								return nil, fmt.Errorf("invalid UTF-8 byte sequence in bufio.Reader.ReadString()")
							}
							return StringValue(string(accumulated)), nil
						}
					}
					if err == io.EOF || err == io.ErrNoProgress {
						if len(accumulated) > 0 {
							if !utf8.Valid(accumulated) {
								return nil, fmt.Errorf("invalid UTF-8 byte sequence in bufio.Reader.ReadString()")
							}
							return StringValue(string(accumulated)), nil
						}
						return NilValue{}, nil
					}
					if err != nil {
						return nil, err
					}
				}
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
			// A6: n < 0 is error; n == 0 returns empty Bytes
			if numBytes < 0 {
				return nil, fmt.Errorf("Peek: negative byte count %d", numBytes)
			}
			if numBytes == 0 {
				return BytesValue(""), nil
			}

			return NewActionThunk(func() (Value, error) {
				br := getReader()
				bytes, err := br.Peek(numBytes)
				if err != nil && err != io.EOF && err != io.ErrNoProgress && len(bytes) == 0 {
					return nil, err
				}
				if len(bytes) > 0 {
					return BytesValue(string(bytes)), nil
				}
				return NilValue{}, nil
			}), nil
		},
	}

	return inst
}

// makeBufioWriterInstance wraps a *bufio.Writer getter into a Les StructInstance.
func (ev *Evaluator) makeBufioWriterInstance(getWriter func() *bufio.Writer, getUnderlying func() io.Writer, def *StructDefValue) *StructInstance {
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
			bv, ok := forced.(BytesValue)
			if !ok {
				return nil, fmt.Errorf("Write expects Bytes, got %s", typeNameOf(forced))
			}
			bw := getWriter()
			n, err := bw.Write([]byte(string(bv)))
			if err != nil {
				return nil, err
			}
			var und io.Writer
			if getUnderlying != nil {
				und = getUnderlying()
			}
			ev.globalEnv.RegisterBufioWriter(bw, und)
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
