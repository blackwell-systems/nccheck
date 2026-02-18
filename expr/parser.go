package expr

import (
	"fmt"
	"strconv"
)

// NodeType classifies AST nodes.
type NodeType int

const (
	NodeLitInt NodeType = iota
	NodeLitBool
	NodeVar
	NodeNot
	NodeAnd
	NodeOr
	NodeEq
	NodeNeq
	NodeLt
	NodeLe
	NodeGt
	NodeGe
	NodeAdd
	NodeSub
	NodeMul
	NodeDiv
	NodeMod
	NodeIf // if-then-else
	NodeCall
)

// Node is an AST node.
type Node struct {
	Type     NodeType
	IntVal   int
	BoolVal  bool
	Name     string // for Var, Call
	Children []*Node
}

// Parser is a Pratt parser for expressions.
type Parser struct {
	tokens []Token
	pos    int
}

// Parse parses an expression string into an AST.
func Parse(input string) (*Node, error) {
	tokens, err := Lex(input)
	if err != nil {
		return nil, err
	}
	p := &Parser{tokens: tokens}
	node, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	if p.peek().Type != TokEOF {
		return nil, fmt.Errorf("unexpected token %q at position %d", p.peek().Val, p.peek().Pos)
	}
	return node, nil
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	t := p.peek()
	p.pos++
	return t
}

func (p *Parser) expect(tt TokenType) (Token, error) {
	t := p.advance()
	if t.Type != tt {
		return t, fmt.Errorf("expected %d, got %q at position %d", tt, t.Val, t.Pos)
	}
	return t, nil
}

// Precedence levels.
const (
	precNone    = 0
	precOr      = 1
	precAnd     = 2
	precCompare = 3
	precAdd     = 4
	precMul     = 5
	precUnary   = 6
)

func (p *Parser) parseExpr(minPrec int) (*Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}

	for {
		tok := p.peek()
		prec, nodeType, ok := infixInfo(tok.Type)
		if !ok || prec < minPrec {
			break
		}

		// Handle 'if' as a special ternary.
		if tok.Type == TokIf {
			// left is already parsed (unused in standard ternary, but we handle
			// this differently: 'if' starts at unary level)
			break
		}

		p.advance()
		right, err := p.parseExpr(prec + 1) // left-associative
		if err != nil {
			return nil, err
		}
		left = &Node{Type: nodeType, Children: []*Node{left, right}}
	}

	return left, nil
}

func (p *Parser) parseUnary() (*Node, error) {
	tok := p.peek()

	// 'not' prefix
	if tok.Type == TokNot {
		p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &Node{Type: NodeNot, Children: []*Node{operand}}, nil
	}

	// 'if' ternary
	if tok.Type == TokIf {
		p.advance()
		cond, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TokThen); err != nil {
			return nil, fmt.Errorf("expected 'then' in if-then-else")
		}
		then, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TokElse); err != nil {
			return nil, fmt.Errorf("expected 'else' in if-then-else")
		}
		els, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		return &Node{Type: NodeIf, Children: []*Node{cond, then, els}}, nil
	}

	// Unary minus
	if tok.Type == TokMinus {
		p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		// Represent as 0 - operand
		return &Node{Type: NodeSub, Children: []*Node{
			{Type: NodeLitInt, IntVal: 0},
			operand,
		}}, nil
	}

	return p.parseAtom()
}

func (p *Parser) parseAtom() (*Node, error) {
	tok := p.advance()

	switch tok.Type {
	case TokInt:
		v, err := strconv.Atoi(tok.Val)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", tok.Val)
		}
		return &Node{Type: NodeLitInt, IntVal: v}, nil

	case TokTrue:
		return &Node{Type: NodeLitBool, BoolVal: true}, nil

	case TokFalse:
		return &Node{Type: NodeLitBool, BoolVal: false}, nil

	case TokIdent:
		// Check for function call: min, max, clamp.
		if p.peek().Type == TokLParen && isBuiltin(tok.Val) {
			return p.parseCall(tok.Val)
		}
		return &Node{Type: NodeVar, Name: tok.Val}, nil

	case TokLParen:
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TokRParen); err != nil {
			return nil, fmt.Errorf("expected closing ')'")
		}
		return expr, nil

	default:
		return nil, fmt.Errorf("unexpected token %q at position %d", tok.Val, tok.Pos)
	}
}

func (p *Parser) parseCall(name string) (*Node, error) {
	p.advance() // consume '('
	var args []*Node
	for {
		arg, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.peek().Type == TokComma {
			p.advance()
			continue
		}
		break
	}
	if _, err := p.expect(TokRParen); err != nil {
		return nil, fmt.Errorf("expected ')' after %s arguments", name)
	}

	// Validate arity.
	switch name {
	case "min", "max":
		if len(args) != 2 {
			return nil, fmt.Errorf("%s requires 2 arguments, got %d", name, len(args))
		}
	case "clamp":
		if len(args) != 3 {
			return nil, fmt.Errorf("clamp requires 3 arguments, got %d", len(args))
		}
	}

	return &Node{Type: NodeCall, Name: name, Children: args}, nil
}

func isBuiltin(name string) bool {
	return name == "min" || name == "max" || name == "clamp"
}

func infixInfo(tt TokenType) (prec int, nt NodeType, ok bool) {
	switch tt {
	case TokOr:
		return precOr, NodeOr, true
	case TokAnd:
		return precAnd, NodeAnd, true
	case TokEq:
		return precCompare, NodeEq, true
	case TokNeq:
		return precCompare, NodeNeq, true
	case TokLt:
		return precCompare, NodeLt, true
	case TokLe:
		return precCompare, NodeLe, true
	case TokGt:
		return precCompare, NodeGt, true
	case TokGe:
		return precCompare, NodeGe, true
	case TokPlus:
		return precAdd, NodeAdd, true
	case TokMinus:
		return precAdd, NodeSub, true
	case TokStar:
		return precMul, NodeMul, true
	case TokSlash:
		return precMul, NodeDiv, true
	case TokPercent:
		return precMul, NodeMod, true
	default:
		return 0, 0, false
	}
}
