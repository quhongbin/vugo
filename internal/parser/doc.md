# `internal/parser` — 模板语法分析器

## 功能职责

`internal/parser` 是转译流水线的第二层：接收 `internal/lexer` 产出的 Token 流，维护开标签栈还原 HTML 树形嵌套结构，将标签属性分类为普通属性（`Attr`）、指令（`Directive`）与绑定（`Binding`），并通过 `isGoAction` 区分原生 `html/template` 动作与类 Vue 插值表达式，最终输出 `*ast.Document`。当普通属性值引号内包含 `{{ ... }}` 时，`splitValue` 会将其进一步拆分为 `Text` / `Interpolation` / `GoAction` 子片段。

## 接收内容（输入）

- `ParseString(src string)`：直接接收类 Vue 模板源码字符串；
- 或 `Parse(tokens []lexer.Token)`：接收词法分析器生成的 Token 切片。

## 输出内容（输出）

- `*ast.Document`：结构化的 vugo 模板 AST 根节点。
- `error`：当标签未闭合、闭合标签不匹配、属性语法非法或属性列表中裸写 `{{ }}` 时返回带行号的错误（`vugo: 第 N 行: ...`）。

## 测试示例

在 `internal/parser/parser_test.go` 中可通过以下测试直观理解 `parser` 如何将标签属性与插值分类为 AST 节点：

```go
func TestParseDirectiveClassification(t *testing.T) {
	doc, err := ParseString(`<a v-bind:href="url" v-on:click="go" @submit="s" :key="x.id" v-for="u in users">{{ u.name }}</a>`)
	if err != nil {
		t.Fatal(err)
	}
	el := doc.Nodes[0].(*ast.Element)
	t.Logf("标签名: %s", el.Name)
	t.Logf("指令列表 Directives: %+v", el.Directives)
	t.Logf("绑定列表 Bindings:   %+v", el.Bindings)
	t.Logf("子节点 Children[0]:  %+v", el.Children[0])
}
```

运行命令：

```bash
go test -v -run TestParseDirectiveClassification ./internal/parser
```

输出效果：
- `el.Directives` 包含 1 个 `v-for` 指令：`{Name:"for", Value:"u in users"}`
- `el.Bindings` 包含 4 个属性/事件绑定（`href`、`click`、`submit`、`key`）
- `el.Children[0]` 为 `*ast.Interpolation{Expr:"u.name"}`

