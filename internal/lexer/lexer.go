// Package lexer 是转译管道的第一层：把类 Vue 模板源码切分为 token 序列。
//
// 本层只负责"字符流 → token 流"，不关心 token 之间的语法关系，
// 也不做任何语义改写（例如不会把 msg 改写成 .msg）。
package lexer

import (
	"fmt"
	"strings"
)

// TokenType 标识 token 的种类。
type TokenType int

const (
	// EOF 表示输入结束。
	EOF TokenType = iota
	// Text 是普通文本片段（含空白）。
	Text
	// Mustache 是 {{ ... }}，Raw 为去掉花括号并 TrimSpace 后的内容。
	Mustache
	// Comment 是 <!-- ... -->，Raw 为完整原文。
	Comment
	// OpenTag 是开始标签 <div ...>，Name 为标签名，Raw 为属性原文。
	OpenTag
	// SelfCloseTag 是自闭合标签 <br />。
	SelfCloseTag
	// CloseTag 是结束标签 </div>。
	CloseTag
)

// String 返回 token 类型的可读名称，便于报错与调试。
func (t TokenType) String() string {
	switch t {
	case EOF:
		return "EOF"
	case Text:
		return "text"
	case Mustache:
		return "mustache"
	case Comment:
		return "comment"
	case OpenTag:
		return "open-tag"
	case SelfCloseTag:
		return "self-closing-tag"
	case CloseTag:
		return "close-tag"
	default:
		return "unknown"
	}
}

// Token 是一个词法单元。字段的含义随 Type 不同而不同：
// Raw 在 Text/Comment 下为原文，在 Mustache 下为内部表达式，在 Tag 下为属性原文。
type Token struct {
	Type TokenType
	Name string // 仅 Tag 类 token 有效，标签名（已小写）
	Raw  string
	Line int // 起始行号，从 1 开始
}

// Lexer 扫描一份模板源码。
type Lexer struct {
	src string
	pos int
}

// New 创建一个扫描 src 的词法分析器。
func New(src string) *Lexer {
	return &Lexer{src: src}
}

// Run 扫描全部输入并返回 token 序列（以 EOF 结尾）。
func (l *Lexer) Run() ([]Token, error) {
	var out []Token
	for {
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		out = append(out, tok)
		if tok.Type == EOF {
			return out, nil
		}
	}
}

// next 扫描下一个 token。
func (l *Lexer) next() (Token, error) {
	if l.pos >= len(l.src) {
		return Token{Type: EOF, Line: l.line(l.pos)}, nil
	}
	rest := l.src[l.pos:]
	switch {
	case strings.HasPrefix(rest, "{{"):
		return l.scanMustache()
	case strings.HasPrefix(rest, "<!--"):
		return l.scanComment()
	case l.src[l.pos] == '<' && l.tagStarts(l.pos):
		return l.scanTag()
	default:
		return l.scanText()
	}
}

// tagStarts 判断 pos 处的 '<' 是否真的是一个标签的开始，
// 用于区分 `a < b` 这类普通文本中的小于号。
func (l *Lexer) tagStarts(pos int) bool {
	if pos+1 >= len(l.src) {
		return false
	}
	c := l.src[pos+1]
	return c == '/' || c == '!' || c == '?' || isAlpha(c)
}

// scanMustache 扫描 {{ ... }}。
func (l *Lexer) scanMustache() (Token, error) {
	start := l.pos
	l.pos += 2
	end := strings.Index(l.src[l.pos:], "}}")
	if end < 0 {
		return Token{}, l.errorf(start, "插值 {{ 未闭合，缺少 }}")
	}
	raw := strings.TrimSpace(l.src[l.pos : l.pos+end])
	l.pos += end + 2
	return Token{Type: Mustache, Raw: raw, Line: l.line(start)}, nil
}

// scanComment 扫描 <!-- ... -->。
func (l *Lexer) scanComment() (Token, error) {
	start := l.pos
	end := strings.Index(l.src[l.pos:], "-->")
	if end < 0 {
		return Token{}, l.errorf(start, "HTML 注释未闭合，缺少 -->")
	}
	l.pos += end + 3
	return Token{Type: Comment, Raw: l.src[start:l.pos], Line: l.line(start)}, nil
}

// scanTag 扫描一个开始标签、自闭合标签或结束标签。
func (l *Lexer) scanTag() (Token, error) {
	start := l.pos
	l.pos++ // 吃掉 '<'

	// <!DOCTYPE ...>、<?xml ...> 等声明不是元素，整体按文本透传。
	if c := l.src[l.pos]; c == '!' || c == '?' {
		l.pos--
		return l.scanDeclaration()
	}

	closing := false
	if l.src[l.pos] == '/' {
		closing = true
		l.pos++
	}

	nameStart := l.pos
	for l.pos < len(l.src) && isTagName(l.src[l.pos]) {
		l.pos++
	}
	name := strings.ToLower(l.src[nameStart:l.pos])
	if name == "" {
		return Token{}, l.errorf(start, "标签缺少标签名")
	}

	if closing {
		if i := strings.IndexByte(l.src[l.pos:], '>'); i >= 0 {
			l.pos += i + 1
		} else {
			return Token{}, l.errorf(start, "闭合标签 </%s> 缺少 >", name)
		}
		return Token{Type: CloseTag, Name: name, Line: l.line(start)}, nil
	}

	bodyStart := l.pos
	if err := l.skipToTagEnd(); err != nil {
		return Token{}, err
	}
	body := strings.TrimSpace(l.src[bodyStart : l.pos-1]) // 去掉结尾的 '>'

	typ := OpenTag
	selfClosing := strings.HasSuffix(body, "/")
	if selfClosing {
		typ = SelfCloseTag
		body = strings.TrimSpace(body[:len(body)-1])
	}
	return Token{Type: typ, Name: name, Raw: body, Line: l.line(start)}, nil
}

// scanDeclaration 扫描 <!DOCTYPE ...> 之类的声明，整体作为文本透传。
func (l *Lexer) scanDeclaration() (Token, error) {
	start := l.pos
	i := strings.IndexByte(l.src[l.pos:], '>')
	if i < 0 {
		l.pos = len(l.src)
	} else {
		l.pos += i + 1
	}
	return Token{Type: Text, Raw: l.src[start:l.pos], Line: l.line(start)}, nil
}

// skipToTagEnd 前进到标签结尾的 '>' 之后，属性值中的引号会被正确跳过。
func (l *Lexer) skipToTagEnd() error {
	start := l.pos
	var quote byte
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			l.pos++
			return nil
		}
		l.pos++
	}
	return fmt.Errorf("vugo: 第 %d 行: 标签未闭合，缺少 >", l.line(start))
}

// scanText 扫描普通文本，直到遇到 {{ 或下一个标签起点。
func (l *Lexer) scanText() (Token, error) {
	start := l.pos
	for l.pos < len(l.src) {
		if strings.HasPrefix(l.src[l.pos:], "{{") {
			break
		}
		if l.src[l.pos] == '<' && l.tagStarts(l.pos) {
			break
		}
		l.pos++
	}
	if l.pos == start {
		// 兜底：至少前进一个字节，避免死循环
		l.pos++
	}
	return Token{Type: Text, Raw: l.src[start:l.pos], Line: l.line(start)}, nil
}

// line 返回字节偏移 pos 所在的行号（从 1 开始）。
func (l *Lexer) line(pos int) int {
	if pos > len(l.src) {
		pos = len(l.src)
	}
	return strings.Count(l.src[:pos], "\n") + 1
}

// errorf 构造一个带行号的词法错误。
func (l *Lexer) errorf(pos int, format string, args ...any) error {
	return fmt.Errorf("vugo: 第 %d 行: %s", l.line(pos), fmt.Sprintf(format, args...))
}

func isAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isTagName(c byte) bool {
	return isAlpha(c) || isDigit(c) || c == '-' || c == '_' || c == ':' || c == '.'
}
