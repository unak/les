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
