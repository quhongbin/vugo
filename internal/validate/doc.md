# `internal/validate` — 语法树与模板产物校验器

## 功能职责

`internal/validate` 是转译流水线的第四层（安全网），负责验证转译器构建出的 `[]*parse.Tree` 语法树以及序列化后的模板源码完全符合 Go 标准库 `html/template` 的规范：
- `Trees(name string, trees []*parse.Tree, funcs template.FuncMap) error`：递归遍历每棵 `parse.Tree` 中的所有命令节点，检查调用的每个函数（`parse.IdentifierNode`）是否已在内置函数表或 `funcs` 中注册，随后调用 `(*html/template.Template).AddParseTree` 验证每棵语法树都能被标准库接收。
- `Template(name, src string, funcs template.FuncMap) error`：调用 `html/template.New(name).Funcs(funcs).Parse(src)` 对序列化后的模板源码进行二次校验。

## 接收内容（输入）

- `name string`：模板名称。
- `trees []*parse.Tree` 或 `src string`：待校验的语法树切片或模板源码字符串。
- `funcs template.FuncMap`：业务注册的自定义模板函数映射表。

## 输出内容（输出）

- `error`：校验通过返回 `nil`；当存在未定义函数、语法树非法或 `Parse` 失败时返回以 `vugo:` 开头的错误。

## 测试示例

```go
func TestValidateTrees(t *testing.T) {
	res, err := compiler.New().CompileTree(`<p>{{ msg }}</p>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := validate.Trees("vugo", res.Trees(), nil); err != nil {
		t.Fatalf("校验失败: %v", err)
	}
}
```

运行命令：

```bash
go test -v -run TestCompileValidateToggle ./internal/compiler
```

