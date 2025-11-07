package core

import (
	"errors"
	"fmt"
	"strconv"
	"unicode"
)

// Token types
const (
	TokenNumber = iota
	TokenPlus
	TokenMinus
	TokenMultiply
	TokenDivide
	TokenLParen
	TokenRParen
	TokenIdentifier
	TokenAssign
	TokenEOF
)

type Token struct {
	Type  int
	Value string
}

type Lexer struct {
	input        string
	position     int
	readPosition int
	ch           byte
}

func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
}

func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' {
		l.readChar()
	}
}

func (l *Lexer) readNumber() string {
	position := l.position
	for unicode.IsDigit(rune(l.ch)) || l.ch == '.' {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	switch l.ch {
	case '+':
		tok = Token{Type: TokenPlus, Value: "+"}
	case '-':
		tok = Token{Type: TokenMinus, Value: "-"}
	case '*':
		tok = Token{Type: TokenMultiply, Value: "*"}
	case '/':
		tok = Token{Type: TokenDivide, Value: "/"}
	case '(':
		tok = Token{Type: TokenLParen, Value: "("}
	case ')':
		tok = Token{Type: TokenRParen, Value: ")"}
	case '=':
		tok = Token{Type: TokenAssign, Value: "="}
	case 0:
		tok = Token{Type: TokenEOF, Value: ""}
	default:
		if isLetter(l.ch) {
			tok.Value = l.readIdentifier()
			tok.Type = TokenIdentifier
			return tok
		} else if unicode.IsDigit(rune(l.ch)) || l.ch == '.' {
			tok.Value = l.readNumber()
			tok.Type = TokenNumber
			return tok
		} else {
			tok = Token{Type: -1, Value: string(l.ch)}
		}
	}

	l.readChar()
	return tok
}

// AST Nodes
type Node interface {
	Value(vars map[string]float64, strVars map[string]string) (interface{}, error)
}

type NumberNode struct {
	Val float64
}

func (n *NumberNode) Value(vars map[string]float64, strVars map[string]string) (interface{}, error) {
	return n.Val, nil
}

type VariableNode struct {
	Name string
}

func (v *VariableNode) Value(vars map[string]float64, strVars map[string]string) (interface{}, error) {
	if val, ok := vars[v.Name]; ok {
		return val, nil
	}
	if val, ok := strVars[v.Name]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("неизвестная переменная: %s", v.Name)
}

type BinaryOpNode struct {
	Left     Node
	Operator string
	Right    Node
}

func (b *BinaryOpNode) Value(vars map[string]float64, strVars map[string]string) (interface{}, error) {
	left, err := b.Left.Value(vars, strVars)
	if err != nil {
		return nil, err
	}
	right, err := b.Right.Value(vars, strVars)
	if err != nil {
		return nil, err
	}

	leftNum, ok1 := left.(float64)
	rightNum, ok2 := right.(float64)

	if !ok1 || !ok2 {
		return nil, errors.New("арифметические операции возможны только между числами")
	}

	switch b.Operator {
	case "+":
		return leftNum + rightNum, nil
	case "-":
		return leftNum - rightNum, nil
	case "*":
		return leftNum * rightNum, nil
	case "/":
		if rightNum == 0 {
			return nil, errors.New("деление на ноль")
		}
		return leftNum / rightNum, nil
	default:
		return nil, fmt.Errorf("неизвестный оператор: %s", b.Operator)
	}
}

type AssignmentNode struct {
	Variable string
	Expr     Node
}

func (a *AssignmentNode) Value(vars map[string]float64, strVars map[string]string) (interface{}, error) {
	right, err := a.Expr.Value(vars, strVars)
	if err != nil {
		return nil, err
	}

	if val, ok := right.(float64); ok {
		vars[a.Variable] = val
		return val, nil
	} else if val, ok := right.(string); ok {
		strVars[a.Variable] = val
		return val, nil
	} else {
		return nil, fmt.Errorf("неподдерживаемый тип для присваивания: %T", right)
	}
}

type Parser struct {
	lexer        *Lexer
	currentToken Token
	peekToken    Token
}

func NewParser(input string) *Parser {
	p := &Parser{lexer: NewLexer(input)}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.currentToken = p.peekToken
	p.peekToken = p.lexer.NextToken()
}

func (p *Parser) ParseExpression() (Node, error) {
	return p.parseAssignment()
}

func (p *Parser) parseAssignment() (Node, error) {
	node, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}

	if p.currentToken.Type == TokenAssign {
		varNode, ok := node.(*VariableNode)
		if !ok {
			return nil, errors.New("слева от '=' должно быть имя переменной")
		}
		varName := varNode.Name
		p.nextToken() // consume '='
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		return &AssignmentNode{Variable: varName, Expr: right}, nil
	}

	return node, nil
}

func (p *Parser) parseAdditive() (Node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}

	for p.currentToken.Type == TokenPlus || p.currentToken.Type == TokenMinus {
		op := p.currentToken.Value
		p.nextToken()
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Left: left, Operator: op, Right: right}
	}

	return left, nil
}

func (p *Parser) parseMultiplicative() (Node, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	for p.currentToken.Type == TokenMultiply || p.currentToken.Type == TokenDivide {
		op := p.currentToken.Value
		p.nextToken()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Left: left, Operator: op, Right: right}
	}

	return left, nil
}

func (p *Parser) parsePrimary() (Node, error) {
	switch p.currentToken.Type {
	case TokenNumber:
		val, err := strconv.ParseFloat(p.currentToken.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("некорректное число: %s", p.currentToken.Value)
		}
		p.nextToken()
		return &NumberNode{Val: val}, nil
	case TokenIdentifier:
		name := p.currentToken.Value
		p.nextToken()
		return &VariableNode{Name: name}, nil
	case TokenLParen:
		p.nextToken()
		expr, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		if p.currentToken.Type != TokenRParen {
			return nil, errors.New("ожидается ')'")
		}
		p.nextToken()
		return expr, nil
	case TokenMinus:
		p.nextToken()
		node, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &BinaryOpNode{
			Left:     &NumberNode{Val: 0},
			Operator: "-",
			Right:    node,
		}, nil
	default:
		return nil, fmt.Errorf("неожиданный токен: %s", p.currentToken.Value)
	}
}