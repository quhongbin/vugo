# `internal/node` — 标准库 `text/template/parse` 节点工厂与适配层

## 功能职责

`internal/node` 是 `vugo` 与 Go 标准库 `text/template/parse`（即 `html/template` 底层 AST）之间的核心适配层，包含两个文件：
- `factory.go`：提供 `Factory`，利用非常规定界符（`\x02` / `\x03`）在 `parse.Tree` 上解析「原始 `.vugo` 源码 + 原型模板」，从而在不调用任何未导出函数的前提下，获得内部私有字段 `tr *parse.Tree` 正确指向 `.vugo` 源码树的全部节点原型（`ListNode`、`TextNode`、`ActionNode`、`PipeNode`、`CommandNode`、`FieldNode`、`DotNode`、`VariableNode`、`StringNode`、`NumberNode`、`BoolNode`、`NilNode`、`IfNode`、`RangeNode`、`WithNode`、`TemplateNode`、`BreakNode`、`ContinueNode`）。后续所有节点通过克隆原型并改写导出字段与源码偏移 `Pos` 生成，确保直接调用 `(*html/template.Template).AddParseTree` 时不触发 `tr == nil` 的 panic，且转义报错行列号精确对应 `.vugo` 源文件。
- `native.go`：提供 `ParseBranchPipe` 与 `ParseAction`，结合当前可见的模板变量列表（`vars`），将原生 `html/template` 动作（如 `if .show`、`range $i, $v := .list`、`len .items | printf "%d"`）解析为标准库 `parse.PipeNode` 或 `parse.Node`。

## 接收内容（输入）

- `node.New(name, src string)`：接收模板名称 `name` 与完整的 `.vugo` 源码 `src`。
- 各节点构造方法（`List`、`Text`、`Action`、`Pipe`、`Command`、`Field`、`Range`、`If` 等）：接收源码字节偏移 `pos` 及对应子节点/字面量参数。
- `ParseBranchPipe` / `ParseAction`：接收原生动作代码片段、源码字节偏移 `pos` 以及当前作用域内已声明的 `$变量` 切片。

## 输出内容（输出）

- 标准库 `text/template/parse` 包下的各类 AST 节点（`parse.Node`、`*parse.ListNode`、`*parse.PipeNode` 等）以及可直接传给 `html/template.AddParseTree` 的主树 `MainTree` 与子模板树 `DefineTree`。

## 测试示例

```go
func TestFactoryAddParseTree(t *testing.T) {
	src := "<p>{{ user.name }}</p>"
	f, err := node.New("demo.vugo", src)
	if err != nil {
		t.Fatal(err)
	}

	// 手工用 Factory 组装: <p>{{ .user.name }}</p>
	root := f.List(0)
	f.AppendText(root, 0, "<p>")
	pipe := f.Pipe(6, nil, f.Command(6, f.Field(6, "user", "name")))
	root.Nodes = append(root.Nodes, f.Action(6, pipe))
	f.AppendText(root, 18, "</p>")

	tree := f.MainTree(root)

	// 直接挂载到 html/template 并执行
	tmpl, err := template.New("demo.vugo").AddParseTree("demo.vugo", tree)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_ = tmpl.Execute(&buf, map[string]any{"user": map[string]any{"name": "<Alice>"}})
	t.Logf("渲染输出: %s", buf.String()) // 输出: <p>&lt;Alice&gt;</p>
}
```

