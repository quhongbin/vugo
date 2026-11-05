# `internal/expr` — 类 Vue 表达式到 `parse.PipeNode` 构建引擎

## 功能职责

`internal/expr` 负责将类 Vue 表达式（如 `user.name`、`formatTime(time)`、`a && !b`、`count >= 10`）解析并直接构建为 Go 标准库 `text/template/parse` 的管道节点（`*parse.PipeNode`）。本包是一个独立的子语言层，由 5 个文件组成：
- `scanner.go`：表达式词法扫描，识别标识符、数字、字符串（单引号自动转为双引号）、标点及运算符（`===` 归一化为 `==`，`!==` 归一化为 `!=`）。
- `parser.go`：按运算符优先级递归下降解析表达式，生成表达式 AST。
- `ast.go`：定义表达式 AST 节点（`Ident`、`Call`、`Literal`、`Paren`、`Unary`、`Binary`）。
- `scope.go`：维护 `v-for` 循环变量与原生 `$变量` 的词法作用域栈（`Scope`），支持自内向外将类 Vue 循环变量解析为点作用域 `.`（`DotScope`）或命名模板变量 `$var`。
- `build.go`：遍历表达式 AST，调用 `internal/node.Factory` 直接构造 `*parse.PipeNode`（将 `==`/`!=`/`>`/`<`/`&&`/`||`/`!` 转化为 `eq`/`ne`/`gt`/`lt`/`and`/`or`/`not` 的 `CommandNode`，复合子表达式嵌套为括号 `PipeNode`）。

## 接收内容（输入）

- `Build(src string, base int, scope *Scope, f *node.Factory)`：
  - `src`：类 Vue 表达式字符串（如 `a && !b`、`item.name`）。
  - `base`：表达式首字节在 `.vugo` 源码中的偏移。
  - `scope`：当前变量作用域栈（传 `nil` 表示根作用域）。
  - `f`：`internal/node.Factory` 节点工厂。

## 输出内容（输出）

- `*parse.PipeNode`：可直接装入 `parse.ActionNode` 或 `parse.BranchNode` 的标准库管道 AST 节点。
- `error`：当表达式语法非法，或包含 `html/template` AST 无法直接表达的语法（如中缀算术运算 `+ - * / %`、带点号的函数名 `pkg.Fn()`、未声明的 `$变量`、单独输出 `null`）时返回明确错误。

## 测试示例

参见 `internal/expr/expr_test.go` 中的 `TestBooleanFunctionsAreValidGoTemplate`：

```go
func TestBooleanFunctionsAreValidGoTemplate(t *testing.T) {
	const exprSrc = "a && !b"
	f, _ := node.New("t", exprSrc)
	pipe, err := Build(exprSrc, 0, nil, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PipeNode.String() = %s", pipe.String()) // 输出: and .a (not .b)

	root := f.List(0)
	root.Nodes = append(root.Nodes, f.Action(0, pipe))
	tmpl, _ := template.New("t").AddParseTree("t", f.MainTree(root))

	var sb strings.Builder
	_ = tmpl.Execute(&sb, map[string]any{"a": true, "b": false})
	t.Logf("Execute 结果 = %s", sb.String()) // 输出: true
}
```

运行命令：

```bash
go test -v -run TestBooleanFunctionsAreValidGoTemplate ./internal/expr
```

