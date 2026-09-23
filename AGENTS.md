### 项目概述

本项目名为`vugo`（占位名，可替换为最终确定的项目名），核心目标是将类Vue语法的模板转译为Go标准库`html/template`支持的模板语法，提升模板的人类可读性，同时完全兼容现有Go模板生态，不引入额外运行时依赖，无需改变用户现有的构建流程。

### 核心原则

- 转译输出必须是纯`html/template`标准语法，不能残留任何自定义指令
- 完全复用`html/template`的自动XSS转义能力，禁止自行实现转义逻辑
- 保留`html/template`的所有原生能力，包括模板继承（`{{ define }}`/`{{ template }}`）、自定义函数、管道操作等
- 转译后的模板必须通过`html/template`的`Parse`校验，无语法错误
- 语义必须和原始类Vue模板完全一致，不能丢失任何逻辑

### 转译映射规范

#### 基础变量插值
| 类Vue语法 | 转译后html/template语法 |
|---|---|
| `{{ msg }}` | `{{ .msg }}` |
| `{{ user.name }}` | `{{ .user.name }}` |
| `{{ formatTime(time) }}` | `{{ formatTime .time }}`（需确保`formatTime`已注册为模板函数） |

#### 条件渲染
| 类Vue语法 | 转译后html/template语法 |
|---|---|
| `<div v-if="show">内容</div>` | `{{ if .show }}<div>内容</div>{{ end }}` |
| `<div v-else-if="other">内容</div>` | `{{ else if .other }}<div>内容</div>` |
| `<div v-else>兜底内容</div>` | `{{ else }}<div>兜底内容</div>` |

#### 列表渲染
| 类Vue语法 | 转译后html/template语法 |
|---|---|
| `<li v-for="(item, index) in list" :key="item.id">{{ item.name }}</li>` | `{{ range $index, $item := .list }}<li>{{ $item.name }}</li>{{ end }}` |
| `<li v-for="item in list">{{ item }}</li>` | `{{ range .list }}<li>{{ . }}</li>{{ end }}` |

#### 属性绑定
| 类Vue语法 | 转译后html/template语法 |
|---|---|
| `<a :href="url">链接</a>` | `<a href="{{ .url }}">链接</a>` |
| `<div :class="activeClass">内容</div>` | `<div class="{{ .activeClass }}">内容</div>` |
| `<input :disabled="isDisabled" />` | `<input {{ if .isDisabled }}disabled{{ end }} />` |

#### 组件引用
| 类Vue语法 | 转译后html/template语法 |
|---|---|
| `<MyComponent :title="pageTitle" />` | `{{ template "MyComponent" . }}`（需提前用`{{ define "MyComponent" }}`定义组件模板） |

### 禁止行为

- 禁止生成Go源文件，所有输出必须是`.html`/`.tmpl`模板文件
- 禁止修改`html/template`的默认转义行为，不要手动调用`template.HTML`等不安全函数
- 禁止引入第三方运行时依赖，转译工具本身可以是独立二进制，但生成的模板不需要额外依赖
- 禁止破坏原有模板的`{{ define }}`/`{{ template }}`继承结构，遇到已有的模板定义必须保留

### 不支持的语法处理

遇到以下Vue特有、`html/template`无法直接实现的语法时，必须明确告知用户不支持，并给出替代方案：

- `v-model`双向绑定：服务端模板无双向绑定能力，建议拆分为`v-bind:value`展示数据 + `v-on:input`提交到后端处理
- `v-slot`具名插槽：建议改用`html/template`的`{{ define }}`+`{{ template }}`组合实现类似效果
- 响应式计算属性：建议提前在Go后端计算好数据，直接传递结果到模板

### 工作流程

1. 接收用户输入的类Vue语法模板
2. 解析模板中的指令、变量引用、组件调用
3. 按照转译映射规范转换为标准`html/template`语法
4. 校验转译后的模板是否符合`html/template`语法规范
5. 输出转译后的模板文件，并标注所有不支持的语法及替代方案

### 测试要求

- 所有转译用例必须包含对应的原始模板和转译后模板对照
- 转译后的模板必须能通过`html/template.Parse`校验
- 必须包含XSS防护测试用例，确保用户输入会被自动转义
- 必须覆盖循环嵌套、条件嵌套、属性绑定组合等复杂场景

### 代码注释规范

所有导出的 API（包级函数、类型、方法、结构体或接口字段）都必须写规范的 godoc 风格注释，使调用者把鼠标悬停到函数名上即可看见每个参数与返回值的含义。

强制要求：

1. **每个导出标识符都必须有注释**：以标识符名开头的一句话说明（`// Foo xxx ...`）。包级注释遵循 `// Package foo ...` 形式，函数注释以函数名开头。
2. **参数必须显式列出**：用 `// 参数名: 语义说明。` 的形式逐个解释，不要只写一句笼统的话。建议以「`name` — 含义」「`name` string — 含义」或独立的「Parameters:」段呈现，使 godoc 自动生成参数表格。
3. **返回值必须说明**：单返回值要写出含义；多返回值要逐个解释，包括 `error` 的触发条件（什么输入/状态下会返回错误、错误信息是否包含上下文等）。导出会被外部调用的构造函数还应说明零值/默认行为。
4. **副作用与并发语义**：是否会修改入参（如 `Reset`、`Pop`），是否持有资源、是否线程安全，要在注释中明确写出。
5. **错误信息约束**：若函数会拼接错误信息，应说明前缀格式（如 `vugo: 第 N 行:`），方便调用者做错误匹配或截取。
6. **示例（可选但推荐）**：当函数行为不显然时，给出最小可运行的 `Example` 或在注释里附上一段调用代码，对调用方最有价值。
7. **同类函数保持风格一致**：同一包内相似函数（如 `scan*`/`parse*`/`render*`）应使用相同的注释结构，方便阅读者快速对比差异。

反例（禁止）：

```go
// New 创建一个东西。
func New() *Thing
// 渲染一个表达式。
func Render(s string) string
```

正例：

```go
// New 创建一个空作用域，调用方可持有返回值并在多个 goroutine 间复用。
//
// 返回值:
//
//	*Scope — 初始未进入任何帧的作用域，可立即调用 Push/Pop。
func NewScope() *Scope

// Render 把一个类 Vue 表达式转译为 html/template 表达式源码。
//
// 参数:
//   - src:   表达式原文，如 `user.name`、`formatTime(time)`。
//   - scope: 当前作用域，传 nil 时按根作用域处理。
//
// 返回值:
//   - string: 已转译好的源码片段（不含外层 `{{`/`}}`），可直接拼入模板。
//   - error:  解析或渲染失败时返回非 nil 错误，错误信息以 `vugo: 表达式 ...` 开头。
func Render(src string, scope *Scope) (string, error)
```

### 文档同步约定

- **每次改动或新增文件、函数、字段时，必须同步更新 `README.md`**：包括新增/删除/重命名的文件、函数签名变更、对外行为变化、新增的子流程。
- `README.md` 必须为每个内部文件单独说明其职责，并使用流程图（推荐 mermaid，可被 GitHub/VS Code 直接渲染）展示文件之间的协作顺序。
- 改动 CI/Makefile/CLI 参数时，同步更新 `README.md` 的「快速开始」「命令选项」等章节。
- PR / 提交信息里如包含文档无关的代码变更但未更新 README，视为不完整提交，需补齐后再合入。
