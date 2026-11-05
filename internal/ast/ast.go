// Package ast 定义 vugo 模板的抽象语法树（AST）节点。
//
// 本包只描述"模板长什么样"，不包含任何扫描、转译逻辑：
// 由 parser 负责生产，由 transpiler 负责消费。
//
// 所有 Pos / ValuePos 字段都是相对于整份 .vugo 源码的字节偏移，
// transpiler 会把它们写入 text/template/parse 节点，供 html/template 报错定位。
package ast

// Node 是所有 AST 节点的公共接口。
type Node interface {
	node()
}

// Document 是一份模板的根节点。
type Document struct {
	Nodes []Node
}

func (*Document) node() {}

// Text 是原样输出的文本片段（含空白与 HTML 注释）。
type Text struct {
	Value string
	Line  int // 源码中的起始行号，用于报错定位
	Pos   int // 源码中的起始字节偏移
}

func (*Text) node() {}

// Interpolation 是类 Vue 插值 {{ expr }}，转译时需要对表达式做改写。
type Interpolation struct {
	Expr string // 原始表达式，如 "user.name"
	Line int
	Pos  int // 表达式首字节在源码中的偏移
}

func (*Interpolation) node() {}

// GoAction 是原生 html/template action（{{ if }}、{{ define }}、{{ template }} 等），
// 转译时原样透传，以保证已有模板生态不受破坏。
type GoAction struct {
	Code string // 花括号内的原始代码，如 `if .show`
	Line int
	Pos  int // 代码首字节在源码中的偏移
}

func (*GoAction) node() {}

// Attr 是一个普通 HTML 属性。
type Attr struct {
	Name     string
	Value    string // 无值属性为空字符串
	HasValue bool   // 是否显式写了 =value（区分 `disabled` 与 `title=""`）
	ValuePos int    // Value 首字节在源码中的偏移，无值时为属性名偏移
	// Parts 是 Value 中含 {{ }} 时拆分出的片段（Text / Interpolation / GoAction），
	// 不含 {{ }} 时为 nil，此时直接使用 Value。
	Parts []Node
}

// Directive 是一个 v-* 指令，如 v-for="item in list"。
type Directive struct {
	Name     string // 指令名，不含 "v-" 前缀，如 "for"
	Value    string // 指令原始表达式
	Line     int
	ValuePos int // Value 首字节在源码中的偏移
}

// BindingKind 区分绑定的是属性还是事件。
type BindingKind int

const (
	// BindAttr 表示属性绑定：:href="url" 或 v-bind:href="url"
	BindAttr BindingKind = iota
	// BindEvent 表示事件绑定：@click="submit" 或 v-on:click="submit"
	BindEvent
)

// Binding 是一个属性或事件绑定。
type Binding struct {
	Kind     BindingKind
	Attr     string // 绑定的属性/事件名，如 "href"
	Value    string // 绑定的原始表达式
	Line     int
	ValuePos int // Value 首字节在源码中的偏移
}

// Element 是一个 HTML 元素（含自闭合与 void 元素）。
type Element struct {
	Name        string // 标签名，统一小写
	Attrs       []Attr // 普通属性，已剔除指令与绑定
	Directives  []Directive
	Bindings    []Binding
	Children    []Node
	SelfClosing bool // 是否为 <br /> 形式的自闭合标签
	Line        int
	Pos         int // `<` 在源码中的偏移
}

func (*Element) node() {}

// Void 判断标签是否为 HTML void 元素（不需要闭合标签）。
func (e *Element) Void() bool {
	switch e.Name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}
