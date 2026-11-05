// Package transpiler 是转译管道的第三层：把 vugo 模板 AST 直接构建为
// text/template/parse 的语法树（*parse.Tree），可交给 html/template.AddParseTree。
//
// 本层是"类 Vue 语法 → Go 模板语法树"的唯一改写点：
//   - 插值与指令表达式交给 expr 包构建为 PipeNode
//   - v-for 指令展开为 RangeNode（见 for.go）
//   - 原生 html/template action（if/range/with/define/block/template ...）
//     按出现顺序组装为对应的 parse 节点，define/block 产出独立的 Tree
//   - 无法映射的指令/绑定直接报错并给出替代方案（见 directive.go）
//
// 所有 parse 节点都由 internal/node.Factory 创建，保证节点能被 html/template 安全使用。
package transpiler

import (
	"fmt"
	"strconv"
	"strings"
	"text/template/parse"

	"github.com/quhongbin/vugo/internal/ast"
	"github.com/quhongbin/vugo/internal/expr"
	"github.com/quhongbin/vugo/internal/node"
)

// block 是一个尚未遇到 {{ end }} 的控制结构，或一个 v-for 元素体。
type block struct {
	kind   string            // if / with / range / define / block / v-for
	branch *parse.BranchNode // if/with/range 当前分支，else if 时会更新为内层分支
	list   *parse.ListNode   // 当前接收子节点的列表
	name   string            // define/block 的模板名
	pos    int               // 起始偏移，用于报错

	hasElse     bool
	inRangeBody bool // range 尚未进入 else 分支

	// define/block 体使用独立作用域，结束时恢复
	savedScope *expr.Scope
	savedRange int
}

// Transpiler 遍历 vugo AST 并构建 parse 树，同时维护变量作用域与控制结构栈。
//
// Transpiler 不是并发安全的；同一个实例可以顺序复用于多次 Transpile。
type Transpiler struct {
	f          *node.Factory
	scope      *expr.Scope
	root       *parse.ListNode
	blocks     []*block
	defines    []*parse.Tree
	defined    map[string]bool
	rangeDepth int
	elemDepth  int
}

// New 创建一个转译器。
//
// 返回值:
//   - *Transpiler: 零状态的转译器，可直接调用 Transpile。
func New() *Transpiler {
	return &Transpiler{}
}

// Transpile 把 vugo 模板 AST 构建为 html/template 可接收的 parse 树。
//
// 参数:
//   - doc:  parser 产出的模板 AST。
//   - name: 模板名，作为主树的 Tree.Name，也出现在 html/template 的报错位置前缀中。
//   - src:  生成 doc 的 .vugo 源码，节点 Pos 以它为基准，用于报错定位。
//
// 返回值:
//   - *Result: 主树与 define/block 产生的子树。
//   - error: 遇到不支持的语法、表达式错误、原生 action 不配对或跨越元素边界时返回，
//     信息以 `vugo: 第 N 行:` 开头。
func (t *Transpiler) Transpile(doc *ast.Document, name, src string) (*Result, error) {
	f, err := node.New(name, src)
	if err != nil {
		return nil, err
	}
	*t = Transpiler{f: f, scope: expr.NewScope(), root: f.List(0), defined: map[string]bool{}}

	if err := t.writeNodes(doc.Nodes); err != nil {
		return nil, err
	}
	if len(t.blocks) > 0 {
		b := t.blocks[len(t.blocks)-1]
		return nil, t.errorf(b.pos, "{{ %s }} 缺少对应的 {{ end }}", b.kind)
	}
	return &Result{Name: name, Main: f.MainTree(t.root), Defines: t.defines}, nil
}

func (t *Transpiler) errorf(pos int, format string, args ...any) error {
	return fmt.Errorf("vugo: 第 %d 行: %s", t.f.Line(pos), fmt.Sprintf(format, args...))
}

// cur 返回当前接收节点的列表。
func (t *Transpiler) cur() *parse.ListNode {
	if n := len(t.blocks); n > 0 {
		return t.blocks[n-1].list
	}
	return t.root
}

func (t *Transpiler) appendNode(n parse.Node) {
	l := t.cur()
	l.Nodes = append(l.Nodes, n)
}

func (t *Transpiler) writeNodes(nodes []ast.Node) error {
	for _, n := range nodes {
		if err := t.writeNode(n); err != nil {
			return err
		}
	}
	return nil
}

// writeNode 构建单个 AST 节点。
func (t *Transpiler) writeNode(n ast.Node) error {
	switch v := n.(type) {
	case *ast.Text:
		t.f.AppendText(t.cur(), v.Pos, v.Value)
		return nil

	case *ast.GoAction:
		return t.native(v.Code, v.Pos)

	case *ast.Interpolation:
		pipe, err := expr.Build(v.Expr, v.Pos, t.scope, t.f)
		if err != nil {
			return t.errorf(v.Pos, "%v", err)
		}
		t.appendNode(t.f.Action(v.Pos, pipe))
		return nil

	case *ast.Element:
		return t.writeElement(v)

	default:
		return fmt.Errorf("vugo: 无法处理的 AST 节点 %T", n)
	}
}

// writeElement 构建一个元素，必要时用 RangeNode 包裹。
func (t *Transpiler) writeElement(el *ast.Element) error {
	loop, err := resolveElement(el)
	if err != nil {
		return err
	}

	if loop != nil {
		var valuePos int
		for _, d := range el.Directives {
			if d.Name == "for" {
				valuePos = d.ValuePos
			}
		}
		// 可迭代对象在循环外的作用域中求值
		pipe, err := loop.Pipe(t.f, valuePos, t.scope)
		if err != nil {
			return t.errorf(el.Pos, "v-for 表达式错误: %v", err)
		}
		body := t.f.List(el.Pos)
		t.appendNode(t.f.Range(el.Pos, pipe, body, nil))
		t.blocks = append(t.blocks, &block{kind: "v-for", list: body, pos: el.Pos})
		t.scope.Push(loop.Frame)
		t.rangeDepth++
	}
	depth := len(t.blocks)

	t.f.AppendText(t.cur(), el.Pos, "<"+el.Name)
	for _, a := range el.Attrs {
		t.f.AppendText(t.cur(), a.ValuePos, " "+a.Name)
		if !a.HasValue {
			continue
		}
		t.f.AppendText(t.cur(), a.ValuePos, `="`)
		if a.Parts == nil {
			t.f.AppendText(t.cur(), a.ValuePos, a.Value)
		} else if err := t.writeNodes(a.Parts); err != nil {
			return err
		}
		if len(t.blocks) != depth {
			return t.errorf(a.ValuePos, "属性 %s 的值中原生控制结构未在同一个属性值内闭合", a.Name)
		}
		t.f.AppendText(t.cur(), a.ValuePos, `"`)
	}

	switch {
	case el.SelfClosing:
		t.f.AppendText(t.cur(), el.Pos, " />")
	case el.Void():
		t.f.AppendText(t.cur(), el.Pos, ">")
	default:
		t.f.AppendText(t.cur(), el.Pos, ">")
		t.elemDepth++
		if err := t.writeNodes(el.Children); err != nil {
			return err
		}
		t.elemDepth--
		if len(t.blocks) != depth {
			if len(t.blocks) > depth {
				b := t.blocks[len(t.blocks)-1]
				return t.errorf(b.pos, "{{ %s }} 跨越了 <%s> 的结束标签，AST 模式要求原生控制结构在同一个元素内闭合", b.kind, el.Name)
			}
			return t.errorf(el.Pos, "<%s> 内部的 {{ end }} 试图闭合元素外部的控制结构", el.Name)
		}
		t.f.AppendText(t.cur(), el.Pos, "</"+el.Name+">")
	}

	if loop != nil {
		t.blocks = t.blocks[:len(t.blocks)-1]
		t.scope.Pop()
		t.rangeDepth--
	}
	return nil
}

// native 把一个原生 html/template action 组装进当前语法树。
func (t *Transpiler) native(code string, pos int) error {
	if strings.HasPrefix(code, "/*") {
		// 模板注释：html/template 本身也不会输出注释，直接丢弃
		return nil
	}
	head, rest, restPos := splitHead(code, pos)

	switch head {
	case "if", "range", "with":
		pipe, err := t.f.ParseBranchPipe(head, rest, restPos, t.scope.Vars())
		if err != nil {
			return err
		}
		body := t.f.List(pos)
		b := &block{kind: head, list: body, pos: pos}
		switch head {
		case "if":
			n := t.f.If(pos, pipe, body, nil)
			b.branch = &n.BranchNode
			t.appendNode(n)
		case "range":
			n := t.f.Range(pos, pipe, body, nil)
			b.branch = &n.BranchNode
			b.inRangeBody = true
			t.rangeDepth++
			t.appendNode(n)
		case "with":
			n := t.f.With(pos, pipe, body, nil)
			b.branch = &n.BranchNode
			t.appendNode(n)
		}
		t.blocks = append(t.blocks, b)
		t.scope.PushBlock()
		t.declare(pipe)
		return nil

	case "else":
		return t.nativeElse(rest, restPos, pos)

	case "end":
		return t.nativeEnd(pos)

	case "define":
		if len(t.blocks) > 0 || t.elemDepth > 0 {
			return t.errorf(pos, "{{ define }} 只能出现在模板顶层，不能嵌套在元素或控制结构内")
		}
		name, err := strconv.Unquote(strings.TrimSpace(rest))
		if err != nil {
			return t.errorf(pos, "{{ define }} 的模板名必须是一个字符串字面量，实际为 %q", rest)
		}
		t.openDefine("define", name, pos)
		return nil

	case "block":
		// {{ block "name" pipe }} 等价于 {{ define "name" }}...{{ end }} + {{ template "name" pipe }}
		const kw = "template "
		n, err := t.f.ParseAction(kw+rest, restPos-len(kw), t.scope.Vars())
		if err != nil {
			return err
		}
		tn, ok := n.(*parse.TemplateNode)
		if !ok {
			return t.errorf(pos, "{{ block }} 语法应为 {{ block \"name\" pipeline }}")
		}
		t.appendNode(tn)
		t.openDefine("block", tn.Name, pos)
		return nil

	case "break", "continue":
		if rest != "" {
			return t.errorf(pos, "{{ %s }} 后不能跟参数", head)
		}
		if t.rangeDepth == 0 {
			return t.errorf(pos, "{{ %s }} 只能出现在 range / v-for 循环体内", head)
		}
		if head == "break" {
			t.appendNode(t.f.Break(pos))
		} else {
			t.appendNode(t.f.Continue(pos))
		}
		return nil

	default:
		n, err := t.f.ParseAction(code, pos, t.scope.Vars())
		if err != nil {
			return err
		}
		if a, ok := n.(*parse.ActionNode); ok {
			t.declare(a.Pipe)
		}
		t.appendNode(n)
		return nil
	}
}

// nativeElse 处理 {{ else }} / {{ else if ... }} / {{ else with ... }}。
func (t *Transpiler) nativeElse(rest string, restPos, pos int) error {
	if len(t.blocks) == 0 {
		return t.errorf(pos, "多余的 {{ else }}")
	}
	b := t.blocks[len(t.blocks)-1]
	if b.branch == nil {
		return t.errorf(pos, "{{ else }} 不能用于 %s", b.kind)
	}
	if b.hasElse {
		return t.errorf(pos, "同一个 {{ %s }} 中出现了多个 {{ else }}", b.kind)
	}
	if b.inRangeBody {
		b.inRangeBody = false
		t.rangeDepth--
	}

	if rest == "" {
		b.branch.ElseList = t.f.List(pos)
		b.list = b.branch.ElseList
		b.hasElse = true
		return nil
	}

	sub, subRest, subPos := splitHead(rest, restPos)
	if (sub != "if" || b.kind != "if") && (sub != "with" || b.kind != "with") {
		return t.errorf(pos, "不支持 {{ else %s }}：if 只能接 else if，with 只能接 else with", sub)
	}
	pipe, err := t.f.ParseBranchPipe(sub, subRest, subPos, t.scope.Vars())
	if err != nil {
		return err
	}
	inner := t.f.List(pos)
	var nested parse.Node
	var branch *parse.BranchNode
	if sub == "if" {
		n := t.f.If(pos, pipe, inner, nil)
		nested, branch = n, &n.BranchNode
	} else {
		n := t.f.With(pos, pipe, inner, nil)
		nested, branch = n, &n.BranchNode
	}
	b.branch.ElseList = t.f.List(pos)
	b.branch.ElseList.Nodes = append(b.branch.ElseList.Nodes, nested)
	// 后续的 else / end 都作用于内层分支，与 text/template 的 else if 语义一致
	b.branch = branch
	b.list = inner
	t.declare(pipe)
	return nil
}

// nativeEnd 处理 {{ end }}。
func (t *Transpiler) nativeEnd(pos int) error {
	if len(t.blocks) == 0 {
		return t.errorf(pos, "多余的 {{ end }}")
	}
	b := t.blocks[len(t.blocks)-1]
	if b.kind == "v-for" {
		return t.errorf(pos, "多余的 {{ end }}：v-for 元素内没有待闭合的原生控制结构")
	}
	t.blocks = t.blocks[:len(t.blocks)-1]

	switch b.kind {
	case "define", "block":
		t.scope, t.rangeDepth = b.savedScope, b.savedRange
		if t.defined[b.name] {
			return t.errorf(b.pos, "模板 %q 被重复定义", b.name)
		}
		t.defined[b.name] = true
		t.defines = append(t.defines, t.f.DefineTree(b.name, b.list))
	default:
		t.scope.Pop()
		if b.inRangeBody {
			t.rangeDepth--
		}
	}
	return nil
}

// openDefine 进入 define/block 体：使用全新的作用域与循环深度。
func (t *Transpiler) openDefine(kind, name string, pos int) {
	t.blocks = append(t.blocks, &block{
		kind: kind, name: name, list: t.f.List(pos), pos: pos,
		savedScope: t.scope, savedRange: t.rangeDepth,
	})
	t.scope = expr.NewScope()
	t.rangeDepth = 0
}

// declare 登记管道中新声明的模板变量（`$x := ...`，赋值 `$x = ...` 不算声明）。
func (t *Transpiler) declare(pipe *parse.PipeNode) {
	if pipe == nil || pipe.IsAssign {
		return
	}
	for _, v := range pipe.Decl {
		t.scope.Declare(v.Ident[0])
	}
}

// splitHead 把 `if .show` 拆成关键字 `if`、剩余部分 `.show` 及剩余部分在源码中的偏移。
func splitHead(code string, pos int) (head, rest string, restPos int) {
	i := strings.IndexAny(code, " \t\r\n")
	if i < 0 {
		return code, "", pos + len(code)
	}
	head = code[:i]
	j := i
	for j < len(code) && strings.ContainsRune(" \t\r\n", rune(code[j])) {
		j++
	}
	return head, strings.TrimSpace(code[j:]), pos + j
}
