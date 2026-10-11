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

	// Punctuation
	TokenAssign    // =
	TokenColon     // :
	TokenComma     // ,
	TokenDot       // .
	TokenAt        // @
	TokenQuestion  // ?
	TokenPipe      // |
	TokenLParen    // (
	TokenRParen    // )
	TokenLBracket  // [
	TokenRBracket  // ]
	TokenLBrace    // {
	TokenRBrace    // }

	// Operators
	TokenPlus      // +
	TokenMinus     // -
	TokenStar      // *
	TokenSlash     // /
	TokenPercent   // %
	TokenEq        // ==
	TokenNotEq     // !=
	TokenLt        // <
	TokenLte       // <=
	TokenGt        // >
	TokenGte       // >=
	TokenNot       // !
	TokenAnd       // &&
	TokenOr        // ||
)

// Token holds token information.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}
