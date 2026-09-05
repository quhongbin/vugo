package expr

import "fmt"

// Parse 把一个类 Vue 表达式解析为表达式 AST。
func Parse(src string) (Expr, error) {
	tokens, err := (&scanner{src: src}).scan()
	if err != nil {
		return nil, err
	}
	p := &exprParser{tokens: tokens, src: src}
	e, err := p.parseBinary(0)
	if err != nil {
		return nil, err
	}
	if tok := p.peek(); tok.kind != tokEOF {
		return nil, p.errorf(tok, "多余的 %q", tok.val)
	}
	return e, nil
}

// precedence 是二元运算符优先级，数值越大越先结合。
var precedence = map[string]int{
	"||": 1,
	"&&": 2,
	"==": 3, "!=": 3, ">": 3, "<": 3, ">=": 3, "<=": 3,
	"+": 4, "-": 4,
	"*": 5, "/": 5, "%": 5,
}

// exprParser 用递归下降法解析表达式。
type exprParser struct {
	tokens []token
	pos    int
	src    string
}

// parseBinary 解析二元运算，minPrec 为当前可接受的最低优先级。
func (p *exprParser) parseBinary(minPrec int) (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		tok := p.peek()
		if tok.kind != tokOperator {
			return left, nil
		}
		prec, ok := precedence[tok.val]
		if !ok || prec < minPrec {
			return left, nil
		}
		p.advance()
		right, err := p.parseBinary(prec + 1)
		if err != nil {
			return nil, err
		}
		left = &Binary{Op: tok.val, L: left, R: right, at: tok.pos}
	}
}

// parseUnary 解析一元运算。
func (p *exprParser) parseUnary() (Expr, error) {
	tok := p.peek()
	if tok.kind != tokOperator {
		return p.parsePrimary()
	}
	switch tok.val {
	case "!":
		p.advance()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &Unary{Op: tok.val, X: x, at: tok.pos}, nil

	case "-":
		// html/template 没有一元负号，只把紧跟数字的写法当作负数字面量
		if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].kind == tokNumber {
			p.advance()
			num := p.peek()
			p.advance()
			return &Literal{Value: "-" + num.val, at: tok.pos}, nil
		}
		return nil, p.errorf(tok, "html/template 不支持一元负号，请改写成 0 - x")

	default:
		return p.parsePrimary()
	}
}

// parsePrimary 解析原子表达式，并处理其后缀（函数调用、字段访问、下标/切片）。
func (p *exprParser) parsePrimary() (Expr, error) {
	tok := p.peek()
	switch tok.kind {
	case tokNumber, tokString:
		p.advance()
		return &Literal{Value: tok.val, at: tok.pos}, nil

	case tokPunct:
		if tok.val != "(" {
			return nil, p.errorf(tok, "意外的 %q", tok.val)
		}
		p.advance()
		inner, err := p.parseBinary(0)
		if err != nil {
			return nil, err
		}
		if next := p.peek(); next.kind != tokPunct || next.val != ")" {
			return nil, p.errorf(next, "缺少右括号 )")
		}
		p.advance()
		return &Paren{X: inner, at: tok.pos}, nil

	case tokIdent:
		p.advance()
		ident := &Ident{Name: tok.val, at: tok.pos}
		return p.parsePostfix(ident)

	default:
		return nil, p.errorf(tok, "表达式不能以 %q 开头", tok.val)
	}
}

// parsePostfix 处理变量之后的调用、取字段与下标。
func (p *exprParser) parsePostfix(ident *Ident) (Expr, error) {
	for {
		switch tok := p.peek(); {
		case tok.kind == tokPunct && tok.val == ".":
			p.advance()
			field := p.peek()
			if field.kind != tokIdent {
				return nil, p.errorf(field, "\".\" 之后需要字段名")
			}
			p.advance()
			ident.Path = append(ident.Path, field.val)

		case tok.kind == tokPunct && tok.val == "(":
			p.advance()
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			return &Call{Func: dottedName(ident), Args: args, at: ident.at}, nil

		default:
			return ident, nil
		}
	}
}

// parseArgs 解析函数调用的参数列表，遇到右括号停止。
func (p *exprParser) parseArgs() ([]Expr, error) {
	var args []Expr
	if tok := p.peek(); tok.kind == tokPunct && tok.val == ")" {
		p.advance()
		return nil, nil
	}
	for {
		arg, err := p.parseBinary(0)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		tok := p.peek()
		if tok.kind == tokPunct && tok.val == "," {
			p.advance()
			continue
		}
		if tok.kind == tokPunct && tok.val == ")" {
			p.advance()
			return args, nil
		}
		return nil, p.errorf(tok, "参数列表应以 , 或 ) 结束")
	}
}

// dottedName 把 Ident 还原成点分名称，用于函数调用名。
func dottedName(ident *Ident) string {
	name := ident.Name
	for _, part := range ident.Path {
		name += "." + part
	}
	return name
}

func (p *exprParser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{kind: tokEOF, pos: len(p.src)}
	}
	return p.tokens[p.pos]
}

func (p *exprParser) advance() {
	if p.pos < len(p.tokens) {
		p.pos++
	}
}

func (p *exprParser) errorf(tok token, format string, args ...any) error {
	return fmt.Errorf("vugo: 表达式 %q 第 %d 列: %s", p.src, tok.pos+1, fmt.Sprintf(format, args...))
}
