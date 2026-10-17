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
	input := `~ & ^ | << >> <<< >>> < > <= >= && || |>`
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
		{TokenArithLShift, "<<<"},
		{TokenArithRShift, ">>>"},
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
