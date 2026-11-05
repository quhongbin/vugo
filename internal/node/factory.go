// Package node 是 vugo 与 text/template/parse 之间的适配层：
// 负责创建可以被 html/template.AddParseTree 直接接收的 parse 节点。
//
// 为什么需要这一层：parse 包的节点构造函数（newText、newAction ...）都不导出，
// 节点内部还持有一个不导出的 tr *parse.Tree 字段，html/template 在报告转义错误时
// 会调用 (*parse.Tree)(nil).ErrorContext(node)，而 ErrorContext 依赖 node.tr 中的源码文本。
// 如果直接用结构体字面量 &parse.TextNode{...} 构造节点，tr 为 nil，一旦出现转义错误
// 就会在 Error() 中触发空指针 panic。
//
// 解决办法（只使用 parse 包公开 API）：
//  1. 用一对绝不会出现在源码里的定界符（\x02 / \x03）调用 (*parse.Tree).Parse，
//     解析「原始 .vugo 源码 + 一段原型模板」。源码部分整体成为一个文本节点，
//     因此 Tree 内部的 text 字段恰好以 .vugo 源码开头；
//  2. 原型模板部分会产出每一种需要的节点（If/Range/Action/Field ...），
//     它们的 tr 都指向这棵 Tree；
//  3. 之后所有节点都通过「复制原型 + 改写导出字段」得到，tr 自动继承，
//     而 Pos 写入 .vugo 源码中的真实偏移，于是报错能准确定位到 .vugo 的行列。
package node

import (
	"fmt"
	"strconv"
	"strings"
	"text/template/parse"
)

// 原型解析使用的定界符。它们是 ASCII 控制字符 STX/ETX，正常模板中不会出现。
const (
	leftDelim  = "\x02"
	rightDelim = "\x03"
)

// protoSrc 是原型模板，覆盖转译需要的全部节点类型。
// 文本原型 "x" 被动作节点隔开，避免和源码文本合并成同一个节点。
var protoSrc = leftDelim + "if .a" + rightDelim + leftDelim + "end" + rightDelim +
	leftDelim + "range $i, $v := .a" + rightDelim + leftDelim + "break" + rightDelim + leftDelim + "continue" + rightDelim + leftDelim + "end" + rightDelim +
	leftDelim + "with .a" + rightDelim + leftDelim + "end" + rightDelim +
	leftDelim + `template "x" .` + rightDelim +
	leftDelim + ".a" + rightDelim + "x" +
	leftDelim + "." + rightDelim +
	leftDelim + "$" + rightDelim +
	leftDelim + `"s"` + rightDelim +
	leftDelim + "1" + rightDelim +
	leftDelim + "true" + rightDelim +
	leftDelim + "print nil" + rightDelim

// Factory 创建绑定在同一棵 parse.Tree 上的 parse 节点。
//
// 一个 Factory 对应一份 .vugo 源码；它创建的所有节点的 Pos 都解释为该源码的字节偏移。
// Factory 不是并发安全的，应在单个 goroutine 内使用。
type Factory struct {
	name string
	src  string
	tree *parse.Tree

	list     *parse.ListNode
	text     *parse.TextNode
	action   *parse.ActionNode
	pipe     *parse.PipeNode
	cmd      *parse.CommandNode
	field    *parse.FieldNode
	dot      *parse.DotNode
	variable *parse.VariableNode
	str      *parse.StringNode
	num      *parse.NumberNode
	boolean  *parse.BoolNode
	null     *parse.NilNode
	ifNode   *parse.IfNode
	rng      *parse.RangeNode
	with     *parse.WithNode
	tmpl     *parse.TemplateNode
	brk      *parse.BreakNode
	cont     *parse.ContinueNode
}

// New 为一份 .vugo 源码创建节点工厂。
//
// 参数:
//   - name: 模板名，会写入 Tree.Name / Tree.ParseName，出现在 html/template 的报错前缀中。
//   - src:  完整的 .vugo 源码；节点的 Pos 都以它为基准。
//
// 返回值:
//   - *Factory: 可立即用于创建节点的工厂。
//   - error:    src 中包含保留控制字符 \x02/\x03，或原型解析失败时返回，信息以 `vugo:` 开头。
func New(name, src string) (*Factory, error) {
	if strings.ContainsAny(src, leftDelim+rightDelim) {
		return nil, fmt.Errorf("vugo: 源码中包含保留控制字符 \\x02 或 \\x03，无法构建 parse 树")
	}
	tree := parse.New(name)
	tree.Mode = parse.SkipFuncCheck
	if _, err := tree.Parse(src+protoSrc, leftDelim, rightDelim, map[string]*parse.Tree{}); err != nil {
		return nil, fmt.Errorf("vugo: 构建节点原型失败: %w", err)
	}
	f := &Factory{name: name, src: src, tree: tree}
	if err := f.collect(tree.Root.Nodes); err != nil {
		return nil, err
	}
	// 用非动作定界符把 tree 内部的 leftDelim/rightDelim 恢复为默认的 "{{" 与 "}}"，
	// 但要避免 src 自身的 {{ ... }} 被标准库直接当 Go 模板解析失败：
	// 这里先用 "\x02\x03" 作为左定界符保证 src 完全作为文本解析，
	// 序列化输出 .tmpl 时由 Result.String 遍历 AST 或统一替换定界符。
	tree.Root = f.List(0)
	return f, nil
}

// collect 从原型树中按类型挑出原型节点。
func (f *Factory) collect(nodes []parse.Node) error {
	for _, n := range nodes {
		switch v := n.(type) {
		case *parse.TextNode:
			if string(v.Text) == "x" {
				f.text = v
			}
		case *parse.IfNode:
			f.ifNode = v
			f.list = v.List
		case *parse.RangeNode:
			f.rng = v
			for _, c := range v.List.Nodes {
				switch b := c.(type) {
				case *parse.BreakNode:
					f.brk = b
				case *parse.ContinueNode:
					f.cont = b
				}
			}
		case *parse.WithNode:
			f.with = v
		case *parse.TemplateNode:
			f.tmpl = v
		case *parse.ActionNode:
			cmd := v.Pipe.Cmds[0]
			switch a := cmd.Args[0].(type) {
			case *parse.FieldNode:
				f.action, f.pipe, f.cmd, f.field = v, v.Pipe, cmd, a
			case *parse.DotNode:
				f.dot = a
			case *parse.VariableNode:
				f.variable = a
			case *parse.StringNode:
				f.str = a
			case *parse.NumberNode:
				f.num = a
			case *parse.BoolNode:
				f.boolean = a
			case *parse.IdentifierNode:
				if nl, ok := cmd.Args[1].(*parse.NilNode); ok {
					f.null = nl
				}
			}
		}
	}
	if f.list == nil || f.text == nil || f.action == nil || f.dot == nil || f.variable == nil ||
		f.str == nil || f.num == nil || f.boolean == nil || f.null == nil || f.rng == nil ||
		f.with == nil || f.tmpl == nil || f.brk == nil || f.cont == nil {
		return fmt.Errorf("vugo: 节点原型不完整，当前 Go 版本的 text/template/parse 结构可能已变化")
	}
	return nil
}

// Name 返回工厂对应的模板名。
//
// 返回值:
//   - string: 调用 New 时传入的 name。
func (f *Factory) Name() string { return f.name }

// Src 返回工厂对应的 .vugo 源码。
//
// 返回值:
//   - string: 调用 New 时传入的 src。
func (f *Factory) Src() string { return f.src }

// Line 计算源码偏移 pos 所在的行号（从 1 开始），供 ActionNode.Line 等字段使用。
//
// 参数:
//   - pos: 源码中的字节偏移，超出范围时按源码末尾计算。
//
// 返回值:
//   - int: 行号，从 1 开始。
func (f *Factory) Line(pos int) int {
	if pos > len(f.src) {
		pos = len(f.src)
	}
	if pos < 0 {
		pos = 0
	}
	return strings.Count(f.src[:pos], "\n") + 1
}

// MainTree 返回承载主模板的 parse.Tree，并把 root 挂为它的根节点。
//
// 该 Tree 就是原型解析使用的那棵树，其内部源码文本以 .vugo 源码开头，
// 因此 Tree.Copy、ErrorContext 等方法都能正常工作。
//
// 参数:
//   - root: 主模板的根节点列表，通常由 List 创建。
//
// 返回值:
//   - *parse.Tree: Name/ParseName 均为工厂名的树，可直接交给 html/template.AddParseTree。
//
// 副作用: 会覆盖内部树的 Root；多次调用以最后一次为准。
func (f *Factory) MainTree(root *parse.ListNode) *parse.Tree {
	f.tree.Root = root
	return f.tree
}

// DefineTree 为 {{ define "name" }} / {{ block "name" }} 创建一棵独立的 parse.Tree。
//
// 参数:
//   - name: 被定义的模板名。
//   - root: 定义体的根节点列表。
//
// 返回值:
//   - *parse.Tree: ParseName 为工厂名的新树；其中节点的 tr 仍指向主树，报错可定位到源码。
func (f *Factory) DefineTree(name string, root *parse.ListNode) *parse.Tree {
	t := f.tree.Copy()
	t.Name = name
	t.ParseName = f.name
	t.Root = root
	return t
}

// List 创建一个空的 ListNode。
//
// 参数:
//   - pos: 源码字节偏移。
//
// 返回值:
//   - *parse.ListNode: Nodes 为空的列表节点，调用方可直接 append 到 Nodes。
func (f *Factory) List(pos int) *parse.ListNode {
	n := f.list.CopyList()
	n.Pos = parse.Pos(pos)
	n.Nodes = nil
	return n
}

// AppendText 向 list 末尾追加一段文本；若末尾已是文本节点则直接合并，减少节点数量。
//
// 参数:
//   - list: 目标列表，会被原地修改。
//   - pos:  文本在源码中的字节偏移（仅在新建节点时使用）。
//   - s:    文本内容，为空时什么也不做。
func (f *Factory) AppendText(list *parse.ListNode, pos int, s string) {
	if s == "" {
		return
	}
	if k := len(list.Nodes); k > 0 {
		if t, ok := list.Nodes[k-1].(*parse.TextNode); ok {
			t.Text = append(t.Text, s...)
			return
		}
	}
	list.Nodes = append(list.Nodes, f.Text(pos, s))
}

// Text 创建一个文本节点。
//
// 参数:
//   - pos: 源码字节偏移。
//   - s:   原样输出的文本。
//
// 返回值:
//   - *parse.TextNode: 新文本节点。
func (f *Factory) Text(pos int, s string) *parse.TextNode {
	n := f.text.Copy().(*parse.TextNode)
	n.Pos = parse.Pos(pos)
	n.Text = []byte(s)
	return n
}

// Action 创建一个 {{ pipe }} 动作节点。
//
// 参数:
//   - pos:  源码字节偏移。
//   - pipe: 动作的管道，通常由 Pipe 创建。
//
// 返回值:
//   - *parse.ActionNode: 新动作节点，Line 已按 pos 计算。
func (f *Factory) Action(pos int, pipe *parse.PipeNode) *parse.ActionNode {
	n := f.action.Copy().(*parse.ActionNode)
	n.Pos = parse.Pos(pos)
	n.Line = f.Line(pos)
	n.Pipe = pipe
	return n
}

// Pipe 创建一个管道节点。
//
// 参数:
//   - pos:  源码字节偏移。
//   - decl: 管道声明的变量（如 range 的 $i, $v），没有时传 nil。
//   - cmds: 管道中的命令，按 `|` 顺序排列。
//
// 返回值:
//   - *parse.PipeNode: 新管道节点，IsAssign 为 false。
func (f *Factory) Pipe(pos int, decl []*parse.VariableNode, cmds ...*parse.CommandNode) *parse.PipeNode {
	n := f.pipe.CopyPipe()
	n.Pos = parse.Pos(pos)
	n.Line = f.Line(pos)
	n.IsAssign = false
	n.Decl = decl
	n.Cmds = cmds
	return n
}

// Command 创建一个命令节点，如 `eq .a .b` 对应 Args = [eq, .a, .b]。
//
// 参数:
//   - pos:  源码字节偏移。
//   - args: 命令的操作数，首个元素为函数名（IdentifierNode）或单个值。
//
// 返回值:
//   - *parse.CommandNode: 新命令节点。
func (f *Factory) Command(pos int, args ...parse.Node) *parse.CommandNode {
	n := f.cmd.Copy().(*parse.CommandNode)
	n.Pos = parse.Pos(pos)
	n.Args = args
	return n
}

// Identifier 创建一个函数名节点，如 `formatTime`、`eq`。
//
// 参数:
//   - pos:  源码字节偏移。
//   - name: 函数名，必须是合法的 Go 标识符。
//
// 返回值:
//   - *parse.IdentifierNode: 绑定到工厂树的标识符节点。
func (f *Factory) Identifier(pos int, name string) *parse.IdentifierNode {
	return parse.NewIdentifier(name).SetTree(f.tree).SetPos(parse.Pos(pos))
}

// Field 创建一个字段访问节点，如 path = ["user", "name"] 对应 `.user.name`。
//
// 参数:
//   - pos:  源码字节偏移。
//   - path: 字段路径，至少一个元素。
//
// 返回值:
//   - *parse.FieldNode: 新字段节点。
func (f *Factory) Field(pos int, path ...string) *parse.FieldNode {
	n := f.field.Copy().(*parse.FieldNode)
	n.Pos = parse.Pos(pos)
	n.Ident = append([]string(nil), path...)
	return n
}

// Dot 创建一个 `.` 节点。
//
// 参数:
//   - pos: 源码字节偏移。
//
// 返回值:
//   - *parse.DotNode: 新点节点。
func (f *Factory) Dot(pos int) *parse.DotNode {
	n := f.dot.Copy().(*parse.DotNode)
	n.Pos = parse.Pos(pos)
	return n
}

// Variable 创建一个变量节点，如 ident = ["$item", "name"] 对应 `$item.name`。
//
// 参数:
//   - pos:   源码字节偏移。
//   - ident: 变量名（含 `$`）及后续字段路径。
//
// 返回值:
//   - *parse.VariableNode: 新变量节点。
func (f *Factory) Variable(pos int, ident ...string) *parse.VariableNode {
	n := f.variable.Copy().(*parse.VariableNode)
	n.Pos = parse.Pos(pos)
	n.Ident = append([]string(nil), ident...)
	return n
}

// String 创建一个字符串字面量节点。
//
// 参数:
//   - pos:    源码字节偏移。
//   - quoted: 带双引号的 Go 字符串字面量，如 `"hello"`。
//
// 返回值:
//   - *parse.StringNode: 新字符串节点。
//   - error:  quoted 不是合法的 Go 字符串字面量时返回，信息以 `vugo:` 开头。
func (f *Factory) String(pos int, quoted string) (*parse.StringNode, error) {
	text, err := strconv.Unquote(quoted)
	if err != nil {
		return nil, fmt.Errorf("vugo: 非法字符串字面量 %s: %v", quoted, err)
	}
	n := f.str.Copy().(*parse.StringNode)
	n.Pos = parse.Pos(pos)
	n.Quoted = quoted
	n.Text = text
	return n, nil
}

// Number 创建一个数字字面量节点，仅支持十进制整数与小数（可带负号）。
//
// 参数:
//   - pos:  源码字节偏移。
//   - text: 数字原文，如 `10`、`-3`、`1.5`。
//
// 返回值:
//   - *parse.NumberNode: 按 text/template 规则填好 IsInt/IsUint/IsFloat 等字段的节点。
//   - error: text 无法解析为数字时返回，信息以 `vugo:` 开头。
func (f *Factory) Number(pos int, text string) (*parse.NumberNode, error) {
	n := f.num.Copy().(*parse.NumberNode)
	n.Pos = parse.Pos(pos)
	n.Text = text
	n.IsInt, n.IsUint, n.IsFloat, n.IsComplex = false, false, false, false
	n.Int64, n.Uint64, n.Float64, n.Complex128 = 0, 0, 0, 0
	// 与 parse.newNumber 的规则保持一致
	if u, err := strconv.ParseUint(text, 0, 64); err == nil {
		n.IsUint, n.Uint64 = true, u
	}
	if i, err := strconv.ParseInt(text, 0, 64); err == nil {
		n.IsInt, n.Int64 = true, i
		if i == 0 {
			n.IsUint, n.Uint64 = true, 0
		}
	}
	switch {
	case n.IsInt:
		n.IsFloat, n.Float64 = true, float64(n.Int64)
	case n.IsUint:
		n.IsFloat, n.Float64 = true, float64(n.Uint64)
	default:
		fv, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, fmt.Errorf("vugo: 非法数字字面量 %q", text)
		}
		n.IsFloat, n.Float64 = true, fv
		if float64(int64(fv)) == fv {
			n.IsInt, n.Int64 = true, int64(fv)
		}
		if fv >= 0 && float64(uint64(fv)) == fv {
			n.IsUint, n.Uint64 = true, uint64(fv)
		}
	}
	return n, nil
}

// Bool 创建一个布尔字面量节点。
//
// 参数:
//   - pos: 源码字节偏移。
//   - v:   布尔值。
//
// 返回值:
//   - *parse.BoolNode: 新布尔节点。
func (f *Factory) Bool(pos int, v bool) *parse.BoolNode {
	n := f.boolean.Copy().(*parse.BoolNode)
	n.Pos = parse.Pos(pos)
	n.True = v
	return n
}

// Nil 创建一个 nil 字面量节点。nil 只能作为函数参数，不能单独构成命令。
//
// 参数:
//   - pos: 源码字节偏移。
//
// 返回值:
//   - *parse.NilNode: 新 nil 节点。
func (f *Factory) Nil(pos int) *parse.NilNode {
	n := f.null.Copy().(*parse.NilNode)
	n.Pos = parse.Pos(pos)
	return n
}

// If 创建一个 {{ if pipe }}list{{ else }}elseList{{ end }} 节点。
//
// 参数:
//   - pos:      源码字节偏移。
//   - pipe:     条件管道。
//   - list:     条件为真时执行的列表。
//   - elseList: 条件为假时执行的列表，没有 else 时传 nil。
//
// 返回值:
//   - *parse.IfNode: 新 if 节点。
func (f *Factory) If(pos int, pipe *parse.PipeNode, list, elseList *parse.ListNode) *parse.IfNode {
	n := f.ifNode.Copy().(*parse.IfNode)
	f.fillBranch(&n.BranchNode, pos, pipe, list, elseList)
	return n
}

// Range 创建一个 {{ range pipe }}list{{ else }}elseList{{ end }} 节点。
//
// 参数:
//   - pos:      源码字节偏移。
//   - pipe:     被遍历的管道，range 声明的变量放在 pipe.Decl 中。
//   - list:     循环体。
//   - elseList: 集合为空时执行的列表，没有时传 nil。
//
// 返回值:
//   - *parse.RangeNode: 新 range 节点。
func (f *Factory) Range(pos int, pipe *parse.PipeNode, list, elseList *parse.ListNode) *parse.RangeNode {
	n := f.rng.Copy().(*parse.RangeNode)
	f.fillBranch(&n.BranchNode, pos, pipe, list, elseList)
	return n
}

// With 创建一个 {{ with pipe }}list{{ else }}elseList{{ end }} 节点。
//
// 参数:
//   - pos:      源码字节偏移。
//   - pipe:     求值后作为新 `.` 的管道。
//   - list:     值非空时执行的列表。
//   - elseList: 值为空时执行的列表，没有时传 nil。
//
// 返回值:
//   - *parse.WithNode: 新 with 节点。
func (f *Factory) With(pos int, pipe *parse.PipeNode, list, elseList *parse.ListNode) *parse.WithNode {
	n := f.with.Copy().(*parse.WithNode)
	f.fillBranch(&n.BranchNode, pos, pipe, list, elseList)
	return n
}

func (f *Factory) fillBranch(b *parse.BranchNode, pos int, pipe *parse.PipeNode, list, elseList *parse.ListNode) {
	b.Pos = parse.Pos(pos)
	b.Line = f.Line(pos)
	b.Pipe = pipe
	b.List = list
	b.ElseList = elseList
}

// Template 创建一个 {{ template "name" pipe }} 节点。
//
// 参数:
//   - pos:  源码字节偏移。
//   - name: 被调用的模板名。
//   - pipe: 传入的数据管道，可为 nil（等价于 `{{ template "name" }}`）。
//
// 返回值:
//   - *parse.TemplateNode: 新 template 节点。
func (f *Factory) Template(pos int, name string, pipe *parse.PipeNode) *parse.TemplateNode {
	n := f.tmpl.Copy().(*parse.TemplateNode)
	n.Pos = parse.Pos(pos)
	n.Line = f.Line(pos)
	n.Name = name
	n.Pipe = pipe
	return n
}

// Break 创建一个 {{ break }} 节点，调用方需保证它位于 range 之内。
//
// 参数:
//   - pos: 源码字节偏移。
//
// 返回值:
//   - *parse.BreakNode: 新 break 节点。
func (f *Factory) Break(pos int) *parse.BreakNode {
	n := f.brk.Copy().(*parse.BreakNode)
	n.Pos = parse.Pos(pos)
	n.Line = f.Line(pos)
	return n
}

// Continue 创建一个 {{ continue }} 节点，调用方需保证它位于 range 之内。
//
// 参数:
//   - pos: 源码字节偏移。
//
// 返回值:
//   - *parse.ContinueNode: 新 continue 节点。
func (f *Factory) Continue(pos int) *parse.ContinueNode {
	n := f.cont.Copy().(*parse.ContinueNode)
	n.Pos = parse.Pos(pos)
	n.Line = f.Line(pos)
	return n
}
