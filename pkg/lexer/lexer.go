package lexer

import (
	"fmt"
	"strings"
	"unicode"
)

// Lexer produces tokens from source code with Python-like off-side indentation.
type Lexer struct {
	source      []rune
	pos         int
	line        int
	col         int
	indentStack []int
	pending     []Token
	parenDepth  int // bracket/brace/paren depth disables newlines/indents
}

// NewLexer creates a new Lexer instance.
func NewLexer(input string) *Lexer {
	return &Lexer{
		source:      []rune(input),
		pos:         0,
		line:        1,
		col:         1,
		indentStack: []int{0},
		pending:     make([]Token, 0),
		parenDepth:  0,
	}
}

// NextToken returns the next token.
func (l *Lexer) PeekRestOfLine() string {
	var sb strings.Builder
	for i := l.pos; i < len(l.source); i++ {
		r := l.source[i]
		if r == '\n' || r == '\r' {
			break
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func (l *Lexer) NextToken() Token {
	if len(l.pending) > 0 {
		tok := l.pending[0]
		l.pending = l.pending[1:]
		return tok
	}

	for l.pos < len(l.source) {
		r := l.source[l.pos]

		// Handle comments
		if r == '#' {
			for l.pos < len(l.source) && l.source[l.pos] != '\n' {
				l.advance()
			}
			continue
		}

		// Handle newlines and indentation
		if r == '\n' {
			startLine := l.line
			startCol := l.col
			l.advance()

			if l.parenDepth > 0 {
				// Inside brackets/parentheses, ignore newlines and indentation
				continue
			}

			// Gather blank lines and calculate indentation of the next non-empty line
			indentSize := 0
			isBlankLine := false

			for l.pos < len(l.source) {
				curr := l.source[l.pos]
				if curr == ' ' {
					indentSize++
					l.advance()
				} else if curr == '\t' {
					indentSize += 4
					l.advance()
				} else if curr == '#' {
					for l.pos < len(l.source) && l.source[l.pos] != '\n' {
						l.advance()
					}
				} else if curr == '\r' {
					l.advance()
				} else if curr == '\n' {
					indentSize = 0
					l.advance()
				} else {
					break
				}
			}

			if l.pos >= len(l.source) {
				// Reached EOF
				break
			}

			if isBlankLine {
				continue
			}

			// Emit TokenNewline
			l.pending = append(l.pending, Token{
				Type:    TokenNewline,
				Literal: "\n",
				Line:    startLine,
				Col:     startCol,
			})

			// Check indentation against indentStack
			currentIndent := l.indentStack[len(l.indentStack)-1]
			if indentSize > currentIndent {
				l.indentStack = append(l.indentStack, indentSize)
				l.pending = append(l.pending, Token{
					Type:    TokenIndent,
					Literal: fmt.Sprintf("%d", indentSize),
					Line:    l.line,
					Col:     1,
				})
			} else if indentSize < currentIndent {
				for len(l.indentStack) > 1 && l.indentStack[len(l.indentStack)-1] > indentSize {
					l.indentStack = l.indentStack[:len(l.indentStack)-1]
					l.pending = append(l.pending, Token{
						Type:    TokenDedent,
						Literal: fmt.Sprintf("%d", indentSize),
						Line:    l.line,
						Col:     1,
					})
				}
			}

			if len(l.pending) > 0 {
				tok := l.pending[0]
				l.pending = l.pending[1:]
				return tok
			}
			continue
		}

		if r == ' ' || r == '\t' || r == '\r' {
			l.advance()
			continue
		}

		// String literals
		if r == '"' || r == '\'' {
			return l.readString(r)
		}

		// Numbers
		if unicode.IsDigit(r) {
			return l.readNumber()
		}

		// Identifiers / keywords
		if unicode.IsLetter(r) || r == '_' {
			return l.readIdentifier()
		}

		// Two-character operators
		if l.pos+1 < len(l.source) {
			two := string(l.source[l.pos : l.pos+2])
			switch two {
			case "==":
				t := Token{Type: TokenEq, Literal: "==", Line: l.line, Col: l.col}
				l.advance()
				l.advance()
				return t
			case "!=":
				t := Token{Type: TokenNotEq, Literal: "!=", Line: l.line, Col: l.col}
				l.advance()
				l.advance()
				return t
			case "<=":
				t := Token{Type: TokenLte, Literal: "<=", Line: l.line, Col: l.col}
				l.advance()
				l.advance()
				return t
			case ">=":
				t := Token{Type: TokenGte, Literal: ">=", Line: l.line, Col: l.col}
				l.advance()
				l.advance()
				return t
			case "&&":
				t := Token{Type: TokenAnd, Literal: "&&", Line: l.line, Col: l.col}
				l.advance()
				l.advance()
				return t
			case "||":
				t := Token{Type: TokenOr, Literal: "||", Line: l.line, Col: l.col}
				l.advance()
				l.advance()
				return t
			case "->":
				t := Token{Type: TokenArrow, Literal: "->", Line: l.line, Col: l.col}
				l.advance()
				l.advance()
				return t
			}
		}

		// Single character tokens
		tok := Token{Line: l.line, Col: l.col}
		switch r {
		case '=':
			tok.Type = TokenAssign
			tok.Literal = "="
		case ':':
			tok.Type = TokenColon
			tok.Literal = ":"
		case ',':
			tok.Type = TokenComma
			tok.Literal = ","
		case '@':
			tok.Type = TokenAt
			tok.Literal = "@"
		case '.':
			tok.Type = TokenDot
			tok.Literal = "."
		case '?':
			tok.Type = TokenQuestion
			tok.Literal = "?"
		case '|':
			tok.Type = TokenPipe
			tok.Literal = "|"
		case '(':
			tok.Type = TokenLParen
			tok.Literal = "("
			l.parenDepth++
		case ')':
			tok.Type = TokenRParen
			tok.Literal = ")"
			if l.parenDepth > 0 {
				l.parenDepth--
			}
		case '[':
			tok.Type = TokenLBracket
			tok.Literal = "["
			l.parenDepth++
		case ']':
			tok.Type = TokenRBracket
			tok.Literal = "]"
			if l.parenDepth > 0 {
				l.parenDepth--
			}
		case '{':
			tok.Type = TokenLBrace
			tok.Literal = "{"
			l.parenDepth++
		case '}':
			tok.Type = TokenRBrace
			tok.Literal = "}"
			if l.parenDepth > 0 {
				l.parenDepth--
			}
		case '+':
			tok.Type = TokenPlus
			tok.Literal = "+"
		case '-':
			tok.Type = TokenMinus
			tok.Literal = "-"
		case '*':
			tok.Type = TokenStar
			tok.Literal = "*"
		case '/':
			tok.Type = TokenSlash
			tok.Literal = "/"
		case '%':
			tok.Type = TokenPercent
			tok.Literal = "%"
		case '<':
			tok.Type = TokenLt
			tok.Literal = "<"
		case '>':
			tok.Type = TokenGt
			tok.Literal = ">"
		case '!':
			tok.Type = TokenNot
			tok.Literal = "!"
		default:
			tok.Type = TokenError
			tok.Literal = string(r)
		}
		l.advance()
		return tok
	}

	// At EOF, unwind remaining indentation
	if len(l.indentStack) > 1 {
		l.indentStack = l.indentStack[:len(l.indentStack)-1]
		return Token{
			Type:    TokenDedent,
			Literal: "",
			Line:    l.line,
			Col:     l.col,
		}
	}

	return Token{
		Type:    TokenEOF,
		Literal: "",
		Line:    l.line,
		Col:     l.col,
	}
}

func (l *Lexer) advance() {
	if l.pos < len(l.source) {
		if l.source[l.pos] == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.pos++
	}
}

func (l *Lexer) readString(quote rune) Token {
	startLine := l.line
	startCol := l.col
	l.advance() // skip open quote

	var sb strings.Builder
	for l.pos < len(l.source) && l.source[l.pos] != quote {
		r := l.source[l.pos]
		if r == '\\' && l.pos+1 < len(l.source) {
			l.advance()
			next := l.source[l.pos]
			switch next {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case 'r':
				sb.WriteRune('\r')
			case '\\':
				sb.WriteRune('\\')
			case quote:
				sb.WriteRune(quote)
			default:
				sb.WriteRune(next)
			}
		} else {
			sb.WriteRune(r)
		}
		l.advance()
	}

	if l.pos < len(l.source) {
		l.advance() // skip closing quote
	}

	return Token{
		Type:    TokenString,
		Literal: sb.String(),
		Line:    startLine,
		Col:     startCol,
	}
}

func (l *Lexer) readNumber() Token {
	startLine := l.line
	startCol := l.col
	var sb strings.Builder
	isFloat := false

	for l.pos < len(l.source) {
		r := l.source[l.pos]
		if unicode.IsDigit(r) {
			sb.WriteRune(r)
			l.advance()
		} else if r == '.' && !isFloat && l.pos+1 < len(l.source) && unicode.IsDigit(l.source[l.pos+1]) {
			isFloat = true
			sb.WriteRune(r)
			l.advance()
		} else {
			break
		}
	}

	tokType := TokenInt
	if isFloat {
		tokType = TokenFloat
	}

	return Token{
		Type:    tokType,
		Literal: sb.String(),
		Line:    startLine,
		Col:     startCol,
	}
}

func (l *Lexer) readIdentifier() Token {
	startLine := l.line
	startCol := l.col
	var sb strings.Builder

	for l.pos < len(l.source) {
		r := l.source[l.pos]
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			sb.WriteRune(r)
			l.advance()
		} else {
			break
		}
	}

	name := sb.String()
	tokType := TokenIdent
	switch name {
	case "def":
		tokType = TokenDef
	case "if":
		tokType = TokenIf
	case "else":
		tokType = TokenElse
	case "return":
		tokType = TokenReturn
	case "break":
		tokType = TokenBreak
	case "do":
		tokType = TokenDo
	case "module":
		tokType = TokenModule
	case "import":
		tokType = TokenImport
	case "is":
		tokType = TokenIs
	case "match":
		tokType = TokenMatch
	case "case":
		tokType = TokenCase
	case "struct":
		tokType = TokenStruct
	case "interface":
		tokType = TokenInterface
	case "for":
		tokType = TokenFor
	case "continue":
		tokType = TokenContinue
	case "True", "False":
		tokType = TokenBool
	case "Nil":
		tokType = TokenNil
	}

	return Token{
		Type:    tokType,
		Literal: name,
		Line:    startLine,
		Col:     startCol,
	}
}
