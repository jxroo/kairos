package tools

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

func calcDef() ToolDefinition {
	return ToolDefinition{
		Name:        "calc",
		Description: "Evaluate a mathematical expression (arithmetic only: +, -, *, /, ^, parentheses)",
		InputSchema: map[string]Param{
			"expression": {Type: "string", Description: "Math expression to evaluate", Required: true},
		},
		Builtin: true,
	}
}

func calcHandler() ToolHandler {
	return func(ctx context.Context, args map[string]any) (*ToolResult, error) {
		expr, _ := args["expression"].(string)
		if expr == "" {
			return &ToolResult{Content: "expression is required", IsError: true}, nil
		}
		val, err := evalExpr(expr)
		if err != nil {
			return &ToolResult{Content: fmt.Sprintf("eval error: %v", err), IsError: true}, nil
		}
		// Format nicely: if integer, no decimals.
		if val == math.Trunc(val) && !math.IsInf(val, 0) {
			return &ToolResult{Content: strconv.FormatFloat(val, 'f', 0, 64)}, nil
		}
		return &ToolResult{Content: strconv.FormatFloat(val, 'f', -1, 64)}, nil
	}
}

// Simple recursive-descent parser for arithmetic expressions.
// Supports: +, -, *, /, ^, unary -, parentheses, decimal numbers.
type parser struct {
	input string
	pos   int
}

func evalExpr(expr string) (float64, error) {
	p := &parser{input: strings.TrimSpace(expr)}
	val, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.pos < len(p.input) {
		return 0, fmt.Errorf("unexpected character at position %d: %q", p.pos, string(p.input[p.pos]))
	}
	return val, nil
}

func (p *parser) parseExpr() (float64, error) {
	return p.parseAddSub()
}

func (p *parser) parseAddSub() (float64, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.input) {
			break
		}
		op := p.input[p.pos]
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		right, err := p.parseMulDiv()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			left += right
		} else {
			left -= right
		}
	}
	return left, nil
}

func (p *parser) parseMulDiv() (float64, error) {
	left, err := p.parsePow()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.input) {
			break
		}
		op := p.input[p.pos]
		if op != '*' && op != '/' {
			break
		}
		p.pos++
		right, err := p.parsePow()
		if err != nil {
			return 0, err
		}
		if op == '*' {
			left *= right
		} else {
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
		}
	}
	return left, nil
}

func (p *parser) parsePow() (float64, error) {
	base, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.pos < len(p.input) && p.input[p.pos] == '^' {
		p.pos++
		exp, err := p.parsePow() // right-associative
		if err != nil {
			return 0, err
		}
		return math.Pow(base, exp), nil
	}
	return base, nil
}

func (p *parser) parseUnary() (float64, error) {
	p.skipSpaces()
	if p.pos < len(p.input) && p.input[p.pos] == '-' {
		p.pos++
		val, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return -val, nil
	}
	if p.pos < len(p.input) && p.input[p.pos] == '+' {
		p.pos++
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (float64, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	if p.input[p.pos] == '(' {
		p.pos++
		val, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}
	return p.parseNumber()
}

func (p *parser) parseNumber() (float64, error) {
	p.skipSpaces()
	start := p.pos
	for p.pos < len(p.input) && (unicode.IsDigit(rune(p.input[p.pos])) || p.input[p.pos] == '.') {
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("expected number at position %d", p.pos)
	}
	val, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q: %w", p.input[start:p.pos], err)
	}
	return val, nil
}

func (p *parser) skipSpaces() {
	for p.pos < len(p.input) && p.input[p.pos] == ' ' {
		p.pos++
	}
}
