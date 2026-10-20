package parser

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"unicode"

	"les/pkg/ast"
	"les/pkg/lexer"
)

// Precedence levels for operator precedence parsing.
const (
	PREC_LOWEST       = iota
	PREC_PIPE         // |>
	PREC_CALL_NOPAREN // lowest binary expr: f a + b binds as f(a + b)
	PREC_TERNARY      // ? :
	PREC_LOGICAL_OR   // ||
	PREC_LOGICAL_AND  // &&
	PREC_EQUALITY     // == !=
	PREC_IS           // is
	PREC_COMPARISON   // < <= > >=
	PREC_BIT_OR       // |
	PREC_BIT_XOR      // ^
	PREC_BIT_AND      // &
	PREC_SHIFT        // << >>
	PREC_SUM          // + -
	PREC_PRODUCT      // * / %
	PREC_UNARY        // - ! ~
	PREC_CALL         // f()
	PREC_INDEX        // []
	PREC_DOT          // .
	PREC_POSTFIX      // ! (postfix force)
)

type Parser struct {
	l                   *lexer.Lexer
	curToken            lexer.Token
	peekToken           lexer.Token
	errors              []string
	filename            string
	inParenOmissionCall bool
	atStatementStart    bool
	inClosureDepth      int
}

func NewParser(l *lexer.Lexer) *Parser {
	p := &Parser{l: l}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) SetFilename(filename string) {
	p.filename = filename
}

func (p *Parser) curPos() ast.Position {
	return ast.Position{Line: p.curToken.Line, Col: p.curToken.Col}
}

func (p *Parser) peekPos() ast.Position {
	return ast.Position{Line: p.peekToken.Line, Col: p.peekToken.Col}
}

func (p *Parser) addError(line, col int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fn := p.filename
	if fn == "" {
		p.errors = append(p.errors, fmt.Sprintf("%d: %s at col %d", line, msg, col))
	} else {
		p.errors = append(p.errors, fmt.Sprintf("%s:%d: %s at col %d", fn, line, msg, col))
	}
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) curTokenIs(t lexer.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t lexer.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) expectPeek(t lexer.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.addError(p.peekToken.Line, p.peekToken.Col, "expected %s, got %s", t.String(), lexer.FormatToken(p.peekToken))
	return false
}

func (p *Parser) skipNewlines() {
	for p.curTokenIs(lexer.TokenNewline) || p.curTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}
}

func (p *Parser) ParseProgram() *ast.Program {
	prog := &ast.Program{
		Position: p.curPos(),
		Filename: p.filename,
		Body:     make([]ast.Stmt, 0),
	}
	p.skipNewlines()

	// Check optional file-level module declaration
	if p.curTokenIs(lexer.TokenModule) {
		modStmt := p.parseModuleDecl()
		if modStmt != nil {
			prog.ModuleName = modStmt.Name
			prog.Body = append(prog.Body, modStmt)
		}
		p.skipNewlines()
	}

	for !p.curTokenIs(lexer.TokenEOF) {
		prevTok := p.curToken
		stmt := p.parseStmt()
		if stmt != nil {
			prog.Body = append(prog.Body, stmt)
		}
		p.skipNewlines()
		if p.curToken == prevTok {
			p.nextToken()
		}
	}

	return prog
}

func (p *Parser) parseStmt() ast.Stmt {
	p.skipNewlines()
	if p.curTokenIs(lexer.TokenEOF) {
		return nil
	}
	switch p.curToken.Type {
	case lexer.TokenModule:
		return p.parseModuleDecl()
	case lexer.TokenImport:
		return p.parseImportStmt()
	case lexer.TokenDef:
		return p.parseFuncDef()
	case lexer.TokenStruct:
		return p.parseStructDef()
	case lexer.TokenInterface:
		return p.parseInterfaceDef()
	case lexer.TokenIf:
		return p.parseIfStmt()
	case lexer.TokenFor:
		return p.parseForStmt()
	case lexer.TokenReturn:
		return p.parseReturnStmt()
	case lexer.TokenBreak:
		if p.inClosureDepth <= 0 {
			p.addError(p.curToken.Line, p.curToken.Col, "break outside of closure or loop")
		}
		return p.parseBreakStmt()
	case lexer.TokenContinue:
		if p.inClosureDepth <= 0 {
			p.addError(p.curToken.Line, p.curToken.Col, "continue outside of loop")
		}
		return p.parseContinueStmt()
	case lexer.TokenLParen:
		if p.isUnpackParenDeclStart() {
			return p.parseUnpackDeclWithParen()
		}
		return p.parseExprStmt()
	case lexer.TokenIdent:
		// Check for unpacked assignment without paren: h, ...t = expr
		if p.peekTokenIs(lexer.TokenComma) && p.hasAssignOnCurrentLine() {
			return p.parseUnpackDeclWithoutParen()
		}
		// Check for member assignment: `target.member = val`
		if p.peekTokenIs(lexer.TokenDot) && p.isMemberAssignOnCurrentLine() {
			return p.parseMemberAssignStmt()
		}
		// Check for assignment: `x = val` or `x: Int = val`
		if p.peekTokenIs(lexer.TokenAssign) {
			return p.parseVarDecl("")
		}
		if p.peekTokenIs(lexer.TokenColon) {
			// type annotated assignment: x: Type = val
			return p.parseAnnotatedVarDecl()
		}
		return p.parseExprStmt()
	default:
		return p.parseExprStmt()
	}
}

func (p *Parser) parseImportStmt() *ast.ImportStmt {
	pos := p.curPos()
	p.nextToken() // consume 'import'
	var path string
	if p.curTokenIs(lexer.TokenIdent) || p.curTokenIs(lexer.TokenString) {
		path = p.curToken.Literal
	} else {
		p.addError(p.curToken.Line, p.curToken.Col, "expected identifier or string after import, got %s", lexer.FormatToken(p.curToken))
		return nil
	}
	alias := path

	if p.peekTokenIs(lexer.TokenNewline) {
		p.nextToken()
		p.nextToken()
	} else {
		p.nextToken()
	}
	return &ast.ImportStmt{
		Position: pos,
		Path:     path,
		Alias:    alias,
	}
}

func (p *Parser) parseModuleDecl() *ast.ModuleDecl {
	pos := p.curPos()
	p.nextToken() // consume 'module'
	if !p.curTokenIs(lexer.TokenIdent) {
		p.addError(p.curToken.Line, p.curToken.Col, "expected identifier after module, got %s", lexer.FormatToken(p.curToken))
		return nil
	}
	name := p.curToken.Literal
	if p.peekTokenIs(lexer.TokenNewline) {
		p.nextToken()
		p.nextToken()
	} else {
		p.nextToken()
	}
	return &ast.ModuleDecl{Position: pos, Name: name}
}

func (p *Parser) parseStructDef() *ast.StructDef {
	pos := p.curPos()
	p.nextToken() // consume 'struct'
	if !p.curTokenIs(lexer.TokenIdent) {
		p.addError(p.curToken.Line, p.curToken.Col, "expected struct name after struct")
		return nil
	}
	name := p.curToken.Literal

	var typeParams []ast.TypeParam
	if p.peekTokenIs(lexer.TokenLBracket) {
		typeParams = p.parseTypeParams()
	}

	if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken() // advance to ':'
	} else {
		p.addError(p.peekToken.Line, p.peekToken.Col, "expected ':' after struct name")
		return nil
	}

	p.nextToken() // consume ':'
	p.skipNewlines()

	if !p.curTokenIs(lexer.TokenIndent) {
		p.addError(p.curToken.Line, p.curToken.Col, "expected indent after struct declaration")
		return nil
	}
	p.nextToken() // consume TokenIndent

	var fields []ast.StructField
	var methods []*ast.FuncDef

	p.skipNewlines()
	for !p.curTokenIs(lexer.TokenDedent) && !p.curTokenIs(lexer.TokenEOF) {
		if p.curTokenIs(lexer.TokenDef) {
			m := p.parseFuncDef()
			if m != nil {
				methods = append(methods, m)
			}
		} else if p.curTokenIs(lexer.TokenIdent) {
			fPos := p.curPos()
			ident := p.curToken.Literal
			if p.peekTokenIs(lexer.TokenColon) {
				// field with type: x: Int [= expr]
				p.nextToken() // advance to ':'
				p.nextToken() // consume ':', advance to type start
				typeAnnot := p.parseTypeAnnotString()
				var defaultVal ast.Expr
				if p.curTokenIs(lexer.TokenAssign) {
					p.nextToken() // consume '='
					defaultVal = p.parseExpr(PREC_LOWEST)
				} else if p.peekTokenIs(lexer.TokenAssign) {
					p.nextToken() // advance to '='
					p.nextToken() // consume '='
					defaultVal = p.parseExpr(PREC_LOWEST)
				}
				fields = append(fields, ast.StructField{
					Position:     fPos,
					Name:         ident,
					TypeAnnot:    typeAnnot,
					DefaultValue: defaultVal,
					IsEmbedded:   false,
				})
				p.consumeStmtEnd()
			} else {
				// Embedded struct type: e.g. Person or Base[T]
				var typeAnnot string
				if p.peekTokenIs(lexer.TokenLBracket) {
					typeAnnot = p.parseTypeAnnotString()
				} else {
					typeAnnot = ident
					p.nextToken() // consume ident
				}
				fields = append(fields, ast.StructField{
					Position:   fPos,
					Name:       ident,
					TypeAnnot:  typeAnnot,
					IsEmbedded: true,
				})
				p.consumeStmtEnd()
			}
		} else if isOperatorMethodToken(p.curToken.Type) && p.peekTokenIs(lexer.TokenLParen) {
			// Operator method without 'def': e.g. +(other: Vec2): Vec2
			opPos := p.curPos()
			opName := p.parseOperatorMethodName()
			if opName != "" && p.expectPeek(lexer.TokenLParen) {
				params, blockParam := p.parseFuncParams()
				returnType := ""
				if p.peekTokenIs(lexer.TokenArrow) {
					p.nextToken()
					p.nextToken()
					returnType = p.parseTypeAnnotString()
					if p.peekTokenIs(lexer.TokenColon) {
						p.nextToken()
					}
				} else if p.peekTokenIs(lexer.TokenColon) {
					p.nextToken()
					if !p.peekTokenIs(lexer.TokenNewline) && !p.peekTokenIs(lexer.TokenIndent) && !p.peekTokenIs(lexer.TokenEOF) {
						p.nextToken()
						returnType = p.parseTypeAnnotString()
						if p.peekTokenIs(lexer.TokenColon) {
							p.nextToken()
						}
					}
				}
				body := p.parseBlockBody()
				methods = append(methods, &ast.FuncDef{
					Position:   opPos,
					Name:       opName,
					Params:     params,
					BlockParam: blockParam,
					ReturnType: strings.TrimSpace(returnType),
					Body:       body,
				})
			}
		} else {
			p.nextToken()
		}
		p.skipNewlines()
	}

	if p.curTokenIs(lexer.TokenDedent) {
		p.nextToken()
	}

	return &ast.StructDef{
		Position:   pos,
		Name:       name,
		TypeParams: typeParams,
		Fields:     fields,
		Methods:    methods,
	}
}

func (p *Parser) parseInterfaceDef() *ast.InterfaceDef {
	pos := p.curPos()
	p.nextToken() // consume 'interface'
	if !p.curTokenIs(lexer.TokenIdent) {
		p.addError(p.curToken.Line, p.curToken.Col, "expected interface name after interface")
		return nil
	}
	name := p.curToken.Literal

	var typeParams []ast.TypeParam
	if p.peekTokenIs(lexer.TokenLBracket) {
		typeParams = p.parseTypeParams()
	}

	if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken() // advance to ':'
	} else {
		p.addError(p.peekToken.Line, p.peekToken.Col, "expected ':' after interface name")
		return nil
	}

	p.nextToken() // consume ':'
	p.skipNewlines()

	if !p.curTokenIs(lexer.TokenIndent) {
		p.addError(p.curToken.Line, p.curToken.Col, "expected indent after interface declaration")
		return nil
	}
	p.nextToken() // consume TokenIndent

	var embedded []string
	var methods []ast.MethodSig

	p.skipNewlines()
	for !p.curTokenIs(lexer.TokenDedent) && !p.curTokenIs(lexer.TokenEOF) {
		if p.curTokenIs(lexer.TokenIdent) {
			mPos := p.curPos()
			ident := p.curToken.Literal
			var mTypeParams []ast.TypeParam
			if p.peekTokenIs(lexer.TokenLBracket) && isGenericMethodLookahead(p.peekToken.Literal+p.l.PeekRestOfLine()) {
				mTypeParams = p.parseTypeParams()
			}
			if p.peekTokenIs(lexer.TokenLParen) {
				// Method signature: ident(params...): ReturnType
				p.nextToken() // advance to '('
				params, blockParam := p.parseFuncParams()
				retType := "()"
				if p.curTokenIs(lexer.TokenColon) {
					p.nextToken() // consume ':'
					retType = p.parseTypeAnnotString()
				} else if p.peekTokenIs(lexer.TokenColon) {
					p.nextToken() // advance to ':'
					p.nextToken() // consume ':'
					retType = p.parseTypeAnnotString()
				}
				methods = append(methods, ast.MethodSig{
					Position:   mPos,
					Name:       ident,
					TypeParams: mTypeParams,
					Params:     params,
					BlockParam: blockParam,
					ReturnType: strings.TrimSpace(retType),
				})
				p.consumeStmtEnd()
			} else {
				// Embedded interface: e.g. Reader or Reader[T]
				var embName string
				if p.peekTokenIs(lexer.TokenLBracket) {
					embName = p.parseTypeAnnotString()
				} else {
					embName = ident
					p.nextToken() // consume ident
				}
				embedded = append(embedded, embName)
				p.consumeStmtEnd()
			}
		} else if isOperatorMethodToken(p.curToken.Type) {
			opPos := p.curPos()
			opName := p.parseOperatorMethodName()
			if opName != "" && p.expectPeek(lexer.TokenLParen) {
				params, blockParam := p.parseFuncParams()
				retType := "()"
				if p.curTokenIs(lexer.TokenColon) {
					p.nextToken() // consume ':'
					retType = p.parseTypeAnnotString()
				} else if p.peekTokenIs(lexer.TokenColon) {
					p.nextToken() // advance to ':'
					p.nextToken() // consume ':'
					retType = p.parseTypeAnnotString()
				}
				methods = append(methods, ast.MethodSig{
					Position:   opPos,
					Name:       opName,
					Params:     params,
					BlockParam: blockParam,
					ReturnType: strings.TrimSpace(retType),
				})
				p.consumeStmtEnd()
			}
		} else {
			p.nextToken()
		}
		p.skipNewlines()
	}

	if p.curTokenIs(lexer.TokenDedent) {
		p.nextToken()
	}

	return &ast.InterfaceDef{
		Position:   pos,
		Name:       name,
		TypeParams: typeParams,
		Embedded:   embedded,
		Methods:    methods,
	}
}

func (p *Parser) parseFuncDef() *ast.FuncDef {
	// def name(params): body
	pos := p.curPos()
	p.nextToken() // consume 'def'
	var name string
	if p.curTokenIs(lexer.TokenIdent) {
		name = p.curToken.Literal
	} else if isOperatorMethodToken(p.curToken.Type) {
		name = p.parseOperatorMethodName()
	} else {
		p.addError(p.curToken.Line, p.curToken.Col, "expected function name after def")
		return nil
	}

	var typeParams []ast.TypeParam
	if p.peekTokenIs(lexer.TokenLBracket) {
		typeParams = p.parseTypeParams()
	}

	if !p.expectPeek(lexer.TokenLParen) {
		return nil
	}

	params, blockParam := p.parseFuncParams()

	returnType := ""
	if p.peekTokenIs(lexer.TokenArrow) {
		p.nextToken() // advance to '->'
		p.nextToken() // consume '->', advance to return type
		returnType = p.parseTypeAnnotString()
		if p.peekTokenIs(lexer.TokenColon) {
			p.nextToken() // advance to ':'
		}
	} else if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken() // advance to ':'
		// If followed by return type annot on same line instead of newline/indent:
		if !p.peekTokenIs(lexer.TokenNewline) && !p.peekTokenIs(lexer.TokenIndent) && !p.peekTokenIs(lexer.TokenEOF) {
			p.nextToken() // advance to return type
			returnType = p.parseTypeAnnotString()
			if p.peekTokenIs(lexer.TokenColon) {
				p.nextToken() // advance to ':'
			}
		}
	}

	body := p.parseBlockBody()

	return &ast.FuncDef{
		Position:   pos,
		Name:       name,
		TypeParams: typeParams,
		Params:     params,
		BlockParam: blockParam,
		ReturnType: strings.TrimSpace(returnType),
		Body:       body,
	}
}

func (p *Parser) parseFuncParams() ([]ast.FuncParam, *ast.FuncParam) {
	params := make([]ast.FuncParam, 0)
	var blockParam *ast.FuncParam

	if p.peekTokenIs(lexer.TokenRParen) {
		p.nextToken()
		return params, nil
	}

	p.nextToken()
	hasDefault := false

	for {
		// 1. Block param: &block [: Type] [= Expr]
		if p.curTokenIs(lexer.TokenAmpersand) {
			p.nextToken() // consume '&'
			if !p.curTokenIs(lexer.TokenIdent) {
				p.addError(p.curToken.Line, p.curToken.Col, "expected identifier after '&'")
				break
			}
			bp := &ast.FuncParam{
				Position: p.curPos(),
				Name:     p.curToken.Literal,
			}
			if p.peekTokenIs(lexer.TokenColon) {
				p.nextToken() // advance to ':'
				p.nextToken() // advance to type annot start
				bp.TypeAnnot = p.parseTypeAnnotString()
			}
			if p.peekTokenIs(lexer.TokenAssign) {
				p.nextToken() // advance to '='
				p.nextToken() // advance to expr start
				bp.DefaultValue = p.parseExpr(PREC_LOWEST)
			}
			blockParam = bp

			// &block must be the last parameter!
			if p.peekTokenIs(lexer.TokenComma) {
				p.nextToken() // consume ','
				if !p.peekTokenIs(lexer.TokenRParen) {
					p.addError(p.curToken.Line, p.curToken.Col, "block parameter &%s must be the last parameter", bp.Name)
				}
			}
			break
		}

		// 2. Normal / Variadic param
		paramPos := p.curPos()
		isVariadic := false
		if p.curTokenIs(lexer.TokenStar) {
			isVariadic = true
			p.nextToken() // consume '*'
		}
		if !p.curTokenIs(lexer.TokenIdent) {
			break
		}
		param := ast.FuncParam{
			Position:   paramPos,
			Name:       p.curToken.Literal,
			IsVariadic: isVariadic,
		}
		if p.peekTokenIs(lexer.TokenColon) {
			p.nextToken() // consume ':'
			p.nextToken() // advance to type annot start
			param.TypeAnnot = p.parseTypeAnnotString()
		}
		if p.peekTokenIs(lexer.TokenAssign) {
			if isVariadic {
				p.addError(p.curToken.Line, p.curToken.Col, "variadic parameter *%s cannot have a default value", param.Name)
			}
			p.nextToken() // advance to '='
			p.nextToken() // advance to expr start
			param.DefaultValue = p.parseExpr(PREC_LOWEST)
			hasDefault = true
		} else if !isVariadic {
			if hasDefault {
				p.addError(p.curToken.Line, p.curToken.Col, "required parameter '%s' cannot follow default parameter", param.Name)
			}
		}

		params = append(params, param)

		if p.peekTokenIs(lexer.TokenComma) {
			p.nextToken()
			if p.peekTokenIs(lexer.TokenRParen) {
				break
			}
			p.nextToken()
		} else {
			break
		}
	}

	p.expectPeek(lexer.TokenRParen)
	return params, blockParam
}

func (p *Parser) parseTypeAnnotString() string {
	tok := p.curToken
	var parts []string
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	for {
		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
			if p.curTokenIs(lexer.TokenComma) || p.curTokenIs(lexer.TokenRParen) || p.curTokenIs(lexer.TokenRBracket) || p.curTokenIs(lexer.TokenRBrace) || p.curTokenIs(lexer.TokenNewline) || p.curTokenIs(lexer.TokenSemicolon) || p.curTokenIs(lexer.TokenEOF) || p.curTokenIs(lexer.TokenAssign) || p.curTokenIs(lexer.TokenColon) {
				break
			}
		}
		if p.curTokenIs(lexer.TokenLParen) {
			parenDepth++
		} else if p.curTokenIs(lexer.TokenRParen) {
			parenDepth--
		} else if p.curTokenIs(lexer.TokenLBracket) {
			bracketDepth++
		} else if p.curTokenIs(lexer.TokenRBracket) {
			bracketDepth--
		} else if p.curTokenIs(lexer.TokenLBrace) {
			braceDepth++
		} else if p.curTokenIs(lexer.TokenRBrace) {
			braceDepth--
		}
		parts = append(parts, p.curToken.Literal)
		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
			if p.peekTokenIs(lexer.TokenComma) || p.peekTokenIs(lexer.TokenRParen) || p.peekTokenIs(lexer.TokenRBracket) || p.peekTokenIs(lexer.TokenRBrace) || p.peekTokenIs(lexer.TokenNewline) || p.peekTokenIs(lexer.TokenEOF) || p.peekTokenIs(lexer.TokenAssign) || p.peekTokenIs(lexer.TokenColon) {
				break
			}
			if p.peekTokenIs(lexer.TokenPipe) {
				rest := strings.TrimSpace(p.l.PeekRestOfLine())
				if rest == "" || strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, ",") || strings.HasPrefix(rest, "#") || strings.HasPrefix(rest, ";") {
					break
				}
			}
		}
		p.nextToken()
	}
	annot := normalizeTypeAnnot(parts)
	p.validateTypeAnnotation(annot, tok.Line, tok.Col)
	return annot
}

func (p *Parser) parseBlockBody() []ast.Stmt {
	if p.curTokenIs(lexer.TokenColon) {
		p.nextToken()
	} else if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken()
		p.nextToken()
	}
	p.skipNewlines()

	if !p.curTokenIs(lexer.TokenIndent) {
		return []ast.Stmt{}
	}
	p.nextToken() // consume TokenIndent

	body := make([]ast.Stmt, 0)
	p.skipNewlines()

	for !p.curTokenIs(lexer.TokenDedent) && !p.curTokenIs(lexer.TokenEOF) {
		prevTok := p.curToken
		stmt := p.parseStmt()
		if stmt != nil {
			body = append(body, stmt)
		}
		p.skipNewlines()
		if p.curToken == prevTok {
			p.nextToken()
		}
	}

	if p.curTokenIs(lexer.TokenDedent) {
		p.nextToken()
	}
	return body
}

func (p *Parser) parseIfStmt() *ast.IfStmt {
	pos := p.curPos()
	p.nextToken() // consume 'if'
	cond := p.parseExpr(PREC_LOWEST)

	thenBody := p.parseBlockBody()
	var elseBody []ast.Stmt

	p.skipNewlines()
	if p.curTokenIs(lexer.TokenElse) {
		p.nextToken() // consume 'else'
		elseBody = p.parseBlockBody()
	}

	return &ast.IfStmt{
		Position: pos,
		Cond:     cond,
		ThenBody: thenBody,
		ElseBody: elseBody,
	}
}

func (p *Parser) parseForStmt() *ast.ForStmt {
	pos := p.curPos()
	p.nextToken() // consume 'for'
	var cond ast.Expr
	if !p.curTokenIs(lexer.TokenColon) && !p.peekTokenIs(lexer.TokenColon) {
		cond = p.parseExpr(PREC_LOWEST)
	}

	p.inClosureDepth++
	body := p.parseBlockBody()
	p.inClosureDepth--

	return &ast.ForStmt{
		Position: pos,
		Cond:     cond,
		Body:     body,
	}
}

func (p *Parser) parseContinueStmt() *ast.ContinueStmt {
	pos := p.curPos()
	p.nextToken() // consume 'continue'
	p.consumeStmtEnd()
	return &ast.ContinueStmt{Position: pos}
}

func (p *Parser) parseMatchExpr() *ast.MatchExpr {
	pos := p.curPos()
	p.nextToken() // consume 'match'
	target := p.parseExpr(PREC_LOWEST)

	if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken() // consume ':'
	}

	p.skipNewlines()
	for p.curTokenIs(lexer.TokenNewline) {
		p.nextToken()
	}
	if p.peekTokenIs(lexer.TokenNewline) {
		p.nextToken()
	}
	if !p.expectPeek(lexer.TokenIndent) {
		return nil
	}
	p.nextToken() // consume TokenIndent

	var cases []ast.MatchCase
	p.skipNewlines()

	for !p.curTokenIs(lexer.TokenDedent) && !p.curTokenIs(lexer.TokenEOF) {
		if p.curTokenIs(lexer.TokenCase) {
			cPos := p.curPos()
			p.nextToken() // consume 'case'
			var pattern string
			var tuplePat *ast.TuplePattern

			if p.curTokenIs(lexer.TokenLParen) {
				tuplePat, pattern = p.parseTuplePattern()
			} else {
				pattern = p.parseTypeAnnotString()
			}

			if p.curTokenIs(lexer.TokenColon) {
				// curToken is ':'
			} else if p.peekTokenIs(lexer.TokenColon) {
				p.nextToken() // advance to ':'
			}

			// Parse case body statements
			body := p.parseBlockBody()
			cases = append(cases, ast.MatchCase{
				Position:     cPos,
				Pattern:      pattern,
				TuplePattern: tuplePat,
				Body:         body,
			})
			p.skipNewlines()
		} else {
			p.nextToken()
		}
	}

	if p.curTokenIs(lexer.TokenDedent) {
		p.nextToken()
	}

	return &ast.MatchExpr{
		Position: pos,
		Target:   target,
		Cases:    cases,
	}
}

func (p *Parser) parseTuplePattern() (*ast.TuplePattern, string) {
	pos := p.curPos()
	p.nextToken() // consume '('
	var rawParts []string
	rawParts = append(rawParts, "(")
	var elems []string
	var restName string

	for !p.curTokenIs(lexer.TokenRParen) && !p.curTokenIs(lexer.TokenEOF) {
		if p.curTokenIs(lexer.TokenDot) {
			// check for ...
			for p.curTokenIs(lexer.TokenDot) {
				p.nextToken()
			}
			rawParts = append(rawParts, "...")
			if p.curTokenIs(lexer.TokenIdent) {
				restName = p.curToken.Literal
				rawParts = append(rawParts, restName)
				p.nextToken()
			} else {
				restName = "_"
			}
			if p.curTokenIs(lexer.TokenComma) {
				p.nextToken()
			}
			break
		}

		if p.curTokenIs(lexer.TokenIdent) {
			name := p.curToken.Literal
			elems = append(elems, name)
			rawParts = append(rawParts, name)
			p.nextToken()
		} else {
			p.nextToken()
		}

		if p.curTokenIs(lexer.TokenComma) {
			rawParts = append(rawParts, ",")
			p.nextToken()
		} else {
			break
		}
	}

	if p.curTokenIs(lexer.TokenRParen) {
		rawParts = append(rawParts, ")")
		p.nextToken() // consume ')'
	}

	return &ast.TuplePattern{
		Position: pos,
		Elements: elems,
		RestName: restName,
	}, strings.Join(rawParts, "")
}

func (p *Parser) isMemberAssignOnCurrentLine() bool {
	rest := strings.TrimSpace(p.l.PeekRestOfLine())
	if len(rest) == 0 {
		return false
	}
	runes := []rune(rest)
	if !unicode.IsLetter(runes[0]) && runes[0] != '_' {
		return false
	}
	i := 1
	for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
		i++
	}
	afterIdent := strings.TrimSpace(string(runes[i:]))
	return strings.HasPrefix(afterIdent, "=") && !strings.HasPrefix(afterIdent, "==") && !strings.HasPrefix(afterIdent, "=>")
}

func (p *Parser) parseMemberAssignStmt() *ast.MemberAssignStmt {
	pos := p.curPos()
	target := &ast.Identifier{Position: pos, Name: p.curToken.Literal}
	p.nextToken() // advance to '.'
	p.nextToken() // consume '.', advance to member ident
	if !p.curTokenIs(lexer.TokenIdent) {
		p.addError(p.curToken.Line, p.curToken.Col, "expected member identifier after '.', got %s", lexer.FormatToken(p.curToken))
		return nil
	}
	member := p.curToken.Literal
	if !p.expectPeek(lexer.TokenAssign) {
		return nil
	}
	p.nextToken() // consume '='
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.MemberAssignStmt{
		Position: pos,
		Target:   target,
		Member:   member,
		Value:  val,
	}
}

func (p *Parser) hasAssignOnCurrentLine() bool {
	rest := p.l.PeekRestOfLine()
	return strings.Contains(rest, "=")
}

func (p *Parser) isUnpackParenDeclStart() bool {
	rest := p.l.PeekRestOfLine()
	idxClose := strings.Index(rest, ")")
	if idxClose == -1 {
		return false
	}
	idxAssign := strings.Index(rest[idxClose:], "=")
	return idxAssign != -1
}

func (p *Parser) parseUnpackDeclWithParen() *ast.UnpackDecl {
	pos := p.curPos()
	tuplePat, _ := p.parseTuplePattern()
	if p.peekTokenIs(lexer.TokenAssign) {
		p.nextToken()
	}
	if p.curTokenIs(lexer.TokenAssign) {
		p.nextToken() // consume '='
	}
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.UnpackDecl{
		Position: pos,
		Pattern:  tuplePat,
		Value:   val,
	}
}

func (p *Parser) parseUnpackDeclWithoutParen() *ast.UnpackDecl {
	pos := p.curPos()
	var elems []string
	var restName string
	for {
		if p.curTokenIs(lexer.TokenDot) {
			for p.curTokenIs(lexer.TokenDot) {
				p.nextToken()
			}
			if p.curTokenIs(lexer.TokenIdent) {
				restName = p.curToken.Literal
				p.nextToken()
			} else {
				restName = "_"
			}
			if p.curTokenIs(lexer.TokenComma) {
				p.nextToken()
			}
			break
		}
		if p.curTokenIs(lexer.TokenIdent) {
			elems = append(elems, p.curToken.Literal)
			p.nextToken()
		} else {
			break
		}
		if p.curTokenIs(lexer.TokenComma) {
			p.nextToken()
		} else {
			break
		}
	}
	if p.curTokenIs(lexer.TokenAssign) {
		p.nextToken() // consume '='
	}
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.UnpackDecl{
		Position: pos,
		Pattern: &ast.TuplePattern{
			Position: pos,
			Elements: elems,
			RestName: restName,
		},
		Value: val,
	}
}

func (p *Parser) consumeStmtEnd() {
	if p.curTokenIs(lexer.TokenNewline) || p.curTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	} else if p.peekTokenIs(lexer.TokenNewline) || p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
		p.nextToken()
	} else if p.peekTokenIs(lexer.TokenEOF) || p.peekTokenIs(lexer.TokenDedent) {
		p.nextToken()
	}
}

func (p *Parser) parseVarDecl(typeAnnot string) *ast.VarDecl {
	pos := p.curPos()
	name := p.curToken.Literal
	p.nextToken() // consume ident
	p.nextToken() // consume '='
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.VarDecl{
		Position:  pos,
		Name:      name,
		TypeAnnot: typeAnnot,
		Value:     val,
	}
}

func (p *Parser) parseAnnotatedVarDecl() *ast.VarDecl {
	pos := p.curPos()
	name := p.curToken.Literal
	p.nextToken() // consume ident
	p.nextToken() // consume ':'

	typeAnnot := p.parseTypeAnnotString()

	if !p.expectPeek(lexer.TokenAssign) {
		return nil
	}
	p.nextToken() // consume '='
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.VarDecl{
		Position:  pos,
		Name:      name,
		TypeAnnot: typeAnnot,
		Value:     val,
	}
}

func (p *Parser) parseReturnStmt() *ast.ReturnStmt {
	pos := p.curPos()
	p.nextToken() // consume 'return'
	if p.curTokenIs(lexer.TokenNewline) || p.curTokenIs(lexer.TokenSemicolon) || p.curTokenIs(lexer.TokenEOF) || p.curTokenIs(lexer.TokenDedent) {
		if p.curTokenIs(lexer.TokenSemicolon) {
			p.nextToken()
		}
		return &ast.ReturnStmt{Position: pos, Value: nil}
	}
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.ReturnStmt{Position: pos, Value: val}
}

func (p *Parser) parseBreakStmt() *ast.BreakStmt {
	pos := p.curPos()
	p.nextToken() // consume 'break'
	if p.curTokenIs(lexer.TokenNewline) || p.curTokenIs(lexer.TokenSemicolon) || p.curTokenIs(lexer.TokenEOF) || p.curTokenIs(lexer.TokenDedent) {
		if p.curTokenIs(lexer.TokenSemicolon) {
			p.nextToken()
		}
		return &ast.BreakStmt{Position: pos, Value: nil}
	}
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.BreakStmt{Position: pos, Value: val}
}

func exprEndsWithBlock(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.MatchExpr:
		return true
	case *ast.CallExpr:
		return e.Block != nil
	case *ast.LambdaExpr:
		return len(e.Body) > 0 && e.Expr == nil
	default:
		return false
	}
}

func (p *Parser) parseExprStmt() *ast.ExprStmt {
	pos := p.curPos()
	p.atStatementStart = true
	expr := p.parseExpr(PREC_LOWEST)
	p.atStatementStart = false
	if exprEndsWithBlock(expr) {
		p.skipNewlines()
		return &ast.ExprStmt{Position: pos, Value: expr}
	}
	p.consumeStmtEnd()
	return &ast.ExprStmt{Position: pos, Value: expr}
}


func (p *Parser) parseExpr(precedence int) ast.Expr {
	isStmtStart := p.atStatementStart
	p.atStatementStart = false // only top-level statement expression starts here

	prefix := p.parsePrefix()
	if prefix == nil {
		return nil
	}

	// Paren-omitted call: only at statement start, not inside nested expressions
	if isStmtStart && precedence == PREC_LOWEST && !p.inParenOmissionCall && p.canStartArgument() {
		_, isIdent := prefix.(*ast.Identifier)
		_, isModAccess := prefix.(*ast.ModuleAccessExpr)
		if isIdent || isModAccess {
			p.inParenOmissionCall = true
			args := p.parseCallArgumentsWithoutParens()
			p.inParenOmissionCall = false

			var block *ast.BlockLiteral
			if p.peekTokenIs(lexer.TokenDo) {
				p.nextToken()
				block = p.parseBlockLiteral()
			}
			prefix = &ast.CallExpr{
				Callee: prefix,
				Args:   args,
				Block:  block,
			}
		}
	}

	if lam, isLam := prefix.(*ast.LambdaExpr); isLam && lam.Expr == nil {
		return prefix
	}

	for !p.peekTokenIs(lexer.TokenNewline) && !p.peekTokenIs(lexer.TokenSemicolon) && !p.peekTokenIs(lexer.TokenEOF) && precedence < p.peekPrecedence() {
		p.nextToken()
		prefix = p.parseInfix(prefix)
	}

	return prefix
}

func (p *Parser) canStartArgument() bool {
	// If peek is on the same line and can start an argument expression
	// Exclude '(' because 'f(...)' is standard call handled by parseInfix
	t := p.peekToken.Type
	switch t {
	case lexer.TokenLBracket:
		// If there is no whitespace between identifier and '[', it's an indexing expression target[idx], not a call argument
		if p.peekToken.Line == p.curToken.Line && p.peekToken.Col == p.curToken.Col+len(p.curToken.Literal) {
			return false
		}
		return true
	case lexer.TokenLParen:
		// If there is whitespace between identifier and '(', it's a call argument: print (42).Type(), not a standard f(...) call
		if p.peekToken.Line == p.curToken.Line && p.peekToken.Col > p.curToken.Col+len(p.curToken.Literal) {
			return true
		}
		return false
	case lexer.TokenIdent, lexer.TokenInt, lexer.TokenFloat, lexer.TokenString, lexer.TokenBool, lexer.TokenNil,
		lexer.TokenLBrace, lexer.TokenAt, lexer.TokenMatch, lexer.TokenForce:
		return true
	default:
		return false
	}
}

func (p *Parser) parseCallArgumentsWithoutParens() []ast.Expr {
	args := make([]ast.Expr, 0)
	p.nextToken() // move to first token of argument
	firstArg := p.parseExpr(PREC_LOWEST)
	args = append(args, firstArg)

	for p.peekTokenIs(lexer.TokenComma) {
		p.nextToken() // consume ','
		p.nextToken() // advance to next arg
		args = append(args, p.parseExpr(PREC_LOWEST))
	}

	return args
}

func (p *Parser) parsePrefix() ast.Expr {
	pos := p.curPos()
	switch p.curToken.Type {
	case lexer.TokenIdent:
		if p.curToken.Literal == "scriptLine" {
			return &ast.IntLit{Position: pos, Value: int64(p.curToken.Line)}
		}
		return &ast.Identifier{Position: pos, Name: p.curToken.Literal}
	case lexer.TokenInt:
		lit := strings.ReplaceAll(p.curToken.Literal, "_", "")
		base := 10
		digits := lit
		if len(lit) >= 2 && lit[0] == '0' {
			switch lit[1] {
			case 'b', 'B':
				base = 2
				digits = lit[2:]
			case 'o', 'O':
				base = 8
				digits = lit[2:]
			case 'd', 'D':
				base = 10
				digits = lit[2:]
			case 'x', 'X':
				base = 16
				digits = lit[2:]
			}
		}
		if v, err := strconv.ParseInt(digits, base, 64); err == nil {
			return &ast.IntLit{Position: pos, Value: v}
		}
		if bi, ok := new(big.Int).SetString(digits, base); ok {
			if bi.IsInt64() {
				return &ast.IntLit{Position: pos, Value: bi.Int64()}
			}
			return &ast.IntLit{Position: pos, BigValue: bi}
		}
		p.addError(pos.Line, pos.Col, "invalid integer literal '%s'", p.curToken.Literal)
		return &ast.IntLit{Position: pos, Value: 0}
	case lexer.TokenFloat:
		lit := strings.ReplaceAll(p.curToken.Literal, "_", "")
		v, _ := strconv.ParseFloat(lit, 64)
		return &ast.FloatLit{Position: pos, Value: v}
	case lexer.TokenString:
		return &ast.StringLit{Position: pos, Value: p.curToken.Literal}
	case lexer.TokenBool:
		return &ast.BoolLit{Position: pos, Value: p.curToken.Literal == "true"}
	case lexer.TokenNil:
		return &ast.NilLit{Position: pos}
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenNot, lexer.TokenTilde:
		op := p.curToken.Literal
		p.nextToken()
		right := p.parseExpr(PREC_UNARY)
		return &ast.UnaryExpr{Position: pos, Op: op, Expr: right}
	case lexer.TokenForce:
		p.nextToken()
		right := p.parseExpr(PREC_UNARY)
		return &ast.ForceExpr{Position: pos, Expr: right}
	case lexer.TokenLParen:
		return p.parseGroupOrTuple()
	case lexer.TokenLBracket:
		// Could be list or index, but in Les: arrays are tuple or dict. Allow bracketed tuples/lists if needed
		return p.parseTuple(lexer.TokenRBracket)
	case lexer.TokenAt:
		// @module.Member
		p.nextToken() // consume '@'
		if !p.curTokenIs(lexer.TokenIdent) {
			p.addError(p.curToken.Line, p.curToken.Col, "expected module identifier after '@', got %s", lexer.FormatToken(p.curToken))
			return nil
		}
		modName := p.curToken.Literal
		if !p.expectPeek(lexer.TokenDot) {
			return nil
		}
		p.nextToken() // consume '.'
		if !p.curTokenIs(lexer.TokenIdent) {
			p.addError(p.curToken.Line, p.curToken.Col, "expected member identifier after '.', got %s", lexer.FormatToken(p.curToken))
			return nil
		}
		memberName := p.curToken.Literal
		return &ast.ModuleAccessExpr{
			Position: pos,
			Module:   modName,
			Member:   memberName,
		}
	case lexer.TokenLBrace:
		return p.parseDict()
	case lexer.TokenMatch:
		return p.parseMatchExpr()
	case lexer.TokenPipe, lexer.TokenOr:
		return p.parseLambdaExpr()
	default:
		p.addError(p.curToken.Line, p.curToken.Col, "unexpected token %s", lexer.FormatToken(p.curToken))
		return nil
	}
}

func (p *Parser) parseLambdaExpr() *ast.LambdaExpr {
	pos := p.curPos()
	p.inClosureDepth++
	defer func() { p.inClosureDepth-- }()

	var params []ast.FuncParam

	if p.curTokenIs(lexer.TokenOr) {
		// 0-argument lambda: ||:
		p.nextToken() // consume '||'
	} else if p.curTokenIs(lexer.TokenPipe) {
		p.nextToken() // consume opening '|'
		if p.curTokenIs(lexer.TokenPipe) {
			// 0-argument lambda: | |:
			p.nextToken() // consume closing '|'
		} else {
			// parse parameter list
			for !p.curTokenIs(lexer.TokenPipe) && !p.curTokenIs(lexer.TokenEOF) {
				paramPos := p.curPos()
				isVariadic := false
				if p.curTokenIs(lexer.TokenStar) {
					isVariadic = true
					p.nextToken() // consume '*'
				}
				if !p.curTokenIs(lexer.TokenIdent) {
					break
				}
				paramName := p.curToken.Literal
				typeAnnot := ""
				if p.peekTokenIs(lexer.TokenColon) {
					p.nextToken() // consume ':'
					p.nextToken() // advance to type annot start
					typeAnnot = p.parseTypeAnnotString()
				}
				params = append(params, ast.FuncParam{
					Position:   paramPos,
					Name:       paramName,
					TypeAnnot:  typeAnnot,
					IsVariadic: isVariadic,
				})

				if p.peekTokenIs(lexer.TokenComma) {
					p.nextToken() // consume ','
					if p.peekTokenIs(lexer.TokenPipe) {
						break
					}
					p.nextToken() // advance to next param
				} else {
					break
				}
			}
			if p.curTokenIs(lexer.TokenPipe) {
				p.nextToken() // consume closing '|'
			} else if p.peekTokenIs(lexer.TokenPipe) {
				p.nextToken() // advance to closing '|'
				p.nextToken() // consume closing '|'
			}
		}
	}

	// Expect ':'
	if p.curTokenIs(lexer.TokenColon) {
		// curToken is ':'
	} else if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken() // advance to ':'
	}

	// Check if multi-line block (newline after colon) or single-line inline expression
	if p.peekTokenIs(lexer.TokenNewline) {
		body := p.parseBlockBody()
		return &ast.LambdaExpr{
			Position: pos,
			Params:   params,
			Body:     body,
		}
	}

	// Single-line inline expression
	if p.curTokenIs(lexer.TokenColon) {
		p.nextToken() // consume ':'
	}
	expr := p.parseExpr(PREC_LOWEST)
	return &ast.LambdaExpr{
		Position: pos,
		Params:   params,
		Body:     []ast.Stmt{&ast.ExprStmt{Position: pos, Value: expr}},
		Expr:     expr,
	}
}

func (p *Parser) parseGroupOrTuple() ast.Expr {
	pos := p.curPos()
	// ( ) -> empty tuple
	if p.peekTokenIs(lexer.TokenRParen) {
		p.nextToken() // consume ')'
		return &ast.TupleLit{Position: pos, Elements: []ast.Expr{}}
	}

	p.nextToken() // advance to first element
	first := p.parseExpr(PREC_LOWEST)

	if p.peekTokenIs(lexer.TokenComma) {
		// Tuple with elements
		elems := []ast.Expr{first}
		for p.peekTokenIs(lexer.TokenComma) {
			p.nextToken() // consume ','
			if p.peekTokenIs(lexer.TokenRParen) {
				break // single tuple trailing comma: (1,)
			}
			p.nextToken()
			elems = append(elems, p.parseExpr(PREC_LOWEST))
		}
		p.expectPeek(lexer.TokenRParen)
		return &ast.TupleLit{Position: pos, Elements: elems}
	}

	p.expectPeek(lexer.TokenRParen)
	// Regular grouped expression: (a + b)
	return first
}

func (p *Parser) parseTuple(closing lexer.TokenType) ast.Expr {
	pos := p.curPos()
	elems := make([]ast.Expr, 0)
	if p.peekTokenIs(closing) {
		p.nextToken()
		return &ast.TupleLit{Position: pos, Elements: elems}
	}
	p.nextToken()
	elems = append(elems, p.parseExpr(PREC_LOWEST))
	for p.peekTokenIs(lexer.TokenComma) {
		p.nextToken()
		if p.peekTokenIs(closing) {
			break
		}
		p.nextToken()
		elems = append(elems, p.parseExpr(PREC_LOWEST))
	}
	p.expectPeek(closing)
	return &ast.TupleLit{Position: pos, Elements: elems}
}

func (p *Parser) parseDict() ast.Expr {
	pos := p.curPos()
	entries := make([]ast.DictEntry, 0)
	if p.peekTokenIs(lexer.TokenRBrace) {
		p.nextToken()
		return &ast.DictLit{Position: pos, Entries: entries}
	}

	p.nextToken()
	for {
		ePos := p.curPos()
		key := p.parseExpr(PREC_LOWEST)
		if !p.expectPeek(lexer.TokenColon) {
			return nil
		}
		p.nextToken()
		val := p.parseExpr(PREC_LOWEST)
		entries = append(entries, ast.DictEntry{Position: ePos, Key: key, Value: val})

		if p.peekTokenIs(lexer.TokenComma) {
			p.nextToken()
			if p.peekTokenIs(lexer.TokenRBrace) {
				break
			}
			p.nextToken()
		} else {
			break
		}
	}
	p.expectPeek(lexer.TokenRBrace)
	return &ast.DictLit{Position: pos, Entries: entries}
}

func (p *Parser) peekPrecedence() int {
	switch p.peekToken.Type {
	case lexer.TokenPipeGreater:
		return PREC_PIPE
	case lexer.TokenQuestion:
		return PREC_TERNARY
	case lexer.TokenOr:
		return PREC_LOGICAL_OR
	case lexer.TokenAnd:
		return PREC_LOGICAL_AND
	case lexer.TokenEq, lexer.TokenNotEq:
		return PREC_EQUALITY
	case lexer.TokenIs:
		return PREC_IS
	case lexer.TokenLt, lexer.TokenLte, lexer.TokenGt, lexer.TokenGte:
		return PREC_COMPARISON
	case lexer.TokenPipe:
		return PREC_BIT_OR
	case lexer.TokenCaret:
		return PREC_BIT_XOR
	case lexer.TokenAmpersand:
		return PREC_BIT_AND
	case lexer.TokenLShift, lexer.TokenRShift:
		return PREC_SHIFT
	case lexer.TokenPlus, lexer.TokenMinus:
		return PREC_SUM
	case lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent:
		return PREC_PRODUCT
	case lexer.TokenLParen:
		return PREC_CALL
	case lexer.TokenLBracket:
		return PREC_INDEX
	case lexer.TokenDot:
		return PREC_DOT
	case lexer.TokenNot:
		return PREC_POSTFIX
	default:
		return PREC_LOWEST
	}
}

func (p *Parser) curPrecedence() int {
	switch p.curToken.Type {
	case lexer.TokenPipeGreater:
		return PREC_PIPE
	case lexer.TokenQuestion:
		return PREC_TERNARY
	case lexer.TokenOr:
		return PREC_LOGICAL_OR
	case lexer.TokenAnd:
		return PREC_LOGICAL_AND
	case lexer.TokenEq, lexer.TokenNotEq:
		return PREC_EQUALITY
	case lexer.TokenIs:
		return PREC_IS
	case lexer.TokenLt, lexer.TokenLte, lexer.TokenGt, lexer.TokenGte:
		return PREC_COMPARISON
	case lexer.TokenPipe:
		return PREC_BIT_OR
	case lexer.TokenCaret:
		return PREC_BIT_XOR
	case lexer.TokenAmpersand:
		return PREC_BIT_AND
	case lexer.TokenLShift, lexer.TokenRShift:
		return PREC_SHIFT
	case lexer.TokenPlus, lexer.TokenMinus:
		return PREC_SUM
	case lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent:
		return PREC_PRODUCT
	case lexer.TokenLParen:
		return PREC_CALL
	case lexer.TokenLBracket:
		return PREC_INDEX
	case lexer.TokenDot:
		return PREC_DOT
	case lexer.TokenNot:
		return PREC_POSTFIX
	default:
		return PREC_LOWEST
	}
}

func (p *Parser) parseInfix(left ast.Expr) ast.Expr {
	pos := p.curPos()
	switch p.curToken.Type {
	case lexer.TokenDot:
		p.nextToken() // consume '.'
		var member string
		if p.curTokenIs(lexer.TokenIdent) {
			member = p.curToken.Literal
		} else if isOperatorMethodToken(p.curToken.Type) {
			member = p.parseOperatorMethodName()
		} else {
			p.addError(p.curToken.Line, p.curToken.Col, "expected identifier after '.', got %s", lexer.FormatToken(p.curToken))
			return nil
		}
		return &ast.MemberAccessExpr{
			Position: pos,
			Target:   left,
			Member:   member,
		}
	case lexer.TokenQuestion:
		// cond ? then : else
		p.nextToken()
		thenExpr := p.parseExpr(PREC_TERNARY)
		p.expectPeek(lexer.TokenColon)
		p.nextToken()
		elseExpr := p.parseExpr(PREC_TERNARY - 1)
		return &ast.TernaryExpr{
			Position: pos,
			Cond:     left,
			Then:     thenExpr,
			Else:     elseExpr,
		}
	case lexer.TokenIs:
		tok := p.curToken
		p.nextToken() // consume 'is'
		var typeParts []string
		parenDepth := 0
		bracketDepth := 0
		braceDepth := 0
		for {
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				if !p.curTokenIs(lexer.TokenIdent) && !p.curTokenIs(lexer.TokenNil) && !p.curTokenIs(lexer.TokenPipe) && !p.curTokenIs(lexer.TokenLParen) && !p.curTokenIs(lexer.TokenLBracket) && !p.curTokenIs(lexer.TokenLBrace) && !p.curTokenIs(lexer.TokenDot) && !p.curTokenIs(lexer.TokenAt) && !p.curTokenIs(lexer.TokenArrow) {
					break
				}
			}
			if p.curTokenIs(lexer.TokenLParen) {
				parenDepth++
			} else if p.curTokenIs(lexer.TokenRParen) {
				parenDepth--
			} else if p.curTokenIs(lexer.TokenLBracket) {
				bracketDepth++
			} else if p.curTokenIs(lexer.TokenRBracket) {
				bracketDepth--
			} else if p.curTokenIs(lexer.TokenLBrace) {
				braceDepth++
			} else if p.curTokenIs(lexer.TokenRBrace) {
				braceDepth--
			}
			typeParts = append(typeParts, p.curToken.Literal)
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				if p.curTokenIs(lexer.TokenPipe) || p.curTokenIs(lexer.TokenAt) || p.curTokenIs(lexer.TokenDot) || p.curTokenIs(lexer.TokenArrow) {
					// Must continue to next component
				} else if p.peekTokenIs(lexer.TokenPipe) || p.peekTokenIs(lexer.TokenDot) || p.peekTokenIs(lexer.TokenArrow) || p.peekTokenIs(lexer.TokenLBracket) {
					// Continue if followed by '|', '.', '->', or '[' (to capture invalid brackets for error reporting)
				} else {
					break
				}
			}
			p.nextToken()
		}
		annot := normalizeTypeAnnot(typeParts)
		p.validateTypeAnnotation(annot, tok.Line, tok.Col)
		return &ast.IsExpr{
			Position:  pos,
			Expr:      left,
			TypeAnnot: annot,
		}
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent,
		lexer.TokenEq, lexer.TokenNotEq, lexer.TokenLt, lexer.TokenLte, lexer.TokenGt, lexer.TokenGte,
		lexer.TokenAnd, lexer.TokenOr,
		lexer.TokenPipe, lexer.TokenCaret, lexer.TokenAmpersand,
		lexer.TokenLShift, lexer.TokenRShift:
		op := p.curToken.Literal
		prec := p.curPrecedence()
		p.nextToken()
		right := p.parseExpr(prec)
		return &ast.BinaryExpr{
			Position: pos,
			Op:       op,
			Left:     left,
			Right:    right,
		}
	case lexer.TokenLParen:
		// f(a, b)
		args := p.parseCallArguments()
		var block *ast.BlockLiteral
		if p.peekTokenIs(lexer.TokenDo) {
			p.nextToken()
			block = p.parseBlockLiteral()
		}
		return &ast.CallExpr{
			Position: pos,
			Callee:   left,
			Args:     args,
			Block:    block,
		}
	case lexer.TokenLBracket:
		// target[index] or target[TypeArgs...]
		rest := p.peekToken.Literal + " " + p.l.PeekRestOfLine()
		isArrow := hasArrowBeforeRBracket(rest)
		p.nextToken() // consume '['
		if p.curTokenIs(lexer.TokenRBracket) {
			p.addError(p.curToken.Line, p.curToken.Col, "bracket expression cannot be empty")
			return nil
		}
		if isArrow {
			var typeArgs []string
			for {
				arg := p.parseTypeAnnotString()
				typeArgs = append(typeArgs, arg)
				if p.peekTokenIs(lexer.TokenComma) {
					p.nextToken() // advance to ','
					if p.peekTokenIs(lexer.TokenRBracket) {
						break
					}
					p.nextToken() // advance to start of next type annot
				} else {
					break
				}
			}
			if !p.expectPeek(lexer.TokenRBracket) {
				return nil
			}
			return &ast.TypeInstExpr{
				Position: pos,
				Target:   left,
				TypeArgs: typeArgs,
			}
		}
		idx := p.parseExpr(PREC_LOWEST)
		if p.peekTokenIs(lexer.TokenComma) {
			firstArg := exprToTypeAnnot(idx)
			if firstArg == "" {
				p.addError(p.peekToken.Line, p.peekToken.Col, "unexpected ',' in index expression")
				return nil
			}
			typeArgs := []string{firstArg}
			for p.peekTokenIs(lexer.TokenComma) {
				p.nextToken() // advance to ','
				if p.peekTokenIs(lexer.TokenRBracket) {
					break
				}
				p.nextToken() // advance to start of next type annot
				arg := p.parseTypeAnnotString()
				typeArgs = append(typeArgs, arg)
			}
			if !p.expectPeek(lexer.TokenRBracket) {
				return nil
			}
			return &ast.TypeInstExpr{
				Position: pos,
				Target:   left,
				TypeArgs: typeArgs,
			}
		}
		if !p.expectPeek(lexer.TokenRBracket) {
			return nil
		}
		typeArg := exprToTypeAnnot(idx)
		return &ast.IndexExpr{
			Position: pos,
			Target:   left,
			Index:    idx,
			TypeArg:  typeArg,
		}
	case lexer.TokenPipeGreater:
		// left |> right => right(left)
		prec := p.curPrecedence()
		isOp := isOperatorMethodToken(p.peekToken.Type)
		if p.peekToken.Type == lexer.TokenPipe && !strings.HasPrefix(strings.TrimSpace(p.l.PeekRestOfLine()), "(") {
			isOp = false
		}
		if isOp {
			p.nextToken() // advance to operator token
			opName := p.parseOperatorMethodName()
			if !p.peekTokenIs(lexer.TokenLParen) {
				// Standalone operator on RHS of pipe: left |> - desugars to -(left)
				if opName == "+" || opName == "-" || opName == "~" {
					return &ast.UnaryExpr{
						Position: pos,
						Op:       opName,
						Expr:     left,
					}
				}
				p.addError(p.curToken.Line, p.curToken.Col, "unexpected operator %s on RHS of pipe", opName)
				return nil
			}
			// Operator call on RHS of pipe: left |> op(args...)
			p.nextToken() // advance to '('
			args := p.parseCallArguments()
			if opName == "[]" {
				if len(args) == 1 {
					return &ast.IndexExpr{
						Position: pos,
						Target:   left,
						Index:    args[0],
					}
				}
			}
			if len(args) == 1 {
				return &ast.BinaryExpr{
					Position: pos,
					Op:       opName,
					Left:     left,
					Right:    args[0],
				}
			}
			// General call on operator method if other arity (e.g. 0 args: left |> -() )
			return &ast.CallExpr{
				Position: pos,
				Callee: &ast.MemberAccessExpr{
					Position: pos,
					Target:   left,
					Member:   opName,
				},
				Args: args,
			}
		}
		p.nextToken()
		right := p.parseExpr(prec)
		if call, isCall := right.(*ast.CallExpr); isCall {
			newArgs := make([]ast.Expr, 0, len(call.Args)+1)
			newArgs = append(newArgs, left)
			newArgs = append(newArgs, call.Args...)
			return &ast.CallExpr{
				Position: pos,
				Callee:   call.Callee,
				Args:     newArgs,
				Block:    call.Block,
			}
		}
		return &ast.CallExpr{
			Position: pos,
			Callee:   right,
			Args:     []ast.Expr{left},
		}
	case lexer.TokenNot:
		// expr! (postfix force)
		return &ast.ForceExpr{
			Position: pos,
			Expr:     left,
		}
	default:
		return left
	}
}

func (p *Parser) parseCallArgumentItem() ast.Expr {
	if p.curTokenIs(lexer.TokenIdent) && p.peekTokenIs(lexer.TokenColon) {
		pos := p.curPos()
		name := p.curToken.Literal
		p.nextToken() // consume ident
		p.nextToken() // consume ':'
		val := p.parseExpr(PREC_LOWEST)
		return &ast.NamedArgExpr{
			Position: pos,
			Name:     name,
			Value:    val,
		}
	}
	return p.parseExpr(PREC_LOWEST)
}

func (p *Parser) parseCallArguments() []ast.Expr {
	args := make([]ast.Expr, 0)
	if p.peekTokenIs(lexer.TokenRParen) {
		p.nextToken()
		return args
	}

	p.nextToken()
	args = append(args, p.parseCallArgumentItem())

	for p.peekTokenIs(lexer.TokenComma) {
		p.nextToken() // consume ','
		if p.peekTokenIs(lexer.TokenRParen) {
			break
		}
		p.nextToken()
		args = append(args, p.parseCallArgumentItem())
	}

	p.expectPeek(lexer.TokenRParen)
	return args
}

func (p *Parser) parseBlockLiteral() *ast.BlockLiteral {
	pos := p.curPos()
	p.inClosureDepth++
	defer func() { p.inClosureDepth-- }()

	// 'do' already consumed or at 'do'
	params := make([]ast.FuncParam, 0)
	if p.peekTokenIs(lexer.TokenPipe) {
		p.nextToken() // consume '|'
		p.nextToken()
		for !p.curTokenIs(lexer.TokenPipe) && !p.curTokenIs(lexer.TokenEOF) {
			paramPos := p.curPos()
			isVariadic := false
			if p.curTokenIs(lexer.TokenStar) {
				isVariadic = true
				p.nextToken()
			}
			if !p.curTokenIs(lexer.TokenIdent) {
				break
			}
			paramName := p.curToken.Literal
			typeAnnot := ""
			if p.peekTokenIs(lexer.TokenColon) {
				p.nextToken() // consume ':'
				p.nextToken() // advance to type annot start
				typeAnnot = p.parseTypeAnnotString()
			}
			params = append(params, ast.FuncParam{
				Position:   paramPos,
				Name:       paramName,
				TypeAnnot:  typeAnnot,
				IsVariadic: isVariadic,
			})
			if p.peekTokenIs(lexer.TokenComma) {
				p.nextToken()
				if p.peekTokenIs(lexer.TokenPipe) {
					break
				}
				p.nextToken()
			} else {
				break
			}
		}
		if !p.curTokenIs(lexer.TokenPipe) {
			p.expectPeek(lexer.TokenPipe)
		}
	}

	if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken()
	}

	body := p.parseBlockBody()
	return &ast.BlockLiteral{
		Position: pos,
		Params:   params,
		Body:     body,
	}
}

func (p *Parser) Errors() []string {
	return p.errors
}

func normalizeTypeAnnot(parts []string) string {
	var sb strings.Builder
	for i, part := range parts {
		if i > 0 {
			prev := parts[i-1]
			if !noSpaceAfter(prev) && !noSpaceBefore(part, prev) {
				sb.WriteString(" ")
			}
		}
		sb.WriteString(part)
	}
	return sb.String()
}

func noSpaceBefore(part, prev string) bool {
	switch part {
	case ")", "]", "}", ",", ":", ".":
		return true
	case "[":
		return prev != "|" && prev != "->" && prev != "," && prev != ":"
	default:
		return false
	}
}

func noSpaceAfter(prev string) bool {
	switch prev {
	case "(", "[", "{", "@", ".":
		return true
	default:
		return false
	}
}

func (p *Parser) validateTypeAnnotation(annot string, line, col int) {
	annot = strings.TrimSpace(annot)
	if annot == "" {
		return
	}
	if strings.Contains(annot, "Dict[") || strings.Contains(annot, "Dict [") {
		p.addError(line, col, "invalid type annotation '%s': 'Dict[...]' is not supported (use '{Key: Value}' for dictionaries)", annot)
		return
	}
	if strings.Contains(annot, "Tuple[") || strings.Contains(annot, "Tuple [") {
		p.addError(line, col, "invalid type annotation '%s': 'Tuple[...]' is not supported (use '(T1, T2)' for tuples)", annot)
		return
	}
	runes := []rune(annot)
	var braceStack []int
	var bracketStack []int
	for i, r := range runes {
		if r == '{' {
			braceStack = append(braceStack, i)
		} else if r == '}' {
			if len(braceStack) == 0 {
				p.addError(line, col, "unmatched '}' in type annotation '%s'", annot)
				return
			}
			startIdx := braceStack[len(braceStack)-1]
			braceStack = braceStack[:len(braceStack)-1]
			inner := string(runes[startIdx+1 : i])
			if !isValidDictAnnotationContent(inner) {
				p.addError(line, col, "invalid dictionary type annotation in '%s': expected '{Key: Value}'", annot)
				return
			}
		} else if r == '[' {
			bracketStack = append(bracketStack, i)
		} else if r == ']' {
			if len(bracketStack) == 0 {
				p.addError(line, col, "unmatched ']' in type annotation '%s'", annot)
				return
			}
			startIdx := bracketStack[len(bracketStack)-1]
			bracketStack = bracketStack[:len(bracketStack)-1]
			inner := strings.TrimSpace(string(runes[startIdx+1 : i]))
			if inner == "" {
				p.addError(line, col, "invalid type annotation '%s': empty type arguments '[]'", annot)
				return
			}
		}
	}
	if len(braceStack) > 0 {
		p.addError(line, col, "unclosed '{' in type annotation '%s'", annot)
		return
	}
	if len(bracketStack) > 0 {
		p.addError(line, col, "unclosed '[' in type annotation '%s'", annot)
		return
	}
}

func isValidDictAnnotationContent(inner string) bool {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return false
	}
	colonIdx := -1
	pDepth := 0
	bDepth := 0
	brDepth := 0
	runes := []rune(inner)
	for i, r := range runes {
		if r == '(' {
			pDepth++
		} else if r == ')' {
			pDepth--
		} else if r == '[' {
			bDepth++
		} else if r == ']' {
			bDepth--
		} else if r == '{' {
			brDepth++
		} else if r == '}' {
			brDepth--
		} else if r == ':' && pDepth == 0 && bDepth == 0 && brDepth == 0 {
			if colonIdx != -1 {
				// More than one top-level colon
				return false
			}
			colonIdx = i
		}
	}
	if colonIdx == -1 {
		return false
	}
	keyStr := strings.TrimSpace(string(runes[:colonIdx]))
	valStr := strings.TrimSpace(string(runes[colonIdx+1:]))
	return keyStr != "" && valStr != ""
}

func isOperatorMethodToken(t lexer.TokenType) bool {
	switch t {
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent,
		lexer.TokenEq, lexer.TokenLt, lexer.TokenLte, lexer.TokenGt, lexer.TokenGte,
		lexer.TokenTilde, lexer.TokenAmpersand, lexer.TokenPipe, lexer.TokenCaret,
		lexer.TokenLShift, lexer.TokenRShift,
		lexer.TokenLBracket:
		return true
	default:
		return false
	}
}

func (p *Parser) parseOperatorMethodName() string {
	switch p.curToken.Type {
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent,
		lexer.TokenEq, lexer.TokenLt, lexer.TokenLte, lexer.TokenGt, lexer.TokenGte,
		lexer.TokenTilde, lexer.TokenAmpersand, lexer.TokenPipe, lexer.TokenCaret,
		lexer.TokenLShift, lexer.TokenRShift:
		op := p.curToken.Literal
		return op
	case lexer.TokenLBracket:
		if p.peekTokenIs(lexer.TokenRBracket) {
			p.nextToken() // consume ']'
			return "[]"
		}
		return ""
	default:
		return ""
	}
}

func (p *Parser) parseTypeParams() []ast.TypeParam {
	p.nextToken() // advance to '['
	var tps []ast.TypeParam
	if p.peekTokenIs(lexer.TokenRBracket) {
		p.addError(p.peekToken.Line, p.peekToken.Col, "type parameter list cannot be empty")
		p.nextToken() // advance to ']'
		return nil
	}
	p.nextToken() // advance to first param
	for {
		if !p.curTokenIs(lexer.TokenIdent) {
			p.addError(p.curToken.Line, p.curToken.Col, "expected type parameter name, got %s", p.curToken.Literal)
			break
		}
		tp := ast.TypeParam{
			Position: p.curPos(),
			Name:     p.curToken.Literal,
		}
		if p.peekTokenIs(lexer.TokenColon) {
			p.nextToken() // advance to ':'
			p.nextToken() // advance to constraint start
			tp.Constraint = p.parseTypeAnnotString()
		}
		tps = append(tps, tp)
		if p.peekTokenIs(lexer.TokenComma) {
			p.nextToken() // advance to ','
			if p.peekTokenIs(lexer.TokenRBracket) {
				break
			}
			p.nextToken() // advance to next param name
		} else {
			break
		}
	}
	if !p.expectPeek(lexer.TokenRBracket) {
		return nil
	}
	seen := make(map[string]bool)
	for _, tp := range tps {
		if seen[tp.Name] {
			p.addError(tp.Position.Line, tp.Position.Col, "duplicate type parameter '%s'", tp.Name)
		}
		seen[tp.Name] = true
	}
	return tps
}

func isGenericMethodLookahead(rest string) bool {
	depth := 0
	runes := []rune(rest)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '[' {
			depth++
		} else if r == ']' {
			depth--
			if depth == 0 {
				for j := i + 1; j < len(runes); j++ {
					if runes[j] == ' ' || runes[j] == '\t' {
						continue
					}
					return runes[j] == '('
				}
				return false
			}
		}
	}
	return false
}

func hasArrowBeforeRBracket(rest string) bool {
	depth := 1
	runes := []rune(rest)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '[' {
			depth++
		} else if r == ']' {
			depth--
			if depth <= 0 {
				return false
			}
		} else if depth == 1 && r == '-' && i+1 < len(runes) && runes[i+1] == '>' {
			return true
		}
	}
	return false
}

func exprToTypeAnnot(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		return e.Name
	case *ast.NilLit:
		return "Nil"
	case *ast.ModuleAccessExpr:
		return "@" + e.Module + "." + e.Member
	case *ast.BinaryExpr:
		if e.Op == "|" {
			left := exprToTypeAnnot(e.Left)
			right := exprToTypeAnnot(e.Right)
			if left != "" && right != "" {
				return left + " | " + right
			}
		}
		return ""
	case *ast.IndexExpr:
		target := exprToTypeAnnot(e.Target)
		if target != "" && e.TypeArg != "" {
			return target + "[" + e.TypeArg + "]"
		}
		return ""
	case *ast.TypeInstExpr:
		target := exprToTypeAnnot(e.Target)
		if target != "" && len(e.TypeArgs) > 0 {
			return target + "[" + strings.Join(e.TypeArgs, ", ") + "]"
		}
		return ""
	case *ast.TupleLit:
		var elems []string
		for _, el := range e.Elements {
			t := exprToTypeAnnot(el)
			if t == "" {
				return ""
			}
			elems = append(elems, t)
		}
		return "(" + strings.Join(elems, ", ") + ")"
	default:
		return ""
	}
}
