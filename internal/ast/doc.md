# `internal/ast` — vugo 模板抽象语法树定义

## 功能职责

`internal/ast` 定义了 `vugo` 前端的模板抽象语法树（AST）节点数据结构。它作为 `internal/parser`（生产者）与 `internal/transpiler`（消费者）之间的数据契约，只描述“类 Vue 模板长什么样”，不包含任何词法扫描或代码改写逻辑。每个节点都保留了在 `.vugo` 源码中的行号（`Line`）和字节偏移（`Pos` / `ValuePos`）。

## 接收内容（输入）

本包为纯数据结构定义包，由 `internal/parser` 实例化并填充各节点字段：
- `Document`：整份模板的根节点，持有顶层 `Nodes []Node`。
- `Text`：普通文本与 HTML 注释。
- `Interpolation`：类 Vue 插值 `{{ expr }}`。
- `GoAction`：原生 `html/template` 动作（如 `{{ if .show }}`、`{{ define "x" }}`）。
- `Element`：HTML 元素节点，包含普通属性 `Attrs []Attr`（若属性值含 `{{ }}` 则预拆分为 `Parts []Node`）、指令 `Directives []Directive`（如 `v-for`）、绑定 `Bindings []Binding`（`:attr` / `@event`）以及子节点 `Children []Node`。

## 输出内容（输出）

- 提供实现 `ast.Node` 接口的节点树 `*ast.Document`，以及判断 HTML 自闭合空元素的 `(*Element).Void()` 方法，供 `internal/transpiler` 遍历并转换为 `text/template/parse.Tree`。

## 测试示例

```go
func ExampleDocument() {
	doc := &ast.Document{
		Nodes: []ast.Node{
			&ast.Element{
				Name: "li",
				Directives: []ast.Directive{
					{Name: "for", Value: "item in list", Line: 1, ValuePos: 11},
				},
				Children: []ast.Node{
					&ast.Interpolation{Expr: "item.name", Line: 1, Pos: 28},
				},
				Line: 1,
				Pos:  0,
			},
		},
	}
	el := doc.Nodes[0].(*ast.Element)
	fmt.Printf("tag=%s void=%v directive=v-%s expr=%s\n",
		el.Name, el.Void(), el.Directives[0].Name, el.Children[0].(*ast.Interpolation).Expr)
	// Output: tag=li void=false directive=v-for expr=item.name
}
```

