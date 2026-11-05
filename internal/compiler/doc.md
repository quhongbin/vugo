# `internal/compiler` — 转译流水线编排层

## 功能职责

`internal/compiler` 是面向调用方（CLI 及 Go 代码集成方）的高层编排入口，将词法分析、语法分析、语法树构建与校验串联为完整流水线：

`源码 (.vugo) ──lexer──> Token 流 ──parser──> vugo AST ──transpiler──> parse.Tree ──AddParseTree──> html/template.Template`

主要公开方法：
- `(*Compiler).CompileTree(src string) (*transpiler.Result, error)`：输出包含主树与 `define`/`block` 子树的 `*transpiler.Result`（底层为 `*parse.Tree`）。
- `(*Compiler).CompileTemplate(src string) (*template.Template, error)`：构建 `parse.Tree` 并直接通过 `(*template.Template).AddParseTree` 挂载到新建的 `*html/template.Template` 返回。
- `(*Compiler).AddToTemplate(dst *template.Template, src string) (*template.Template, error)`：构建 `parse.Tree` 并通过 `AddParseTree` 挂载到调用方现有的 `*html/template.Template`。
- `(*Compiler).Compile(src string) (string, error)` / `CompileFile(path string)`：构建 `parse.Tree` 后序列化为标准 `.tmpl` 源码字符串，供 CLI 生成模板文件。

## 接收内容（输入）

- 类 Vue 模板源码字符串 `src` 或文件路径 `path`。
- 可选配置：`Validate bool`（是否开启校验，默认 `true`）、`Name string`（模板名）、`Funcs template.FuncMap`（自定义函数表）。

## 输出内容（输出）

- `*transpiler.Result`（含 `*parse.Tree`）、`*html/template.Template`（已挂载语法树）或序列化后的 `html/template` 源码 `string`。
- `error`：任何阶段出错均返回以 `vugo:`（或文件路径）开头的错误。

## 测试示例

参见 `internal/compiler/compiler_test.go` 中的 `TestCompileTemplateAndAddParseTree`：

```go
func TestCompileTemplateAndAddParseTree(t *testing.T) {
	src := `{{ define "badge" }}<span>[{{ label }}]</span>{{ end }}` +
		`<div>{{ template "badge" . }}<ul><li v-for="(item, idx) in items">{{ idx }}:{{ item.name }}</li></ul></div>`

	c := compiler.New()
	tmpl, err := c.CompileTemplate(src)
	if err != nil {
		t.Fatalf("CompileTemplate 失败: %v", err)
	}

	var sb strings.Builder
	data := map[string]any{
		"label": "<admin>",
		"items": []map[string]any{{"name": "<b>alice</b>"}, {"name": "bob"}},
	}
	_ = tmpl.Execute(&sb, data)
	t.Logf("渲染输出: %s", sb.String())
	// 输出: <div><span>[&lt;admin&gt;]</span><ul><li>0:&lt;b&gt;alice&lt;/b&gt;</li><li>1:bob</li></ul></div>
}
```

运行命令：

```bash
go test -v -run TestCompileTemplateAndAddParseTree ./internal/compiler
```

