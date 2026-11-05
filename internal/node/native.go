package node

import (
	"fmt"
	"strings"
	"text/template/parse"
)

// ParseBranchPipe 解析原生 `{{ if/range/with <pipeline> }}` 的管道部分。
//
// 实现方式：拼出 `{{keyword pipeline}}{{end}}` 交给 text/template/parse 解析，再取出 BranchNode.Pipe。
// 因为 parse 会检查变量是否已声明，调用方需要把当前可见的模板变量通过 vars 传入。
//
// 参数:
//   - keyword:  "if"、"range" 或 "with"。
//   - pipeline: 关键字之后的管道源码，如 `.show`、`$i, $v := .list`。
//   - pos:      pipeline 首字节在 .vugo 源码中的偏移，用于让报错行列与源码一致。
//   - vars:     当前可见的模板变量（含 `$` 前缀，不含根变量 `$` 本身）。
//
// 返回值:
//   - *parse.PipeNode: 解析得到的管道，其中节点的 tr 指向一棵只用于定位的临时树。
//   - error: 管道语法错误时返回，信息以 `vugo: 原生 action` 开头并附带 parse 的原始错误。
func (f *Factory) ParseBranchPipe(keyword, pipeline string, pos int, vars []string) (*parse.PipeNode, error) {
	n, err := f.parseSnippet(keyword+" "+pipeline, leftDelim+"end"+rightDelim, pos-len(keyword)-1, vars)
	if err != nil {
		return nil, err
	}
	switch b := n.(type) {
	case *parse.IfNode:
		return b.Pipe, nil
	case *parse.RangeNode:
		return b.Pipe, nil
	case *parse.WithNode:
		return b.Pipe, nil
	}
	return nil, fmt.Errorf("vugo: 第 %d 行: 原生 action %q 不是 %s 语句", f.Line(pos), pipeline, keyword)
}

// ParseAction 解析一个完整的原生单动作，如 `.Title`、`len .items`、`$x := 1`、`template "card" .`。
//
// 参数:
//   - code: 花括号内的原始代码。
//   - pos:  code 首字节在 .vugo 源码中的偏移。
//   - vars: 当前可见的模板变量（含 `$` 前缀，不含根变量 `$` 本身）。
//
// 返回值:
//   - parse.Node: *parse.ActionNode 或 *parse.TemplateNode。
//   - error: 语法错误、使用了未声明的变量，或 code 是需要配对 {{ end }} 的控制结构时返回，
//     信息以 `vugo: 原生 action` 开头。
func (f *Factory) ParseAction(code string, pos int, vars []string) (parse.Node, error) {
	n, err := f.parseSnippet(code, "", pos, vars)
	if err != nil {
		return nil, err
	}
	switch n.(type) {
	case *parse.ActionNode, *parse.TemplateNode:
		return n, nil
	}
	return nil, fmt.Errorf("vugo: 第 %d 行: 原生 action %q 无法作为单个动作解析", f.Line(pos), code)
}

// parseSnippet 解析一段原生 action 并返回解析结果的最后一个顶层节点。
//
// 临时文本的布局为：变量声明 + 换行填充 + 空格填充 + 片段，
// 换行与空格填充让片段在临时树中的行列与 .vugo 源码一致。
func (f *Factory) parseSnippet(code, suffix string, pos int, vars []string) (parse.Node, error) {
	var decls strings.Builder
	for _, v := range vars {
		decls.WriteString(leftDelim + v + " := 0" + rightDelim)
	}
	if pos < 0 {
		pos = 0
	}
	if pos > len(f.src) {
		pos = len(f.src)
	}
	line := f.Line(pos)
	col := pos - (strings.LastIndex(f.src[:pos], "\n") + 1)

	var b strings.Builder
	b.WriteString(decls.String())
	b.WriteString(strings.Repeat("\n", line-1))
	pad := col - len(leftDelim)
	if line == 1 {
		pad -= decls.Len()
	}
	if pad > 0 {
		b.WriteString(strings.Repeat(" ", pad))
	}
	b.WriteString(leftDelim + code + rightDelim + suffix)

	t := parse.New(f.name)
	t.Mode = parse.SkipFuncCheck
	if _, err := t.Parse(b.String(), leftDelim, rightDelim, map[string]*parse.Tree{}); err != nil {
		return nil, fmt.Errorf("vugo: 第 %d 行: 原生 action %q 解析失败: %v", line, code, err)
	}
	t.ParseName = f.name
	if t.Root == nil || len(t.Root.Nodes) == 0 {
		return nil, fmt.Errorf("vugo: 第 %d 行: 原生 action %q 为空", line, code)
	}
	return t.Root.Nodes[len(t.Root.Nodes)-1], nil
}
