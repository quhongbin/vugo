package transpiler

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"text/template/parse"
)

// Result 是一份 .vugo 模板的转译结果：一棵主树加上 define/block 产生的子树。
//
// 所有树都可以直接交给 html/template 的 (*Template).AddParseTree。
type Result struct {
	// Name 是主模板名。
	Name string
	// Main 是主模板的语法树，Tree.Name == Name。
	Main *parse.Tree
	// Defines 是 {{ define }} / {{ block }} 定义的模板，按出现顺序排列。
	Defines []*parse.Tree
}

// Trees 返回全部语法树，主树在前、子树按定义顺序在后。
//
// 返回值:
//   - []*parse.Tree: 新分配的切片，修改切片本身不影响 Result，但其中的树是共享的。
func (r *Result) Trees() []*parse.Tree {
	return append([]*parse.Tree{r.Main}, r.Defines...)
}

// AddTo 将当前转译产出的全部语法树（主树及所有 define/block 子树）通过
// (*template.Template).AddParseTree 挂载到目标 html/template.Template 上。
//
// 为避免 html/template 在后续转义（escape）阶段原地修改 Result 持有的语法树，
// 每次挂载都会调用 Tree.Copy() 传入副本。
//
// 参数:
//   - tmpl: 目标模板实例，不能为 nil，且尚未执行过 Execute。
//
// 返回值:
//   - *template.Template: 挂载主树后的模板实例。
//   - error: 当 tmpl 已经执行过或 AddParseTree 失败时返回非 nil 错误，信息以 `vugo:` 开头。
func (r *Result) AddTo(tmpl *template.Template) (*template.Template, error) {
	for _, d := range r.Defines {
		if _, err := tmpl.AddParseTree(d.Name, d.Copy()); err != nil {
			return nil, fmt.Errorf("vugo: 挂载子模板 %q 语法树失败: %w", d.Name, err)
		}
	}
	mainName := r.Name
	if mainName == "" {
		mainName = tmpl.Name()
	}
	mainTree := r.Main.Copy()
	mainTree.Name = mainName
	mainTmpl, err := tmpl.AddParseTree(mainName, mainTree)
	if err != nil {
		return nil, fmt.Errorf("vugo: 挂载主模板 %q 语法树失败: %w", mainName, err)
	}
	return mainTmpl, nil
}

// String 把语法树序列化为等价的 html/template 源码，供 CLI 写出 .tmpl 文件。
//
// 序列化通过遍历 parse 语法树节点生成，动作统一写成 `{{ expr }}` 带空格的规范格式；
// define/block 子树会以 `{{ define "name" }}...{{ end }}` 的形式写在主模板之前。
//
// 返回值:
//   - string: 可被 html/template.Parse 解析、语义与语法树一致的模板源码。
func (r *Result) String() string {
	var b strings.Builder
	for _, d := range r.Defines {
		b.WriteString("{{ define " + strconv.Quote(d.Name) + " }}")
		writeList(&b, d.Root)
		b.WriteString("{{ end }}")
	}
	writeList(&b, r.Main.Root)
	return b.String()
}

// writeList 递归将 ListNode 序列化为标准 html/template 源码。
func writeList(b *strings.Builder, l *parse.ListNode) {
	if l == nil {
		return
	}
	for _, n := range l.Nodes {
		writeParseNode(b, n)
	}
}

// writeParseNode 将单个 parse.Node 序列化为标准 html/template 源码。
func writeParseNode(b *strings.Builder, n parse.Node) {
	switch v := n.(type) {
	case *parse.TextNode:
		b.Write(v.Text)
	case *parse.ActionNode:
		b.WriteString("{{ " + v.Pipe.String() + " }}")
	case *parse.IfNode:
		writeBranch(b, "if", &v.BranchNode)
	case *parse.RangeNode:
		writeBranch(b, "range", &v.BranchNode)
	case *parse.WithNode:
		writeBranch(b, "with", &v.BranchNode)
	case *parse.TemplateNode:
		b.WriteString("{{ template " + strconv.Quote(v.Name))
		if v.Pipe != nil {
			b.WriteString(" " + v.Pipe.String())
		}
		b.WriteString(" }}")
	case *parse.BreakNode:
		b.WriteString("{{ break }}")
	case *parse.ContinueNode:
		b.WriteString("{{ continue }}")
	}
}

// writeBranch 将 IfNode/RangeNode/WithNode 序列化，并对 else-if / else-with 还原紧凑写法。
func writeBranch(b *strings.Builder, kw string, br *parse.BranchNode) {
	b.WriteString("{{ " + kw + " " + br.Pipe.String() + " }}")
	writeList(b, br.List)
	writeElse(b, br.ElseList)
	b.WriteString("{{ end }}")
}

// writeElse 处理分支节点的 ElseList，识别嵌套的单 IfNode / WithNode 还原为 `{{ else if ... }}`。
func writeElse(b *strings.Builder, elseList *parse.ListNode) {
	if elseList == nil {
		return
	}
	if len(elseList.Nodes) == 1 {
		switch sub := elseList.Nodes[0].(type) {
		case *parse.IfNode:
			b.WriteString("{{ else if " + sub.Pipe.String() + " }}")
			writeList(b, sub.List)
			writeElse(b, sub.ElseList)
			return
		case *parse.WithNode:
			b.WriteString("{{ else with " + sub.Pipe.String() + " }}")
			writeList(b, sub.List)
			writeElse(b, sub.ElseList)
			return
		}
	}
	b.WriteString("{{ else }}")
	writeList(b, elseList)
}
