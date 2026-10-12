package parser

import (
	"fmt"
	"strconv"
	"strings"

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
	PREC_SUM          // + -
	PREC_PRODUCT      // * / %
	PREC_UNARY        // - !
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
	p.errors = append(p.errors, fmt.Sprintf("expected next token %v, got %v (%s) at line %d col %d", t, p.peekToken.Type, p.peekToken.Literal, p.peekToken.Line, p.peekToken.Col))
	return false
}

func (p *Parser) skipNewlines() {
	for p.curTokenIs(lexer.TokenNewline) || p.curTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	}
}

func (p *Parser) ParseProgram() *ast.Program {
	prog := &ast.Program{Body: make([]ast.Stmt, 0)}
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
			p.errors = append(p.errors, fmt.Sprintf("break outside of closure or loop at line %d col %d", p.curToken.Line, p.curToken.Col))
		}
		return p.parseBreakStmt()
	case lexer.TokenContinue:
		if p.inClosureDepth <= 0 {
			p.errors = append(p.errors, fmt.Sprintf("continue outside of loop at line %d col %d", p.curToken.Line, p.curToken.Col))
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
	p.nextToken() // consume 'import'
	var path string
	if p.curTokenIs(lexer.TokenIdent) || p.curTokenIs(lexer.TokenString) {
		path = p.curToken.Literal
	} else {
		p.errors = append(p.errors, fmt.Sprintf("expected identifier or string after import, got %v (%s)", p.curToken.Type, p.curToken.Literal))
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
		Path:  path,
		Alias: alias,
	}
}

func (p *Parser) parseModuleDecl() *ast.ModuleDecl {
	p.nextToken() // consume 'module'
	if !p.curTokenIs(lexer.TokenIdent) {
		p.errors = append(p.errors, fmt.Sprintf("expected identifier after module, got %v (%s)", p.curToken.Type, p.curToken.Literal))
		return nil
	}
	name := p.curToken.Literal
	if p.peekTokenIs(lexer.TokenNewline) {
		p.nextToken()
		p.nextToken()
	} else {
		p.nextToken()
	}
	return &ast.ModuleDecl{Name: name}
}

func (p *Parser) parseStructDef() *ast.StructDef {
	p.nextToken() // consume 'struct'
	if !p.curTokenIs(lexer.TokenIdent) {
		p.errors = append(p.errors, "expected struct name after struct")
		return nil
	}
	name := p.curToken.Literal

	if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken() // advance to ':'
	} else {
		p.errors = append(p.errors, "expected ':' after struct name")
		return nil
	}

	p.nextToken() // consume ':'
	p.skipNewlines()

	if !p.curTokenIs(lexer.TokenIndent) {
		p.errors = append(p.errors, fmt.Sprintf("expected indent after struct declaration at line %d", p.curToken.Line))
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
					Name:         ident,
					TypeAnnot:    typeAnnot,
					DefaultValue: defaultVal,
					IsEmbedded:   false,
				})
				p.consumeStmtEnd()
			} else {
				// Embedded struct type: e.g. Person
				fields = append(fields, ast.StructField{
					Name:       ident,
					TypeAnnot:  ident,
					IsEmbedded: true,
				})
				p.nextToken() // consume ident
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

	return &ast.StructDef{
		Name:    name,
		Fields:  fields,
		Methods: methods,
	}
}

func (p *Parser) parseInterfaceDef() *ast.InterfaceDef {
	p.nextToken() // consume 'interface'
	if !p.curTokenIs(lexer.TokenIdent) {
		p.errors = append(p.errors, "expected interface name after interface")
		return nil
	}
	name := p.curToken.Literal

	if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken() // advance to ':'
	} else {
		p.errors = append(p.errors, "expected ':' after interface name")
		return nil
	}

	p.nextToken() // consume ':'
	p.skipNewlines()

	if !p.curTokenIs(lexer.TokenIndent) {
		p.errors = append(p.errors, fmt.Sprintf("expected indent after interface declaration at line %d", p.curToken.Line))
		return nil
	}
	p.nextToken() // consume TokenIndent

	var embedded []string
	var methods []ast.MethodSig

	p.skipNewlines()
	for !p.curTokenIs(lexer.TokenDedent) && !p.curTokenIs(lexer.TokenEOF) {
		if p.curTokenIs(lexer.TokenIdent) {
			ident := p.curToken.Literal
			if p.peekTokenIs(lexer.TokenLParen) {
				// Method signature: ident(params...): ReturnType
				p.nextToken() // advance to '('
				params := p.parseFuncParams()
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
					Name:       ident,
					Params:     params,
					ReturnType: strings.TrimSpace(retType),
				})
				p.consumeStmtEnd()
			} else {
				// Embedded interface: e.g. Reader
				embedded = append(embedded, ident)
				p.nextToken() // consume ident
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
		Name:     name,
		Embedded: embedded,
		Methods:  methods,
	}
}

func (p *Parser) parseFuncDef() *ast.FuncDef {
	// def name(params): body
	p.nextToken() // consume 'def'
	if !p.curTokenIs(lexer.TokenIdent) {
		p.errors = append(p.errors, "expected function name after def")
		return nil
	}
	name := p.curToken.Literal

	if !p.expectPeek(lexer.TokenLParen) {
		return nil
	}

	params := p.parseFuncParams()

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
		Name:       name,
		Params:     params,
		ReturnType: strings.TrimSpace(returnType),
		Body:       body,
	}
}

func (p *Parser) parseFuncParams() []ast.FuncParam {
	params := make([]ast.FuncParam, 0)
	if p.peekTokenIs(lexer.TokenRParen) {
		p.nextToken()
		return params
	}

	p.nextToken()
	for {
		isVariadic := false
		if p.curTokenIs(lexer.TokenStar) {
			isVariadic = true
			p.nextToken() // consume '*'
		}
		if !p.curTokenIs(lexer.TokenIdent) {
			break
		}
		param := ast.FuncParam{
			Name:       p.curToken.Literal,
			IsVariadic: isVariadic,
		}
		if p.peekTokenIs(lexer.TokenColon) {
			p.nextToken() // consume ':'
			p.nextToken() // advance to type annot start
			param.TypeAnnot = p.parseTypeAnnotString()
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
	return params
}

func (p *Parser) parseTypeAnnotString() string {
	var parts []string
	parenDepth := 0
	bracketDepth := 0
	for {
		if parenDepth == 0 && bracketDepth == 0 {
			if p.curTokenIs(lexer.TokenComma) || p.curTokenIs(lexer.TokenRParen) || p.curTokenIs(lexer.TokenNewline) || p.curTokenIs(lexer.TokenSemicolon) || p.curTokenIs(lexer.TokenEOF) || p.curTokenIs(lexer.TokenAssign) || p.curTokenIs(lexer.TokenColon) {
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
		}
		parts = append(parts, p.curToken.Literal)
		if parenDepth == 0 && bracketDepth == 0 {
			if p.peekTokenIs(lexer.TokenComma) || p.peekTokenIs(lexer.TokenRParen) || p.peekTokenIs(lexer.TokenNewline) || p.peekTokenIs(lexer.TokenEOF) || p.peekTokenIs(lexer.TokenAssign) || p.peekTokenIs(lexer.TokenColon) {
				break
			}
			if p.peekTokenIs(lexer.TokenPipe) {
				rest := strings.TrimSpace(p.l.PeekRestOfLine())
				if strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, ",") {
					break
				}
			}
		}
		p.nextToken()
	}
	return normalizeTypeAnnot(parts)
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
		p.errors = append(p.errors, fmt.Sprintf("expected token %v, got %v (%s) at line %d col %d", lexer.TokenIndent, p.curToken.Type, p.curToken.Literal, p.curToken.Line, p.curToken.Col))
		return nil
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
		Cond:     cond,
		ThenBody: thenBody,
		ElseBody: elseBody,
	}
}

func (p *Parser) parseForStmt() *ast.ForStmt {
	p.nextToken() // consume 'for'
	var cond ast.Expr
	if !p.curTokenIs(lexer.TokenColon) && !p.peekTokenIs(lexer.TokenColon) {
		cond = p.parseExpr(PREC_LOWEST)
	}

	p.inClosureDepth++
	body := p.parseBlockBody()
	p.inClosureDepth--

	return &ast.ForStmt{
		Cond: cond,
		Body: body,
	}
}

func (p *Parser) parseContinueStmt() *ast.ContinueStmt {
	p.nextToken() // consume 'continue'
	p.consumeStmtEnd()
	return &ast.ContinueStmt{}
}

func (p *Parser) parseMatchExpr() *ast.MatchExpr {
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
			p.nextToken() // consume 'case'
			var pattern string
			var tuplePat *ast.TuplePattern

			if p.curTokenIs(lexer.TokenLParen) {
				tuplePat, pattern = p.parseTuplePattern()
			} else {
				pattern = p.curToken.Literal
				p.nextToken() // consume pattern
			}

			if p.curTokenIs(lexer.TokenColon) {
				// curToken is ':'
			} else if p.peekTokenIs(lexer.TokenColon) {
				p.nextToken() // advance to ':'
			}

			// Parse case body statements
			body := p.parseBlockBody()
			cases = append(cases, ast.MatchCase{
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
		Target: target,
		Cases:  cases,
	}
}

func (p *Parser) parseTuplePattern() (*ast.TuplePattern, string) {
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
		Elements: elems,
		RestName: restName,
	}, strings.Join(rawParts, "")
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
		Pattern: tuplePat,
		Value:   val,
	}
}

func (p *Parser) parseUnpackDeclWithoutParen() *ast.UnpackDecl {
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
		Pattern: &ast.TuplePattern{
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
	name := p.curToken.Literal
	p.nextToken() // consume ident
	p.nextToken() // consume '='
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.VarDecl{
		Name:      name,
		TypeAnnot: typeAnnot,
		Value:     val,
	}
}

func (p *Parser) parseAnnotatedVarDecl() *ast.VarDecl {
	name := p.curToken.Literal
	p.nextToken() // consume ident
	p.nextToken() // consume ':'

	// Collect type annotation tokens until '='
	var typeAnnotParts []string
	for !p.curTokenIs(lexer.TokenAssign) && !p.curTokenIs(lexer.TokenEOF) && !p.curTokenIs(lexer.TokenNewline) && !p.curTokenIs(lexer.TokenSemicolon) {
		typeAnnotParts = append(typeAnnotParts, p.curToken.Literal)
		p.nextToken()
	}
	typeAnnot := strings.Join(typeAnnotParts, "")

	if !p.curTokenIs(lexer.TokenAssign) {
		p.errors = append(p.errors, "expected '=' in variable declaration")
		return nil
	}
	p.nextToken() // consume '='
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.VarDecl{
		Name:      name,
		TypeAnnot: typeAnnot,
		Value:     val,
	}
}

func (p *Parser) parseReturnStmt() *ast.ReturnStmt {
	p.nextToken() // consume 'return'
	if p.curTokenIs(lexer.TokenNewline) || p.curTokenIs(lexer.TokenSemicolon) || p.curTokenIs(lexer.TokenEOF) || p.curTokenIs(lexer.TokenDedent) {
		if p.curTokenIs(lexer.TokenSemicolon) {
			p.nextToken()
		}
		return &ast.ReturnStmt{Value: nil}
	}
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.ReturnStmt{Value: val}
}

func (p *Parser) parseBreakStmt() *ast.BreakStmt {
	p.nextToken() // consume 'break'
	if p.curTokenIs(lexer.TokenNewline) || p.curTokenIs(lexer.TokenSemicolon) || p.curTokenIs(lexer.TokenEOF) || p.curTokenIs(lexer.TokenDedent) {
		if p.curTokenIs(lexer.TokenSemicolon) {
			p.nextToken()
		}
		return &ast.BreakStmt{Value: nil}
	}
	val := p.parseExpr(PREC_LOWEST)
	p.consumeStmtEnd()
	return &ast.BreakStmt{Value: val}
}

func (p *Parser) parseExprStmt() *ast.ExprStmt {
	p.atStatementStart = true
	expr := p.parseExpr(PREC_LOWEST)
	p.atStatementStart = false
	if _, isMatch := expr.(*ast.MatchExpr); isMatch {
		p.skipNewlines()
		return &ast.ExprStmt{Value: expr}
	}
	if p.curTokenIs(lexer.TokenNewline) || p.curTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
	} else if p.peekTokenIs(lexer.TokenNewline) || p.peekTokenIs(lexer.TokenSemicolon) {
		p.nextToken()
		p.nextToken()
	} else if !p.curTokenIs(lexer.TokenDedent) && !p.curTokenIs(lexer.TokenEOF) {
		p.nextToken()
	}
	return &ast.ExprStmt{Value: expr}
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
	case lexer.TokenIdent, lexer.TokenInt, lexer.TokenFloat, lexer.TokenString, lexer.TokenBool, lexer.TokenNil,
		lexer.TokenLBracket, lexer.TokenLBrace, lexer.TokenAt, lexer.TokenMatch:
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
	switch p.curToken.Type {
	case lexer.TokenIdent:
		if p.curToken.Literal == "scriptLine" {
			return &ast.IntLit{Value: int64(p.curToken.Line)}
		}
		return &ast.Identifier{Name: p.curToken.Literal}
	case lexer.TokenInt:
		v, _ := strconv.ParseInt(p.curToken.Literal, 10, 64)
		return &ast.IntLit{Value: v}
	case lexer.TokenFloat:
		v, _ := strconv.ParseFloat(p.curToken.Literal, 64)
		return &ast.FloatLit{Value: v}
	case lexer.TokenString:
		return &ast.StringLit{Value: p.curToken.Literal}
	case lexer.TokenBool:
		return &ast.BoolLit{Value: p.curToken.Literal == "true"}
	case lexer.TokenNil:
		return &ast.NilLit{}
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenNot:
		op := p.curToken.Literal
		p.nextToken()
		right := p.parseExpr(PREC_UNARY)
		return &ast.UnaryExpr{Op: op, Expr: right}
	case lexer.TokenLParen:
		return p.parseGroupOrTuple()
	case lexer.TokenLBracket:
		// Could be list or index, but in Les: arrays are tuple or dict. Allow bracketed tuples/lists if needed
		return p.parseTuple(lexer.TokenRBracket)
	case lexer.TokenAt:
		// @module.Member
		p.nextToken() // consume '@'
		if !p.curTokenIs(lexer.TokenIdent) {
			p.errors = append(p.errors, fmt.Sprintf("expected module identifier after '@', got %v (%s)", p.curToken.Type, p.curToken.Literal))
			return nil
		}
		modName := p.curToken.Literal
		if !p.expectPeek(lexer.TokenDot) {
			return nil
		}
		p.nextToken() // consume '.'
		if !p.curTokenIs(lexer.TokenIdent) {
			p.errors = append(p.errors, fmt.Sprintf("expected member identifier after '.', got %v (%s)", p.curToken.Type, p.curToken.Literal))
			return nil
		}
		memberName := p.curToken.Literal
		return &ast.ModuleAccessExpr{
			Module: modName,
			Member: memberName,
		}
	case lexer.TokenLBrace:
		return p.parseDict()
	case lexer.TokenMatch:
		return p.parseMatchExpr()
	case lexer.TokenPipe, lexer.TokenOr:
		return p.parseLambdaExpr()
	default:
		return nil
	}
}

func (p *Parser) parseLambdaExpr() *ast.LambdaExpr {
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
			Params: params,
			Body:   body,
		}
	}

	// Single-line inline expression
	if p.curTokenIs(lexer.TokenColon) {
		p.nextToken() // consume ':'
	}
	expr := p.parseExpr(PREC_LOWEST)
	return &ast.LambdaExpr{
		Params: params,
		Body:   []ast.Stmt{&ast.ExprStmt{Value: expr}},
		Expr:   expr,
	}
}

func (p *Parser) parseGroupOrTuple() ast.Expr {
	// ( ) -> empty tuple
	if p.peekTokenIs(lexer.TokenRParen) {
		p.nextToken() // consume ')'
		return &ast.TupleLit{Elements: []ast.Expr{}}
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
		return &ast.TupleLit{Elements: elems}
	}

	p.expectPeek(lexer.TokenRParen)
	// Regular grouped expression: (a + b)
	return first
}

func (p *Parser) parseTuple(closing lexer.TokenType) ast.Expr {
	elems := make([]ast.Expr, 0)
	if p.peekTokenIs(closing) {
		p.nextToken()
		return &ast.TupleLit{Elements: elems}
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
	return &ast.TupleLit{Elements: elems}
}

func (p *Parser) parseDict() ast.Expr {
	entries := make([]ast.DictEntry, 0)
	if p.peekTokenIs(lexer.TokenRBrace) {
		p.nextToken()
		return &ast.DictLit{Entries: entries}
	}

	p.nextToken()
	for {
		key := p.parseExpr(PREC_LOWEST)
		if !p.expectPeek(lexer.TokenColon) {
			return nil
		}
		p.nextToken()
		val := p.parseExpr(PREC_LOWEST)
		entries = append(entries, ast.DictEntry{Key: key, Value: val})

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
	return &ast.DictLit{Entries: entries}
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
	switch p.curToken.Type {
	case lexer.TokenDot:
		p.nextToken() // consume '.'
		if !p.curTokenIs(lexer.TokenIdent) {
			p.errors = append(p.errors, fmt.Sprintf("expected identifier after '.', got %v (%s)", p.curToken.Type, p.curToken.Literal))
			return nil
		}
		member := p.curToken.Literal
		return &ast.MemberAccessExpr{
			Target: left,
			Member: member,
		}
	case lexer.TokenQuestion:
		// cond ? then : else
		p.nextToken()
		thenExpr := p.parseExpr(PREC_TERNARY)
		p.expectPeek(lexer.TokenColon)
		p.nextToken()
		elseExpr := p.parseExpr(PREC_TERNARY - 1)
		return &ast.TernaryExpr{
			Cond: left,
			Then: thenExpr,
			Else: elseExpr,
		}
	case lexer.TokenIs:
		p.nextToken() // consume 'is'
		var typeParts []string
		parenDepth := 0
		bracketDepth := 0
		for {
			if parenDepth == 0 && bracketDepth == 0 {
				if !p.curTokenIs(lexer.TokenIdent) && !p.curTokenIs(lexer.TokenNil) && !p.curTokenIs(lexer.TokenPipe) && !p.curTokenIs(lexer.TokenLParen) && !p.curTokenIs(lexer.TokenLBracket) && !p.curTokenIs(lexer.TokenDot) && !p.curTokenIs(lexer.TokenAt) && !p.curTokenIs(lexer.TokenArrow) {
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
			}
			typeParts = append(typeParts, p.curToken.Literal)
			if parenDepth == 0 && bracketDepth == 0 {
				if p.curTokenIs(lexer.TokenPipe) || p.curTokenIs(lexer.TokenAt) || p.curTokenIs(lexer.TokenDot) || p.curTokenIs(lexer.TokenArrow) {
					// Must continue to next component
				} else if p.peekTokenIs(lexer.TokenPipe) || p.peekTokenIs(lexer.TokenDot) || p.peekTokenIs(lexer.TokenArrow) {
					// Continue if followed by '|', '.', or '->'
				} else {
					break
				}
			}
			p.nextToken()
		}
		return &ast.IsExpr{
			Expr:      left,
			TypeAnnot: normalizeTypeAnnot(typeParts),
		}
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent,
		lexer.TokenEq, lexer.TokenNotEq, lexer.TokenLt, lexer.TokenLte, lexer.TokenGt, lexer.TokenGte,
		lexer.TokenAnd, lexer.TokenOr:
		op := p.curToken.Literal
		prec := p.curPrecedence()
		p.nextToken()
		right := p.parseExpr(prec)
		return &ast.BinaryExpr{
			Op:    op,
			Left:  left,
			Right: right,
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
			Callee: left,
			Args:   args,
			Block:  block,
		}
	case lexer.TokenLBracket:
		// target[index]
		p.nextToken()
		idx := p.parseExpr(PREC_LOWEST)
		p.expectPeek(lexer.TokenRBracket)
		return &ast.IndexExpr{
			Target: left,
			Index:  idx,
		}
	case lexer.TokenPipeGreater:
		// left |> right => right(left)
		prec := p.curPrecedence()
		p.nextToken()
		right := p.parseExpr(prec)
		if call, isCall := right.(*ast.CallExpr); isCall {
			newArgs := make([]ast.Expr, 0, len(call.Args)+1)
			newArgs = append(newArgs, left)
			newArgs = append(newArgs, call.Args...)
			return &ast.CallExpr{
				Callee: call.Callee,
				Args:   newArgs,
				Block:  call.Block,
			}
		}
		return &ast.CallExpr{
			Callee: right,
			Args:   []ast.Expr{left},
		}
	case lexer.TokenNot:
		// expr! (postfix force)
		return &ast.ForceExpr{
			Expr: left,
		}
	default:
		return left
	}
}

func (p *Parser) parseCallArgumentItem() ast.Expr {
	if p.curTokenIs(lexer.TokenIdent) && p.peekTokenIs(lexer.TokenColon) {
		name := p.curToken.Literal
		p.nextToken() // consume ident
		p.nextToken() // consume ':'
		val := p.parseExpr(PREC_LOWEST)
		return &ast.NamedArgExpr{
			Name:  name,
			Value: val,
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
	p.inClosureDepth++
	defer func() { p.inClosureDepth-- }()

	// 'do' already consumed or at 'do'
	params := make([]ast.FuncParam, 0)
	if p.peekTokenIs(lexer.TokenPipe) {
		p.nextToken() // consume '|'
		p.nextToken()
		for {
			if !p.curTokenIs(lexer.TokenIdent) {
				break
			}
			params = append(params, ast.FuncParam{Name: p.curToken.Literal})
			if p.peekTokenIs(lexer.TokenComma) {
				p.nextToken()
				p.nextToken()
			} else {
				break
			}
		}
		p.expectPeek(lexer.TokenPipe)
	}

	if p.peekTokenIs(lexer.TokenColon) {
		p.nextToken()
	}

	body := p.parseBlockBody()
	return &ast.BlockLiteral{
		Params: params,
		Body:   body,
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
			if prev != "@" && prev != "." && part != "." && part != "|" && prev != "|" && part != "->" && prev != "->" {
				sb.WriteString(" ")
			} else if part == "|" || prev == "|" || part == "->" || prev == "->" {
				sb.WriteString(" ")
			}
		}
		sb.WriteString(part)
	}
	return sb.String()
}
