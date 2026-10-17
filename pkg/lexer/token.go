package lexer

// TokenType indicates lexical token category.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenError

	// Layout
	TokenNewline
	TokenIndent
	TokenDedent

	// Identifiers & Literals
	TokenIdent
	TokenInt
	TokenFloat
	TokenString
	TokenBool
	TokenNil

	// Keywords
	TokenDef
	TokenIf
	TokenElse
	TokenReturn
	TokenBreak
	TokenDo
	TokenModule
	TokenImport
	TokenIs
	TokenMatch
	TokenCase
	TokenStruct
	TokenInterface
	TokenFor
	TokenContinue
	TokenForce

	// Punctuation
	TokenAssign      // =
	TokenColon       // :
	TokenSemicolon   // ;
	TokenComma       // ,
	TokenDot         // .
	TokenAt          // @
	TokenQuestion    // ?
	TokenPipe        // |
	TokenPipeGreater // |>
	TokenArrow       // ->
	TokenLParen      // (
	TokenRParen      // )
	TokenLBracket    // [
	TokenRBracket    // ]
	TokenLBrace      // {
	TokenRBrace      // }

	// Operators
	TokenPlus    // +
	TokenMinus   // -
	TokenStar    // *
	TokenSlash   // /
	TokenPercent // %
	TokenEq      // ==
	TokenNotEq   // !=
	TokenLt      // <
	TokenLte     // <=
	TokenGt      // >
	TokenGte     // >=
	TokenNot       // !
	TokenAnd       // &&
	TokenOr        // ||
	TokenAmpersand // &
)

// Token holds token information.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}

func (t TokenType) String() string {
	switch t {
	case TokenEOF:
		return "end of file"
	case TokenError:
		return "error"
	case TokenNewline:
		return "newline"
	case TokenIndent:
		return "indent"
	case TokenDedent:
		return "dedent"
	case TokenIdent:
		return "identifier"
	case TokenInt:
		return "integer"
	case TokenFloat:
		return "float"
	case TokenString:
		return "string"
	case TokenBool:
		return "bool"
	case TokenNil:
		return "nil"
	case TokenDef:
		return "'def'"
	case TokenIf:
		return "'if'"
	case TokenElse:
		return "'else'"
	case TokenReturn:
		return "'return'"
	case TokenBreak:
		return "'break'"
	case TokenDo:
		return "'do'"
	case TokenModule:
		return "'module'"
	case TokenImport:
		return "'import'"
	case TokenIs:
		return "'is'"
	case TokenMatch:
		return "'match'"
	case TokenCase:
		return "'case'"
	case TokenStruct:
		return "'struct'"
	case TokenInterface:
		return "'interface'"
	case TokenFor:
		return "'for'"
	case TokenContinue:
		return "'continue'"
	case TokenForce:
		return "'force'"
	case TokenAssign:
		return "'='"
	case TokenColon:
		return "':'"
	case TokenSemicolon:
		return "';'"
	case TokenComma:
		return "','"
	case TokenDot:
		return "'.'"
	case TokenAt:
		return "'@'"
	case TokenQuestion:
		return "'?'"
	case TokenPipe:
		return "'|'"
	case TokenPipeGreater:
		return "'|>'"
	case TokenArrow:
		return "'->'"
	case TokenLParen:
		return "'('"
	case TokenRParen:
		return "')'"
	case TokenLBracket:
		return "'['"
	case TokenRBracket:
		return "']'"
	case TokenLBrace:
		return "'{'"
	case TokenRBrace:
		return "'}'"
	case TokenPlus:
		return "'+'"
	case TokenMinus:
		return "'-'"
	case TokenStar:
		return "'*'"
	case TokenSlash:
		return "'/'"
	case TokenPercent:
		return "'%'"
	case TokenEq:
		return "'=='"
	case TokenNotEq:
		return "'!='"
	case TokenLt:
		return "'<'"
	case TokenLte:
		return "'<='"
	case TokenGt:
		return "'>'"
	case TokenGte:
		return "'>='"
	case TokenNot:
		return "'!'"
	case TokenAnd:
		return "'&&'"
	case TokenOr:
		return "'||'"
	case TokenAmpersand:
		return "'&'"
	default:
		return "unknown"
	}
}

// FormatToken returns a human-readable representation of a token for error messages.
func FormatToken(tok Token) string {
	switch tok.Type {
	case TokenEOF:
		return "end of file"
	case TokenNewline:
		return "newline"
	case TokenIndent:
		return "indent"
	case TokenDedent:
		return "dedent"
	case TokenIdent:
		if tok.Literal != "" {
			return "identifier '" + tok.Literal + "'"
		}
		return "identifier"
	case TokenInt, TokenFloat, TokenString, TokenBool:
		if tok.Literal != "" {
			return tok.Type.String() + " " + tok.Literal
		}
		return tok.Type.String()
	case TokenNil:
		return "'nil'"
	default:
		if tok.Literal != "" {
			return "'" + tok.Literal + "'"
		}
		return tok.Type.String()
	}
}

