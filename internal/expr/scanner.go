package expr

import (
	"fmt"
	"strings"
)

// tokenKind 标识表达式内部词法单元的种类。
type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokNumber
	tokString
	tokPunct    // ( ) , .
	tokOperator // 算术、比较、逻辑运算符
)

// token 是表达式内部的一个词法单元。
type token struct {
	kind tokenKind
	val  string
	pos  int
}

// operators 按最长匹配优先排列，key 为源码写法，value 为转译后的 Go 运算符。
var operators = []struct {
	src  string
	goal string
}{
	{"===", "=="},
	{"!==", "!="},
	{"==", "=="},
	{"!=", "!="},
	{">=", ">="},
	{"<=", "<="},
	{"&&", "&&"},
	{"||", "||"},
	{">", ">"},
	{"<", "<"},
	{"+", "+"},
	{"-", "-"},
	{"*", "*"},
	{"/", "/"},
	{"%", "%"},
	{"!", "!"},
}

// scanner 扫描一个表达式字符串。
type scanner struct {
	src string
	pos int
}

// scan 返回表达式的全部 token（以 tokEOF 结尾）。
func (s *scanner) scan() ([]token, error) {
	var out []token
	for {
		s.skipSpace()
		if s.pos >= len(s.src) {
			return append(out, token{kind: tokEOF, pos: s.pos}), nil
		}
		c := s.src[s.pos]
		switch {
		case isIdentStart(c):
			out = append(out, s.scanIdent())
		case isDigit(c):
			out = append(out, s.scanNumber())
		case c == '"' || c == '\'' || c == '`':
			tok, err := s.scanString()
			if err != nil {
				return nil, err
			}
			out = append(out, tok)
		case c == '(' || c == ')' || c == ',' || c == '.':
			out = append(out, token{kind: tokPunct, val: string(c), pos: s.pos})
			s.pos++
		default:
			tok, ok := s.scanOperator()
			if !ok {
				return nil, fmt.Errorf("vugo: 表达式第 %d 列: 无法识别的字符 %q", s.pos+1, string(c))
			}
			out = append(out, tok)
		}
	}
}

func (s *scanner) skipSpace() {
	for s.pos < len(s.src) && (s.src[s.pos] == ' ' || s.src[s.pos] == '\t' || s.src[s.pos] == '\n' || s.src[s.pos] == '\r') {
		s.pos++
	}
}

func (s *scanner) scanIdent() token {
	start := s.pos
	for s.pos < len(s.src) && isIdentChar(s.src[s.pos]) {
		s.pos++
	}
	return token{kind: tokIdent, val: s.src[start:s.pos], pos: start}
}

func (s *scanner) scanNumber() token {
	start := s.pos
	dot := false
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if isDigit(c) {
			s.pos++
			continue
		}
		if c == '.' && !dot {
			dot = true
			s.pos++
			continue
		}
		break
	}
	return token{kind: tokNumber, val: s.src[start:s.pos], pos: start}
}

// scanString 扫描字符串字面量。单引号会按 Go 模板语法改写为双引号。
func (s *scanner) scanString() (token, error) {
	quote := s.src[s.pos]
	start := s.pos
	s.pos++
	var b strings.Builder
	b.WriteByte('"')
	closed := false
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if c == '\\' && s.pos+1 < len(s.src) {
			b.WriteByte(c)
			b.WriteByte(s.src[s.pos+1])
			s.pos += 2
			continue
		}
		if c == quote {
			s.pos++
			closed = true
			break
		}
		if c == '"' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
		s.pos++
	}
	if !closed {
		return token{}, fmt.Errorf("vugo: 表达式第 %d 列: 字符串未闭合", start+1)
	}
	b.WriteByte('"')
	return token{kind: tokString, val: b.String(), pos: start}, nil
}

func (s *scanner) scanOperator() (token, bool) {
	for _, op := range operators {
		if strings.HasPrefix(s.src[s.pos:], op.src) {
			s.pos += len(op.src)
			return token{kind: tokOperator, val: op.goal, pos: s.pos}, true
		}
	}
	return token{}, false
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || isAlpha(c)
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}

func isAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
