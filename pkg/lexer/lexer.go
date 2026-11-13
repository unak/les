package lexer

import (
	"fmt"
	"strings"
	"unicode"

	"les/pkg/ast"
)

// Lexer produces tokens from source code with Python-like off-side indentation.
type Lexer struct {
	source          []rune
	pos             int
	line            int
	col             int
	indentStack     []int
	pending         []Token
	parenDepth      int   // bracket/brace/paren depth disables newlines/indents
	parenIndentBase []int // indentStack length recorded at each opening bracket/brace/paren
	lastTokenType   TokenType
}

// NewLexer creates a new Lexer instance.
func NewLexer(input string) *Lexer {
	return &Lexer{
		source:          []rune(input),
		pos:             0,
		line:            1,
		col:             1,
		indentStack:     []int{0},
		pending:         make([]Token, 0),
		parenDepth:      0,
		parenIndentBase: make([]int, 0),
		lastTokenType:   TokenEOF,
	}
}

// NextToken returns the next token.
func (l *Lexer) PeekRestOfLine() string {
	var sb strings.Builder
	for i := l.pos; i < len(l.source); i++ {
		r := l.source[i]
		if r == '\n' || r == '\r' || r == ';' {
			break
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func isOperandTokenType(t TokenType) bool {
	switch t {
	case TokenIdent, TokenInt, TokenFloat, TokenString, TokenBool, TokenNil,
		TokenRegex, TokenRParen, TokenRBracket, TokenRBrace:
		return true
	default:
		return false
	}
}

func (l *Lexer) enterParen() {
	l.parenDepth++
	l.parenIndentBase = append(l.parenIndentBase, len(l.indentStack))
}

func (l *Lexer) exitParen() {
	if l.parenDepth > 0 {
		l.parenDepth--
	}
	if len(l.parenIndentBase) > 0 {
		l.parenIndentBase = l.parenIndentBase[:len(l.parenIndentBase)-1]
	}
}

func (l *Lexer) minIndentStackLen() int {
	if len(l.parenIndentBase) > 0 {
		return l.parenIndentBase[len(l.parenIndentBase)-1]
	}
	return 1
}

// emitDedents appends a TokenNewline followed by up to maxCount TokenDedent tokens,
// clamped at the active indentation base (level 0 or enclosing bracket base).
func (l *Lexer) emitDedents(maxCount, startLine, startCol int) Token {
	minLen := l.minIndentStackLen()
	l.pending = append(l.pending, Token{Type: TokenNewline, Literal: "\n", Line: startLine, Col: startCol})
	for i := 0; i < maxCount && len(l.indentStack) > minLen; i++ {
		l.indentStack = l.indentStack[:len(l.indentStack)-1]
		top := l.indentStack[len(l.indentStack)-1]
		l.pending = append(l.pending, Token{Type: TokenDedent, Literal: fmt.Sprintf("%d", top), Line: startLine, Col: startCol})
	}
	tok := l.pending[0]
	l.pending = l.pending[1:]
	return tok
}

func (l *Lexer) skipBlankLinesAndMeasureIndent() (indentSize int, reachedEOF bool) {
	indentSize = 0
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
			return indentSize, false
		}
	}
	return 0, true
}

func (l *Lexer) NextToken() Token {
	prev := l.lastTokenType
	tok := l.nextTokenInternal()
	if tok.Type == TokenNot && isOperandTokenType(prev) {
		l.lastTokenType = TokenRParen // Postfix '!' ends an operand
	} else {
		l.lastTokenType = tok.Type
	}
	return tok
}

func (l *Lexer) nextTokenInternal() Token {
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

		// Handle line continuation: '\' optionally followed by whitespace and comment, ending in newline
		if r == '\\' {
			startLine := l.line
			startCol := l.col
			l.advance() // consume '\\'

			// Skip trailing spaces or tabs on the same line
			for l.pos < len(l.source) && (l.source[l.pos] == ' ' || l.source[l.pos] == '\t') {
				l.advance()
			}

			// Skip trailing comment on the same line
			if l.pos < len(l.source) && l.source[l.pos] == '#' {
				for l.pos < len(l.source) && l.source[l.pos] != '\n' {
					l.advance()
				}
			}

			if l.pos < len(l.source) && l.source[l.pos] == '\r' {
				l.advance()
			}
			if l.pos < len(l.source) && l.source[l.pos] == '\n' {
				l.advance() // consume '\n'
				continue
			}
			return Token{
				Type:    TokenError,
				Literal: "unexpected character '\\': line continuation must be followed by newline (or trailing comment)",
				Line:    startLine,
				Col:     startCol,
			}
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
			indentSize, reachedEOF := l.skipBlankLinesAndMeasureIndent()
			if reachedEOF {
				// Reached EOF
				break
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

		// Regex literal or slash
		if r == '/' {
			if l.canStartRegexLiteral() {
				return l.readRegexLiteral()
			}
		}

		// Numbers
		if unicode.IsDigit(r) {
			return l.readNumber()
		}

		// Identifiers / keywords
		if unicode.IsLetter(r) || r == '_' {
			return l.readIdentifier()
		}

		// Explicit indent semicolons: +;, -;, --;, ;;
		if r == '+' && l.pos+1 < len(l.source) && l.source[l.pos+1] == ';' {
			startLine, startCol := l.line, l.col
			l.advance() // consume '+'
			l.advance() // consume ';'

			currentIndent := l.indentStack[len(l.indentStack)-1]
			newIndent := currentIndent

			if l.parenDepth == 0 {
				// Look ahead on current line for optional spaces/comment followed by newline
				k := l.pos
				for k < len(l.source) && (l.source[k] == ' ' || l.source[k] == '\t' || l.source[k] == '\r') {
					k++
				}
				if k < len(l.source) && l.source[k] == '#' {
					for k < len(l.source) && l.source[k] != '\n' {
						k++
					}
				}
				if k < len(l.source) && l.source[k] == '\n' {
					for l.pos <= k {
						l.advance() // consume up to and including '\n'
					}
					if nextIndent, eof := l.skipBlankLinesAndMeasureIndent(); !eof && nextIndent > newIndent {
						newIndent = nextIndent
					}
				}
			}

			l.indentStack = append(l.indentStack, newIndent)
			l.pending = append(l.pending,
				Token{Type: TokenNewline, Literal: "\n", Line: startLine, Col: startCol},
				Token{Type: TokenIndent, Literal: fmt.Sprintf("%d", newIndent), Line: startLine, Col: startCol},
			)
			tok := l.pending[0]
			l.pending = l.pending[1:]
			return tok
		}

		if r == '-' {
			k := l.pos
			for k < len(l.source) && l.source[k] == '-' {
				k++
			}
			if k < len(l.source) && l.source[k] == ';' {
				startLine, startCol := l.line, l.col
				dedentCount := k - l.pos
				for i := 0; i <= dedentCount; i++ {
					l.advance() // consume all '-' and ';'
				}
				return l.emitDedents(dedentCount, startLine, startCol)
			}
		}

		if r == ';' && l.pos+1 < len(l.source) && l.source[l.pos+1] == ';' {
			startLine, startCol := l.line, l.col
			l.advance() // consume first ';'
			l.advance() // consume second ';'
			return l.emitDedents(len(l.indentStack), startLine, startCol)
		}

		// Two-character operators
		if l.pos+1 < len(l.source) {
			two := string(l.source[l.pos : l.pos+2])
			switch two {
			case "::":
				t := Token{Type: TokenColonColon, Literal: "::", Line: l.line, Col: l.col}
				l.advance()
				l.advance()
				return t
			case "=~":
				t := Token{Type: TokenRegexMatch, Literal: "=~", Line: l.line, Col: l.col}
				l.advance()
				l.advance()
				return t
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
			case "<<":
				t := Token{Type: TokenLShift, Literal: "<<", Line: l.line, Col: l.col}
				l.advance()
				l.advance()
				return t
			case ">>":
				t := Token{Type: TokenRShift, Literal: ">>", Line: l.line, Col: l.col}
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
			case "|>":
				t := Token{Type: TokenPipeGreater, Literal: "|>", Line: l.line, Col: l.col}
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
		case ';':
			tok.Type = TokenSemicolon
			tok.Literal = ";"
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
		case '&':
			tok.Type = TokenAmpersand
			tok.Literal = "&"
		case '|':
			tok.Type = TokenPipe
			tok.Literal = "|"
		case '(':
			tok.Type = TokenLParen
			tok.Literal = "("
			l.enterParen()
		case ')':
			tok.Type = TokenRParen
			tok.Literal = ")"
			l.exitParen()
		case '[':
			tok.Type = TokenLBracket
			tok.Literal = "["
			l.enterParen()
		case ']':
			tok.Type = TokenRBracket
			tok.Literal = "]"
			l.exitParen()
		case '{':
			tok.Type = TokenLBrace
			tok.Literal = "{"
			l.enterParen()
		case '}':
			tok.Type = TokenRBrace
			tok.Literal = "}"
			l.exitParen()
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
		case '~':
			tok.Type = TokenTilde
			tok.Literal = "~"
		case '^':
			tok.Type = TokenCaret
			tok.Literal = "^"
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

func (l *Lexer) canStartRegexLiteral() bool {
	switch l.lastTokenType {
	case TokenIdent, TokenInt, TokenFloat, TokenString, TokenBool, TokenNil,
		TokenRegex, TokenRParen, TokenRBracket, TokenRBrace, TokenDef, TokenDot, TokenPipeGreater:
		return false
	}

	// Scan forward on current line to find closing '/'
	inCharClass := false
	charClassStart := -1
	escaped := false
	foundClose := false
	closePos := -1

	for i := l.pos + 1; i < len(l.source); i++ {
		ch := l.source[i]
		if ch == '\n' || ch == '\r' {
			break
		}
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if inCharClass {
			if ch == '[' {
				if nextIdx := ast.SkipPOSIXCharClassInRunes(l.source, i); nextIdx > i {
					i = nextIdx - 1
					continue
				}
			}
			if ch == ']' {
				if i == charClassStart+1 || (i == charClassStart+2 && l.source[charClassStart+1] == '^') {
					continue
				}
				inCharClass = false
				continue
			}
			continue
		}
		if ch == '[' && !inCharClass {
			inCharClass = true
			charClassStart = i
			continue
		}
		if ch == '/' && !inCharClass {
			if i > l.pos+1 { // pattern non-empty
				foundClose = true
				closePos = i
				break
			}
			// if i == l.pos+1, "//" is empty pattern, not regex
			return false
		}
	}

	if !foundClose {
		return false
	}

	// Validate trailing flags immediately after closing '/'
	for i := closePos + 1; i < len(l.source); i++ {
		ch := l.source[i]
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' {
			switch ch {
			case 'i', 'm', 's', 'U':
				// valid flag
			default:
				// invalid regex flag character adjacent to regex literal
				return false
			}
		} else {
			break
		}
	}

	return true
}

func (l *Lexer) readRegexLiteral() Token {
	startLine := l.line
	startCol := l.col

	var sb strings.Builder
	sb.WriteRune(l.source[l.pos]) // leading '/'
	l.advance()

	inCharClass := false
	charClassStart := -1
	escaped := false

	for l.pos < len(l.source) {
		r := l.source[l.pos]
		pos := l.pos
		if r == '\n' || r == '\r' {
			break
		}
		sb.WriteRune(r)
		l.advance()

		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if inCharClass {
			if r == '[' {
				if nextIdx := ast.SkipPOSIXCharClassInRunes(l.source, pos); nextIdx > pos {
					sb.WriteString(string(l.source[l.pos:nextIdx]))
					for l.pos < nextIdx {
						l.advance()
					}
					continue
				}
			}
			if r == ']' {
				if pos == charClassStart+1 || (pos == charClassStart+2 && l.source[charClassStart+1] == '^') {
					continue
				}
				inCharClass = false
				continue
			}
			continue
		}
		if r == '[' && !inCharClass {
			inCharClass = true
			charClassStart = pos
			continue
		}
		if r == '/' && !inCharClass {
			// Closing slash reached
			break
		}
	}

	// Read optional flags
	for l.pos < len(l.source) {
		r := l.source[l.pos]
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			sb.WriteRune(r)
			l.advance()
		} else {
			break
		}
	}

	return Token{
		Type:    TokenRegex,
		Literal: sb.String(),
		Line:    startLine,
		Col:     startCol,
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

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isOctalDigit(r rune) bool {
	return r >= '0' && r <= '7'
}

func isBinaryDigit(r rune) bool {
	return r == '0' || r == '1'
}

func (l *Lexer) readNumber() Token {
	startLine := l.line
	startCol := l.col
	var sb strings.Builder

	// Check for radix prefixes: 0b, 0o, 0d, 0x
	if l.source[l.pos] == '0' && l.pos+1 < len(l.source) {
		nextR := l.source[l.pos+1]
		var isRadixDigit func(rune) bool
		switch nextR {
		case 'b', 'B':
			isRadixDigit = isBinaryDigit
		case 'o', 'O':
			isRadixDigit = isOctalDigit
		case 'd', 'D':
			isRadixDigit = unicode.IsDigit
		case 'x', 'X':
			isRadixDigit = isHexDigit
		}

		if isRadixDigit != nil {
			// Require at least one valid digit immediately after prefix (not '_')
			if l.pos+2 >= len(l.source) || !isRadixDigit(l.source[l.pos+2]) {
				sb.WriteRune(l.source[l.pos])
				l.advance()
				sb.WriteRune(l.source[l.pos])
				l.advance()
				for l.pos < len(l.source) && (unicode.IsLetter(l.source[l.pos]) || unicode.IsDigit(l.source[l.pos]) || l.source[l.pos] == '_') {
					sb.WriteRune(l.source[l.pos])
					l.advance()
				}
				return Token{
					Type:    TokenError,
					Literal: sb.String(),
					Line:    startLine,
					Col:     startCol,
				}
			}

			sb.WriteRune(l.source[l.pos])
			l.advance()
			sb.WriteRune(l.source[l.pos])
			l.advance()

			for l.pos < len(l.source) {
				r := l.source[l.pos]
				if isRadixDigit(r) {
					sb.WriteRune(r)
					l.advance()
				} else if r == '_' {
					// Only allow '_' if followed by a valid radix digit
					if l.pos+1 < len(l.source) && isRadixDigit(l.source[l.pos+1]) {
						sb.WriteRune(r)
						l.advance()
					} else {
						break
					}
				} else {
					break
				}
			}

			// If immediately followed by letters or underscore, it's an invalid suffix
			if l.pos < len(l.source) && (unicode.IsLetter(l.source[l.pos]) || l.source[l.pos] == '_') {
				for l.pos < len(l.source) && (unicode.IsLetter(l.source[l.pos]) || unicode.IsDigit(l.source[l.pos]) || l.source[l.pos] == '_') {
					sb.WriteRune(l.source[l.pos])
					l.advance()
				}
				return Token{
					Type:    TokenError,
					Literal: sb.String(),
					Line:    startLine,
					Col:     startCol,
				}
			}

			return Token{
				Type:    TokenInt,
				Literal: sb.String(),
				Line:    startLine,
				Col:     startCol,
			}
		}
	}

	isFloat := false
	for l.pos < len(l.source) {
		r := l.source[l.pos]
		if unicode.IsDigit(r) {
			sb.WriteRune(r)
			l.advance()
		} else if r == '_' {
			// '_' must be followed by a digit
			if l.pos+1 < len(l.source) && unicode.IsDigit(l.source[l.pos+1]) {
				sb.WriteRune(r)
				l.advance()
			} else {
				break
			}
		} else if r == '.' && !isFloat && l.pos+1 < len(l.source) && unicode.IsDigit(l.source[l.pos+1]) {
			isFloat = true
			sb.WriteRune(r)
			l.advance()
		} else {
			break
		}
	}

	// If immediately followed by letters or underscore, it's an invalid suffix (e.g. 1_, 0a0, 123__456)
	if l.pos < len(l.source) && (unicode.IsLetter(l.source[l.pos]) || l.source[l.pos] == '_') {
		for l.pos < len(l.source) && (unicode.IsLetter(l.source[l.pos]) || unicode.IsDigit(l.source[l.pos]) || l.source[l.pos] == '_') {
			sb.WriteRune(l.source[l.pos])
			l.advance()
		}
		return Token{
			Type:    TokenError,
			Literal: sb.String(),
			Line:    startLine,
			Col:     startCol,
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
	case "force":
		tokType = TokenForce
	case "true", "false":
		tokType = TokenBool
	case "nil":
		tokType = TokenNil
	}

	return Token{
		Type:    tokType,
		Literal: name,
		Line:    startLine,
		Col:     startCol,
	}
}
