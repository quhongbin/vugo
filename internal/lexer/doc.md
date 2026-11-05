# `internal/lexer` — 模板词法分析器

## 功能职责

`internal/lexer` 是转译流水线的第一层：将原始的类 Vue 模板源码（`.vugo` 字符串）按字符流扫描，切分为扁平的词法单元（`Token`）序列，并精确记录每个 Token 的行号（`Line`）、起始字节偏移（`Pos`）以及内容起始偏移（`RawPos`），供后续语法分析及 `text/template/parse` 节点报错定位使用。

## 接收内容（输入）

- `src string`：完整的类 Vue 模板源码字符串。

## 输出内容（输出）

- `[]lexer.Token`：以 `EOF` 结尾的 Token 序列。每个 `Token` 包含：
  - `Type TokenType`：Token 类型（`Text` / `Mustache` / `Comment` / `OpenTag` / `SelfCloseTag` / `CloseTag` / `EOF`）
  - `Name string`：标签名（仅标签类 Token 有效，统一转为小写）
  - `Raw string`：文本原文、Mustache 内部表达式或标签属性原文
  - `Line int`：起始行号（从 1 开始）
  - `Pos int`：Token 在源码中的起始字节偏移
  - `RawPos int`：`Raw` 内容在源码中的起始字节偏移
- `error`：当遇到未闭合的 `{{`、未闭合的 `<!--` 或未闭合的 HTML 标签时返回带行号的错误（`vugo: 第 N 行: ...`）。

## 测试示例

在 `internal/lexer/lexer_test.go` 中可通过以下测试直观查看输入源码如何被切分为 Token 序列：

```go
func TestRun(t *testing.T) {
	src := "<h1>{{ title }}</h1><br />\n"
	tokens, err := lexer.New(src).Run()
	if err != nil {
		t.Fatal(err)
	}
	for i, tok := range tokens {
		t.Logf("[%d] Type=%-16s Name=%-4s Raw=%q Line=%d Pos=%d RawPos=%d",
			i, tok.Type, tok.Name, tok.Raw, tok.Line, tok.Pos, tok.RawPos)
	}
}
```

运行命令：

```bash
go test -v -run TestRun ./internal/lexer
```

预期拆解出的 Token 序列：
1. `OpenTag`：`Name="h1"`, `Pos=0`
2. `Mustache`：`Raw="title"`, `Pos=4`, `RawPos=7`
3. `CloseTag`：`Name="h1"`, `Pos=15`
4. `SelfCloseTag`：`Name="br"`, `Pos=20`
5. `Text`：`Raw="\n"`, `Pos=26`
6. `EOF`：`Pos=27`

