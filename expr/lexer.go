package expr

import (
	"fmt"
	"unicode"
)

// TokenType classifies lexer tokens.
type TokenType int

const (
	TokEOF TokenType = iota
	TokInt
	TokIdent
	TokTrue
	TokFalse
	TokNot
	TokAnd
	TokOr
	TokIf
	TokThen
	TokElse
	TokEq    // ==
	TokNeq   // !=
	TokLt    // <
	TokLe    // <=
	TokGt    // >
	TokGe    // >=
	TokPlus
	TokMinus
	TokStar
	TokSlash
	TokPercent
	TokLParen
	TokRParen
	TokComma
)

// Token is a single lexer token.
type Token struct {
	Type TokenType
	Val  string
	Pos  int
}

var keywords = map[string]TokenType{
	"true":  TokTrue,
	"false": TokFalse,
	"not":   TokNot,
	"and":   TokAnd,
	"or":    TokOr,
	"if":    TokIf,
	"then":  TokThen,
	"else":  TokElse,
}

// Lex tokenizes an expression string.
func Lex(input string) ([]Token, error) {
	var tokens []Token
	i := 0
	for i < len(input) {
		ch := rune(input[i])

		// Skip whitespace.
		if unicode.IsSpace(ch) {
			i++
			continue
		}

		// Numbers.
		if unicode.IsDigit(ch) {
			start := i
			for i < len(input) && unicode.IsDigit(rune(input[i])) {
				i++
			}
			tokens = append(tokens, Token{TokInt, input[start:i], start})
			continue
		}

		// Identifiers and keywords.
		if unicode.IsLetter(ch) || ch == '_' {
			start := i
			for i < len(input) && (unicode.IsLetter(rune(input[i])) || unicode.IsDigit(rune(input[i])) || input[i] == '_') {
				i++
			}
			word := input[start:i]
			if tt, ok := keywords[word]; ok {
				tokens = append(tokens, Token{tt, word, start})
			} else {
				tokens = append(tokens, Token{TokIdent, word, start})
			}
			continue
		}

		// Two-character operators.
		if i+1 < len(input) {
			two := input[i : i+2]
			switch two {
			case "==":
				tokens = append(tokens, Token{TokEq, two, i})
				i += 2
				continue
			case "!=":
				tokens = append(tokens, Token{TokNeq, two, i})
				i += 2
				continue
			case "<=":
				tokens = append(tokens, Token{TokLe, two, i})
				i += 2
				continue
			case ">=":
				tokens = append(tokens, Token{TokGe, two, i})
				i += 2
				continue
			}
		}

		// Single-character operators.
		switch ch {
		case '<':
			tokens = append(tokens, Token{TokLt, "<", i})
		case '>':
			tokens = append(tokens, Token{TokGt, ">", i})
		case '+':
			tokens = append(tokens, Token{TokPlus, "+", i})
		case '-':
			tokens = append(tokens, Token{TokMinus, "-", i})
		case '*':
			tokens = append(tokens, Token{TokStar, "*", i})
		case '/':
			tokens = append(tokens, Token{TokSlash, "/", i})
		case '%':
			tokens = append(tokens, Token{TokPercent, "%", i})
		case '(':
			tokens = append(tokens, Token{TokLParen, "(", i})
		case ')':
			tokens = append(tokens, Token{TokRParen, ")", i})
		case ',':
			tokens = append(tokens, Token{TokComma, ",", i})
		default:
			return nil, fmt.Errorf("unexpected character %q at position %d", ch, i)
		}
		i++
	}
	tokens = append(tokens, Token{TokEOF, "", len(input)})
	return tokens, nil
}
