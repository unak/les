package lexer

import (
	"testing"
)

func TestLexerIndentDedent(t *testing.T) {
	input := `def add(a, b):
    a + b

x = 10`

	l := NewLexer(input)
	tokens := make([]Token, 0)
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}

	expectedTypes := []TokenType{
		TokenDef, TokenIdent, TokenLParen, TokenIdent, TokenComma, TokenIdent, TokenRParen, TokenColon,
		TokenNewline, TokenIndent,
		TokenIdent, TokenPlus, TokenIdent,
		TokenNewline, TokenDedent,
		TokenIdent, TokenAssign, TokenInt,
		TokenEOF,
	}

	idx := 0
	for _, tok := range tokens {
		if tok.Type == TokenNewline && idx < len(expectedTypes) && expectedTypes[idx] != TokenNewline {
			// Skip extra newlines
			continue
		}
		if idx >= len(expectedTypes) {
			t.Fatalf("unexpected extra token: %+v", tok)
		}
		if tok.Type != expectedTypes[idx] {
			t.Fatalf("token %d mismatch: got %v (%s), want %v", idx, tok.Type, tok.Literal, expectedTypes[idx])
		}
		idx++
	}
}

func TestLexerBlockTokens(t *testing.T) {
	input := `each items do |x|:
    print x`

	l := NewLexer(input)
	expected := []TokenType{
		TokenIdent, TokenIdent, TokenDo, TokenPipe, TokenIdent, TokenPipe, TokenColon,
		TokenNewline, TokenIndent,
		TokenIdent, TokenIdent,
		TokenDedent, TokenEOF,
	}

	for _, exp := range expected {
		tok := l.NextToken()
		for tok.Type == TokenNewline && exp != TokenNewline {
			tok = l.NextToken()
		}
		if tok.Type != exp {
			t.Fatalf("expected token type %v, got %v (%s)", exp, tok.Type, tok.Literal)
		}
	}
}

func TestLexerArrowToken(t *testing.T) {
	input := `(Int) -> String`
	l := NewLexer(input)
	expected := []TokenType{
		TokenLParen, TokenIdent, TokenRParen, TokenArrow, TokenIdent, TokenEOF,
	}
	for _, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp {
			t.Fatalf("expected token %v, got %v (%s)", exp, tok.Type, tok.Literal)
		}
	}
}

func TestLexerSemicolon(t *testing.T) {
	input := `a = 1; b = 2;`
	l := NewLexer(input)
	expected := []TokenType{
		TokenIdent, TokenAssign, TokenInt, TokenSemicolon,
		TokenIdent, TokenAssign, TokenInt, TokenSemicolon,
		TokenEOF,
	}
	for _, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp {
			t.Fatalf("expected token %v, got %v (%s)", exp, tok.Type, tok.Literal)
		}
	}
}

func TestLexerLineContinuation(t *testing.T) {
	input := "x = 10 + \\\n    20\n"
	l := NewLexer(input)
	expected := []TokenType{
		TokenIdent, TokenAssign, TokenInt, TokenPlus, TokenInt, TokenEOF,
	}
	for _, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp {
			t.Fatalf("expected token %v, got %v (%s)", exp, tok.Type, tok.Literal)
		}
	}
}

func TestLexerLineContinuationError(t *testing.T) {
	// '\' not followed by newline is an error
	input := "x = 10 \\ 20"
	l := NewLexer(input)
	_ = l.NextToken()    // x
	_ = l.NextToken()    // =
	_ = l.NextToken()    // 10
	tok := l.NextToken() // should be error
	if tok.Type != TokenError {
		t.Fatalf("expected TokenError for invalid line continuation, got %v (%s)", tok.Type, tok.Literal)
	}
}

func TestLexerLineContinuationWithCommentAndWhitespace(t *testing.T) {
	input := "x = 10 + \\   # trailing comment\n    20 + \\   \n    30\n"
	l := NewLexer(input)
	expected := []TokenType{
		TokenIdent, TokenAssign, TokenInt, TokenPlus, TokenInt, TokenPlus, TokenInt, TokenEOF,
	}
	for _, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp {
			t.Fatalf("expected token %v, got %v (%s)", exp, tok.Type, tok.Literal)
		}
	}
}

func TestLexerBoolAndNilKeywords(t *testing.T) {
	input := "true false nil True False Nil"
	l := NewLexer(input)
	expected := []struct {
		tokType TokenType
		literal string
	}{
		{TokenBool, "true"},
		{TokenBool, "false"},
		{TokenNil, "nil"},
		{TokenIdent, "True"},
		{TokenIdent, "False"},
		{TokenIdent, "Nil"},
		{TokenEOF, ""},
	}

	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp.tokType || tok.Literal != exp.literal {
			t.Fatalf("[%d] expected %v (%q), got %v (%q)", i, exp.tokType, exp.literal, tok.Type, tok.Literal)
		}
	}
}

func TestLexerRadixAndDigitSeparators(t *testing.T) {
	input := "0b1010 0B1100_0011 0o755 0O644 0d100 0D1_000_000 0xdead_beef 0XFF 1_000_000 3.14159_26535"
	l := NewLexer(input)
	expected := []struct {
		tokType TokenType
		literal string
	}{
		{TokenInt, "0b1010"},
		{TokenInt, "0B1100_0011"},
		{TokenInt, "0o755"},
		{TokenInt, "0O644"},
		{TokenInt, "0d100"},
		{TokenInt, "0D1_000_000"},
		{TokenInt, "0xdead_beef"},
		{TokenInt, "0XFF"},
		{TokenInt, "1_000_000"},
		{TokenFloat, "3.14159_26535"},
		{TokenEOF, ""},
	}

	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp.tokType || tok.Literal != exp.literal {
			t.Fatalf("[%d] expected %v (%q), got %v (%q)", i, exp.tokType, exp.literal, tok.Type, tok.Literal)
		}
	}
}

func TestLexerInvalidNumericLiterals(t *testing.T) {
	invalidTokens := []string{
		"1_",
		"0a0",
		"123__456",
		"0x",
		"0b",
		"0x_12",
		"3.14_",
	}

	for _, input := range invalidTokens {
		t.Run(input, func(t *testing.T) {
			l := NewLexer(input)
			tok := l.NextToken()
			if tok.Type != TokenError {
				t.Fatalf("expected TokenError for %q, got %v (%q)", input, tok.Type, tok.Literal)
			}
		})
	}
}

func TestLexerBitwiseOperators(t *testing.T) {
	input := `~ & ^ | << >> < > <= >= && || |>`
	l := NewLexer(input)
	expected := []struct {
		tokType TokenType
		literal string
	}{
		{TokenTilde, "~"},
		{TokenAmpersand, "&"},
		{TokenCaret, "^"},
		{TokenPipe, "|"},
		{TokenLShift, "<<"},
		{TokenRShift, ">>"},
		{TokenLt, "<"},
		{TokenGt, ">"},
		{TokenLte, "<="},
		{TokenGte, ">="},
		{TokenAnd, "&&"},
		{TokenOr, "||"},
		{TokenPipeGreater, "|>"},
		{TokenEOF, ""},
	}

	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp.tokType || tok.Literal != exp.literal {
			t.Fatalf("[%d] expected %v (%q), got %v (%q)", i, exp.tokType, exp.literal, tok.Type, tok.Literal)
		}
	}
}

func TestLexerRegexAndMatchTokens(t *testing.T) {
	input := `/foo/i =~ "foobar"
a / b
/hello\/world/
/[a-z/]/m
`
	l := NewLexer(input)
	expected := []struct {
		tokType TokenType
		literal string
	}{
		{TokenRegex, "/foo/i"},
		{TokenRegexMatch, "=~"},
		{TokenString, "foobar"},
		{TokenNewline, "\n"},
		{TokenIdent, "a"},
		{TokenSlash, "/"},
		{TokenIdent, "b"},
		{TokenNewline, "\n"},
		{TokenRegex, "/hello\\/world/"},
		{TokenNewline, "\n"},
		{TokenRegex, "/[a-z/]/m"},
		{TokenEOF, ""},
	}

	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp.tokType || (exp.literal != "\n" && tok.Literal != exp.literal) {
			t.Fatalf("[%d] expected %v (%q), got %v (%q)", i, exp.tokType, exp.literal, tok.Type, tok.Literal)
		}
	}
}

func TestLexerRegexParenAndCharClass(t *testing.T) {
	input := `/[^]/]/
(
/first/,
/second/
)
`
	l := NewLexer(input)
	expected := []struct {
		tokType TokenType
		literal string
	}{
		{TokenRegex, `/[^]/]/`},
		{TokenNewline, "\n"},
		{TokenLParen, `(`},
		{TokenRegex, `/first/`},
		{TokenComma, `,`},
		{TokenRegex, `/second/`},
		{TokenRParen, `)`},
		{TokenEOF, ""},
	}

	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp.tokType || (exp.literal != "\n" && tok.Literal != exp.literal) {
			t.Fatalf("[%d] expected %v (%q), got %v (%q)", i, exp.tokType, exp.literal, tok.Type, tok.Literal)
		}
	}
}

func TestLexerPostfixForceDivision(t *testing.T) {
	input := `a! / b / c`
	l := NewLexer(input)
	toks := []TokenType{TokenIdent, TokenNot, TokenSlash, TokenIdent, TokenSlash, TokenIdent, TokenEOF}
	for i, exp := range toks {
		tok := l.NextToken()
		if tok.Type != exp {
			t.Fatalf(`[%d] expected %v, got %v (%s)`, i, exp, tok.Type, tok.Literal)
		}
	}
}

func TestLexerRegexPOSIXAndNonPOSIXCharClass(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`/[[:]/`, `/[[:]/`},
		{`/[[:a]/`, `/[[:a]/`},
		{`/[[:alnum:]]/`, `/[[:alnum:]]/`},
		{`/[[:^digit:]]/`, `/[[:^digit:]]/`},
	}
	for _, tt := range tests {
		l := NewLexer(tt.input)
		tok := l.NextToken()
		if tok.Type != TokenRegex || tok.Literal != tt.expected {
			t.Fatalf("expected TokenRegex with %q, got %v with %q", tt.expected, tok.Type, tok.Literal)
		}
	}
}

func TestLexerParenLineContinuationDivision(t *testing.T) {
	input := "(a\n / b / c)"
	l := NewLexer(input)
	toks := []TokenType{TokenLParen, TokenIdent, TokenSlash, TokenIdent, TokenSlash, TokenIdent, TokenRParen, TokenEOF}
	for i, exp := range toks {
		tok := l.NextToken()
		if tok.Type != exp {
			t.Fatalf("[%d] expected %v, got %v (%s)", i, exp, tok.Type, tok.Literal)
		}
	}
}

func TestLexerRegexPOSIXCharClassColumn(t *testing.T) {
	input := "/[[:alnum:]]/ x"
	l := NewLexer(input)
	tok1 := l.NextToken()
	if tok1.Type != TokenRegex || tok1.Literal != "/[[:alnum:]]/" {
		t.Fatalf("expected TokenRegex, got %v (%s)", tok1.Type, tok1.Literal)
	}
	tok2 := l.NextToken()
	if tok2.Type != TokenIdent || tok2.Literal != "x" {
		t.Fatalf("expected TokenIdent x, got %v (%s)", tok2.Type, tok2.Literal)
	}
	if tok2.Col != 15 {
		t.Fatalf("expected col 15 for x, got %d", tok2.Col)
	}
}

func TestLexerColonColon(t *testing.T) {
	input := "math::Add(1, 2) ::"
	l := NewLexer(input)
	expected := []struct {
		tokType TokenType
		literal string
	}{
		{TokenIdent, "math"},
		{TokenColonColon, "::"},
		{TokenIdent, "Add"},
		{TokenLParen, "("},
		{TokenInt, "1"},
		{TokenComma, ","},
		{TokenInt, "2"},
		{TokenRParen, ")"},
		{TokenColonColon, "::"},
		{TokenEOF, ""},
	}
	for i, exp := range expected {
		tok := l.NextToken()
		if tok.Type != exp.tokType || (exp.literal != "" && tok.Literal != exp.literal) {
			t.Fatalf("[%d] expected %v (%q), got %v (%q)", i, exp.tokType, exp.literal, tok.Type, tok.Literal)
		}
	}
}

func TestExplicitIndentSemicolons(t *testing.T) {
	collectTypes := func(input string) []TokenType {
		l := NewLexer(input)
		var out []TokenType
		for {
			tok := l.NextToken()
			out = append(out, tok.Type)
			if tok.Type == TokenEOF {
				break
			}
		}
		return out
	}

	assertTypes := func(name, input string, want []TokenType) {
		t.Helper()
		got := collectTypes(input)
		if len(got) != len(want) {
			t.Fatalf("%s: length mismatch\ngot:  %v\nwant: %v", name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: [%d] got %v, want %v\nfull got:  %v\nfull want: %v", name, i, got[i], want[i], got, want)
			}
		}
	}

	// 1. +; and -;
	assertTypes("basic +; and -;", "def f(): +; 1 -;", []TokenType{
		TokenDef, TokenIdent, TokenLParen, TokenRParen, TokenColon,
		TokenNewline, TokenIndent,
		TokenInt,
		TokenNewline, TokenDedent,
		TokenEOF,
	})

	// 2. --; multi-dedent
	assertTypes("multi-dedent --;", "if a: +; if b: +; 1 --; 2", []TokenType{
		TokenIf, TokenIdent, TokenColon, TokenNewline, TokenIndent,
		TokenIf, TokenIdent, TokenColon, TokenNewline, TokenIndent,
		TokenInt,
		TokenNewline, TokenDedent, TokenDedent,
		TokenInt, TokenEOF,
	})

	// 3. Underflow clamping on ---;
	assertTypes("underflow ---;", "if a: +; 1 ---; 2", []TokenType{
		TokenIf, TokenIdent, TokenColon, TokenNewline, TokenIndent,
		TokenInt,
		TokenNewline, TokenDedent,
		TokenInt, TokenEOF,
	})

	// 4. ;; unwinds all levels to 0
	assertTypes(";; unwind to 0", "if a: +; if b: +; 1;; 2", []TokenType{
		TokenIf, TokenIdent, TokenColon, TokenNewline, TokenIndent,
		TokenIf, TokenIdent, TokenColon, TokenNewline, TokenIndent,
		TokenInt,
		TokenNewline, TokenDedent, TokenDedent,
		TokenInt, TokenEOF,
	})

	// 5. ;; at level 0 acts as statement separator
	assertTypes(";; at level 0", "1;; 2", []TokenType{
		TokenInt, TokenNewline, TokenInt, TokenEOF,
	})

	// 6. ; ; (whitespace-separated) emits two TokenSemicolon
	assertTypes("space-separated ; ;", "1; ; 2", []TokenType{
		TokenInt, TokenSemicolon, TokenSemicolon, TokenInt, TokenEOF,
	})

	// 7. + ; and - ; (whitespace-separated) emit operator + semicolon
	assertTypes("space-separated + ; and - ;", "1 + ; 2 - ;", []TokenType{
		TokenInt, TokenPlus, TokenSemicolon, TokenInt, TokenMinus, TokenSemicolon, TokenEOF,
	})

	// 8. ->; does not confuse with -;
	assertTypes("arrow followed by semicolon ->;", "->;", []TokenType{
		TokenArrow, TokenSemicolon, TokenEOF,
	})

	// 9. Hybrid multi-line with +; followed by physical 4-space indent does not emit duplicate TokenIndent
	assertTypes("hybrid +; with physical indent", "def f(): +;\n    x = 1\n    y = 2\nz = 3", []TokenType{
		TokenDef, TokenIdent, TokenLParen, TokenRParen, TokenColon, TokenNewline, TokenIndent,
		TokenIdent, TokenAssign, TokenInt, TokenNewline,
		TokenIdent, TokenAssign, TokenInt, TokenNewline, TokenDedent,
		TokenIdent, TokenAssign, TokenInt, TokenEOF,
	})

	// 10. Multi-line without physical spaces does not prematurely dedent on \n
	assertTypes("multiline without physical spaces", "def f(): +;\na = 1\nb = 2 -;\nc = 3", []TokenType{
		TokenDef, TokenIdent, TokenLParen, TokenRParen, TokenColon, TokenNewline, TokenIndent,
		TokenIdent, TokenAssign, TokenInt, TokenNewline,
		TokenIdent, TokenAssign, TokenInt, TokenNewline, TokenDedent,
		TokenNewline, TokenIdent, TokenAssign, TokenInt, TokenEOF,
	})

	// 11. Inside parens: f(|x|: +; x + 1 -;)
	assertTypes("inside parens", "f(|x|: +; x + 1 -;)", []TokenType{
		TokenIdent, TokenLParen, TokenPipe, TokenIdent, TokenPipe, TokenColon,
		TokenNewline, TokenIndent,
		TokenIdent, TokenPlus, TokenInt,
		TokenNewline, TokenDedent,
		TokenRParen, TokenEOF,
	})

	// 12. +; with trailing comment and newline in hybrid multiline
	assertTypes("hybrid +; with trailing comment", "def f(): +; # body\n    x = 1\ny = 2", []TokenType{
		TokenDef, TokenIdent, TokenLParen, TokenRParen, TokenColon, TokenNewline, TokenIndent,
		TokenIdent, TokenAssign, TokenInt, TokenNewline, TokenDedent,
		TokenIdent, TokenAssign, TokenInt, TokenEOF,
	})

	// 13. +; with trailing spaces and CRLF in hybrid multiline
	assertTypes("hybrid +; with CRLF", "def f(): +;  \r\n    x = 1\r\ny = 2", []TokenType{
		TokenDef, TokenIdent, TokenLParen, TokenRParen, TokenColon, TokenNewline, TokenIndent,
		TokenIdent, TokenAssign, TokenInt, TokenNewline, TokenDedent,
		TokenIdent, TokenAssign, TokenInt, TokenEOF,
	})

	// 14. +; followed by comment-only lines before EOF still closes at EOF
	assertTypes("+; followed by comment-only EOF", "def f(): +;\n   # only comment\n", []TokenType{
		TokenDef, TokenIdent, TokenLParen, TokenRParen, TokenColon, TokenNewline, TokenIndent,
		TokenDedent, TokenEOF,
	})

	// 15. ;;; splits into ;; (full dedent) + ; (TokenSemicolon)
	assertTypes("triple semicolon ;;;", "if a: +; 1;;; 2", []TokenType{
		TokenIf, TokenIdent, TokenColon, TokenNewline, TokenIndent,
		TokenInt, TokenNewline, TokenDedent, TokenSemicolon, TokenInt, TokenEOF,
	})

	// 16. ;; inside parens does not unwind outer physical indentation
	assertTypes(";; inside parens preserves outer indent", "def f():\n    x = (+; 1;;)\n    y = 2", []TokenType{
		TokenDef, TokenIdent, TokenLParen, TokenRParen, TokenColon, TokenNewline, TokenIndent,
		TokenIdent, TokenAssign, TokenLParen, TokenNewline, TokenIndent, TokenInt, TokenNewline, TokenDedent, TokenRParen, TokenNewline,
		TokenIdent, TokenAssign, TokenInt, TokenDedent, TokenEOF,
	})
}
