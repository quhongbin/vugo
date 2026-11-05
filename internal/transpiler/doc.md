# `internal/transpiler` — 语法树转译器（vugo AST → `parse.Tree`）

## 功能职责

`internal/transpiler` 是转译流水线的第三层（核心后端），负责遍历 `internal/ast.Document` 并直接构建出 `html/template` 可通过 `AddParseTree` 接收的 `*parse.Tree` 集合。包含 4 个文件：
- `transpiler.go`：遍历 vugo AST，维护作用域栈与控制流块栈（`blocks`）。将文本转为 `parse.TextNode`，插值转为 `parse.ActionNode`，`v-for` 元素包裹为 `parse.RangeNode`，并将原生 `{{ if }}` / `{{ else if }}` / `{{ else }}` / `{{ range }}` / `{{ with }}` / `{{ define }}` / `{{ block }}` / `{{ template }}` / `{{ break }}` / `{{ continue }}` 组装为对应的标准库 `parse` 节点。
- `for.go`：解析 `v-for="item in list"` 与 `v-for="(item, index) in list"`，构建 `RangeNode` 的 `PipeNode`（自动互换双变量的下标与元素声明顺序 `$index, $item := .list`）并生成作用域帧。
- `directive.go`：检查元素上的指令与属性/事件绑定，遇到不支持的指令（如 `v-if`、`v-model`、`v-slot`、`:href`、`@click`）时返回带行号及原生替代方案的错误。
- `result.go`：封装转译产物 `Result`（含主树 `Main *parse.Tree` 与子模板树 `Defines []*parse.Tree`），提供一键挂载到 `*html/template.Template` 的 `AddTo(tmpl)` 方法，以及将 `parse.Tree` 还原为标准 `.tmpl` 源码的 `String()` 方法。

## 接收内容（输入）

- `doc *ast.Document`：`internal/parser` 产出的 vugo 模板 AST。
- `name string`：主模板名称。
- `src string`：原始 `.vugo` 模板源码（用于计算节点 `Pos` 及定位报错行号）。

## 输出内容（输出）

- `*transpiler.Result`：
  - `Result.Main`：主模板 `*parse.Tree`
  - `Result.Defines`：所有 `{{ define }}` / `{{ block }}` 定义的子模板 `[]*parse.Tree`
  - `Result.AddTo(tmpl *template.Template)`：直接调用 `tmpl.AddParseTree` 将所有语法树挂载到目标 `html/template.Template`
  - `Result.String()`：将语法树序列化为标准 `html/template` 源码字符串
- `error`：遇到不支持的指令、表达式错误或跨标签未闭合的控制结构时返回 `vugo: 第 N 行: ...` 错误。

## 测试示例

参见 `internal/transpiler/transpiler_test.go`：

```go
func TestNestedForExecution(t *testing.T) {
	src := `<div><section v-for="group in groups"><h2>{{ group.title }}</h2>` +
		`<ul><li v-for="(child, idx) in group.children">{{ idx }}. {{ child.name }}</li></ul>` +
		`</section></div>`

	// 构建 parse.Tree 并通过 AddParseTree 挂载到 html/template 执行
	tmpl, err := compiler.New().CompileTemplate(src)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	data := map[string]any{
		"groups": []map[string]any{
			{"title": "A", "children": []map[string]any{{"name": "a1"}, {"name": "a2"}}},
		},
	}
	_ = tmpl.Execute(&buf, data)
	t.Logf("渲染结果: %s", buf.String())
	// 输出: <div><section><h2>A</h2><ul><li>0. a1</li><li>1. a2</li></ul></section></div>
}
```

运行命令：

```bash
go test -v -run TestNestedForExecution ./internal/transpiler
```

