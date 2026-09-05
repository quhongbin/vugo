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

---
需要我帮你起草一份README.md的初稿吗？可以把项目定位、使用示例和转译对照都放进去，方便你直接开始开发。
