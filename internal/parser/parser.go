// Package parser 是转译管道的第二层：把 lexer 产出的 token 流构造成 AST。
//
// 本层负责语法结构与"这是什么语法"的判定（指令 / 绑定 / 插值 / 原生 action），
// 但不负责把类 Vue 语法改写成 Go 语法，那是 transpiler 的职责。
package parser

import (
	"fmt"
	"strings"

	"github.com/quhongbin/vugo/internal/ast"
	"github.com/quhongbin/vugo/internal/lexer"
)

// ParseString 一步完成词法分析与语法分析，便于调用方直接传入模板源码。
func ParseString(src string) (*ast.Document, error) {
	tokens, err := lexer.New(src).Run()
	if err != nil {
		return nil, err
	}
	return Parse(tokens)
}

// Parse 把 token 流构造成 AST。
func Parse(tokens []lexer.Token) (*ast.Document, error) {
	p := &parser{tokens: tokens}
	return p.parseDocument()
}

// parser 持有一份 token 序列与当前读取位置。
type parser struct {
	tokens []lexer.Token
	pos    int
}

// goActionKeywords 是 html/template 的控制流关键字。
// 以这些词开头的 {{ }} 被视为原生 action，原样透传。
var goActionKeywords = map[string]bool{
	"if": true, "else": true, "end": true, "range": true, "with": true,
	"template": true, "define": true, "block": true, "break": true, "continue": true,
}

// goBuiltinFuncs 是 html/template 内置函数。
// 形如 `len .Items` 的写法（参数以 . 或 $ 开头）同样判定为原生 action。
var goBuiltinFuncs = map[string]bool{
	"and": true, "call": true, "html": true, "index": true, "slice": true, "js": true,
	"len": true, "not": true, "or": true, "print": true, "printf": true, "println": true,
	"urlquery": true, "eq": true, "ne": true, "lt": true, "le": true, "gt": true, "ge": true,
}

// isGoAction 判断 mustache 内容是否为原生 html/template action。
// 返回 false 时按类 Vue 表达式处理。
func isGoAction(code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return true
	}
	if c := code[0]; c == '.' || c == '$' {
		return true
	}
	// {{/* 注释 */}} 是原生模板注释
	if strings.HasPrefix(code, "/*") {
		return true
	}
	// 类 Vue 的函数调用形如 len(items)，首个空格前不会是纯关键字，
	// 因此用首词完全匹配即可区分两种风格。
	// 注意：内置函数名（index/len/printf...）也可能是用户的循环变量名，
	// 所以裸词不算原生 action，必须带参数（如 `len .items`）才透传。
	head, rest, _ := strings.Cut(code, " ")
	if goActionKeywords[head] {
		return true
	}
	return goBuiltinFuncs[head] && strings.TrimSpace(rest) != ""
}

// parseDocument 解析整份模板，维护一个开标签栈来还原嵌套结构。
func (p *parser) parseDocument() (*ast.Document, error) {
	doc := &ast.Document{}
	var stack []*ast.Element

	appendNode := func(n ast.Node) {
		if len(stack) == 0 {
			doc.Nodes = append(doc.Nodes, n)
			return
		}
		top := stack[len(stack)-1]
		top.Children = append(top.Children, n)
	}

	for {
		tok := p.peek()
		switch tok.Type {
		case lexer.EOF:
			if len(stack) > 0 {
				return nil, p.errorf(tok, "标签 <%s> 未闭合", stack[len(stack)-1].Name)
			}
			return doc, nil

		case lexer.Text, lexer.Comment:
			p.advance()
			appendNode(&ast.Text{Value: tok.Raw, Line: tok.Line, Pos: tok.Pos})

		case lexer.Mustache:
			p.advance()
			if isGoAction(tok.Raw) {
				appendNode(&ast.GoAction{Code: tok.Raw, Line: tok.Line, Pos: tok.RawPos})
				break
			}
			appendNode(&ast.Interpolation{Expr: tok.Raw, Line: tok.Line, Pos: tok.RawPos})

		case lexer.OpenTag, lexer.SelfCloseTag:
			p.advance()
			el, err := p.parseElement(tok)
			if err != nil {
				return nil, err
			}
			appendNode(el)
			if tok.Type == lexer.OpenTag && !el.Void() {
				stack = append(stack, el)
			}

		case lexer.CloseTag:
			p.advance()
			idx := -1
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].Name == tok.Name {
					idx = i
					break
				}
			}
			switch {
			case len(stack) == 0:
				return nil, p.errorf(tok, "多余的闭合标签 </%s>", tok.Name)
			case idx < 0:
				return nil, p.errorf(tok, "闭合标签 </%s> 与 <%s> 不匹配", tok.Name, stack[len(stack)-1].Name)
			default:
				// 未显式闭合的中间标签按隐式闭合处理，栈回退到匹配层
				stack = stack[:idx]
			}
		}
	}
}

// parseElement 解析一个标签的标签名与属性，区分普通属性、指令与绑定。
func (p *parser) parseElement(tok lexer.Token) (*ast.Element, error) {
	attrs, err := parseAttrs(tok.Raw)
	if err != nil {
		return nil, p.errorf(tok, "标签 <%s> 的属性解析失败: %v", tok.Name, err)
	}

	el := &ast.Element{
		Name:        tok.Name,
		SelfClosing: tok.Type == lexer.SelfCloseTag,
		Line:        tok.Line,
		Pos:         tok.Pos,
	}
	for _, a := range attrs {
		vpos := tok.RawPos + a.valuePos
		if strings.Contains(a.name, "{{") || strings.Contains(a.name, "}}") {
			return nil, p.errorf(tok, "标签 <%s> 的属性列表中直接出现了 {{ }}，AST 模式只支持把 {{ }} 写在属性值的引号内", tok.Name)
		}
		switch {
		case strings.HasPrefix(a.name, "v-"):
			name := strings.TrimPrefix(a.name, "v-")
			switch {
			case strings.HasPrefix(name, "bind:"):
				el.Bindings = append(el.Bindings, ast.Binding{
					Kind: ast.BindAttr, Attr: strings.TrimPrefix(name, "bind:"), Value: a.value, Line: tok.Line, ValuePos: vpos,
				})
			case strings.HasPrefix(name, "on:"):
				el.Bindings = append(el.Bindings, ast.Binding{
					Kind: ast.BindEvent, Attr: strings.TrimPrefix(name, "on:"), Value: a.value, Line: tok.Line, ValuePos: vpos,
				})
			default:
				el.Directives = append(el.Directives, ast.Directive{Name: name, Value: a.value, Line: tok.Line, ValuePos: vpos})
			}

		case strings.HasPrefix(a.name, ":"):
			el.Bindings = append(el.Bindings, ast.Binding{
				Kind: ast.BindAttr, Attr: strings.TrimPrefix(a.name, ":"), Value: a.value, Line: tok.Line, ValuePos: vpos,
			})

		case strings.HasPrefix(a.name, "@"):
			el.Bindings = append(el.Bindings, ast.Binding{
				Kind: ast.BindEvent, Attr: strings.TrimPrefix(a.name, "@"), Value: a.value, Line: tok.Line, ValuePos: vpos,
			})

		default:
			parts, err := splitValue(a.value, vpos, tok.Line)
			if err != nil {
				return nil, p.errorf(tok, "标签 <%s> 的属性 %s: %v", tok.Name, a.name, err)
			}
			el.Attrs = append(el.Attrs, ast.Attr{Name: a.name, Value: a.value, HasValue: a.hasValue, ValuePos: vpos, Parts: parts})
		}
	}
	return el, nil
}

// splitValue 把普通属性值按 {{ }} 拆成 Text / Interpolation / GoAction 片段。
// 值中不含 {{ 时返回 nil；base 为值首字节在源码中的偏移。
func splitValue(v string, base, line int) ([]ast.Node, error) {
	if !strings.Contains(v, "{{") {
		return nil, nil
	}
	var out []ast.Node
	i := 0
	for i < len(v) {
		j := strings.Index(v[i:], "{{")
		if j < 0 {
			out = append(out, &ast.Text{Value: v[i:], Line: line, Pos: base + i})
			break
		}
		if j > 0 {
			out = append(out, &ast.Text{Value: v[i : i+j], Line: line, Pos: base + i})
		}
		start := i + j + 2
		k := strings.Index(v[start:], "}}")
		if k < 0 {
			return nil, fmt.Errorf("属性值中的 {{ 未闭合，缺少 }}")
		}
		inner := v[start : start+k]
		raw := strings.TrimSpace(inner)
		rawPos := base + start + len(inner) - len(strings.TrimLeft(inner, " \t\n\r"))
		if isGoAction(raw) {
			out = append(out, &ast.GoAction{Code: raw, Line: line, Pos: rawPos})
		} else {
			out = append(out, &ast.Interpolation{Expr: raw, Line: line, Pos: rawPos})
		}
		i = start + k + 2
	}
	return out, nil
}

// rawAttr 是尚未分类的属性。valuePos 是 value 相对属性原文的字节偏移。
type rawAttr struct {
	name     string
	value    string
	hasValue bool
	valuePos int
}

// parseAttrs 解析标签内的属性原文，支持 name、name=value、name="value"、name='value'。
func parseAttrs(raw string) ([]rawAttr, error) {
	var out []rawAttr
	i := 0
	for i < len(raw) {
		i = skipSpace(raw, i)
		if i >= len(raw) {
			break
		}

		start := i
		for i < len(raw) && !isSpace(raw[i]) && raw[i] != '=' {
			i++
		}
		name := raw[start:i]
		if name == "" {
			return nil, fmt.Errorf("位置 %d 处存在非法属性", start)
		}

		value := ""
		hasValue := false
		valuePos := start
		i = skipSpace(raw, i)
		if i < len(raw) && raw[i] == '=' {
			hasValue = true
			i++
			i = skipSpace(raw, i)
			valuePos = i
			if i < len(raw) && (raw[i] == '"' || raw[i] == '\'') {
				quote := raw[i]
				i++
				valuePos = i
				end := strings.IndexByte(raw[i:], quote)
				if end < 0 {
					value = raw[i:]
					i = len(raw)
				} else {
					value = raw[i : i+end]
					i += end + 1
				}
			} else {
				start = i
				for i < len(raw) && !isSpace(raw[i]) {
					i++
				}
				value = raw[start:i]
			}
		}
		out = append(out, rawAttr{name: name, value: value, hasValue: hasValue, valuePos: valuePos})
	}
	return out, nil
}

func skipSpace(s string, i int) int {
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	return i
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func (p *parser) peek() lexer.Token {
	if p.pos >= len(p.tokens) {
		return lexer.Token{Type: lexer.EOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) advance() {
	if p.pos < len(p.tokens) {
		p.pos++
	}
}

// errorf 构造一个带行号的语法错误。
func (p *parser) errorf(tok lexer.Token, format string, args ...any) error {
	return fmt.Errorf("vugo: 第 %d 行: %s", tok.Line, fmt.Sprintf(format, args...))
}
