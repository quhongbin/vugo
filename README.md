---

# 🦫 vugo

**让 Go 模板像 Vue 一样丝滑。**

`vugo` 是一个轻量级模板预处理器，将类 Vue 的声明式语法转译为 Go 标准库 `html/template` 语法。

>  **核心目标**：提升模板可读性，同时**零运行时依赖**、**完全兼容现有生态**、**保留原生 XSS 防护**。

---

##  为什么选择 vugo？

| 特性 | 说明 |
| :--- | :--- |
| ** 高可读性** | 告别 `{{ range $i, $v := .List }}`，拥抱 `v-for` 和 `{{ item.name }}` |
| ** 原生安全** | 转译后仍走 `html/template`，自动上下文转义，无需担心 XSS |
| ** 生态兼容** | 输出纯 `.tmpl` 文件，无缝接入 Gin/Echo/标准库，无需改构建流程 |
| ** 零运行时** | 仅在构建/开发阶段转译，生产环境无额外性能开销 |
| **️ 渐进式迁移** | 支持在现有项目中混用，老模板无需一次性重写 |

---

##  快速开始

### 1. 安装

```bash
go install github.com/quhongbin/vugo/cmd/vugo@latest
```

### 2. 编写模板

创建 `index.vugo`：

```html
<div class="user-list">
  <!-- 类 Vue 语法 -->
  <div v-if="isLoggedIn">
    <h2>Welcome, {{ user.name }}!</h2>
    <ul>
      <li v-for="item in todos" :key="item.id">
        <span :class="{ done: item.done }">{{ item.text }}</span>
      </li>
    </ul>
  </div>
  <div v-else>
    <a :href="loginUrl">Please Login</a>
  </div>
</div>
```

### 3. 转译

```bash
vugo -src ./templates -dst ./dist/templates
```

### 4. 查看结果

生成的 `dist/templates/index.tmpl`：

```html
<div class="user-list">
  {{ if .isLoggedIn }}
    <h2>Welcome, {{ .user.name }}!</h2>
    <ul>
      {{ range .todos }}
        <li>
          <span class="{{ if .done }}done{{ end }}">{{ .text }}</span>
        </li>
      {{ end }}
    </ul>
  {{ else }}
    <a href="{{ .loginUrl }}">Please Login</a>
  {{ end }}
</div>
```

---

##  语法对照表

| Vue-like (`vugo`) | Go Standard (`html/template`) | 备注 |
| :--- | :--- | :--- |
| `{{ msg }}` | `{{ .msg }}` | 自动补全根数据点 `.` |
| `<div v-if="show">` | `{{ if .show }}<div>...{{ end }}` | 支持 `v-else-if` / `v-else` |
| `<li v-for="item in list">` | `{{ range .list }}<li>...{{ end }}` | 自动处理作用域 |
| `<a :href="url">` | `<a href="{{ .url }}">` | 属性绑定自动转义 |
| `<div :class="{ active: isActive }">` | `<div class="{{ if .isActive }}active{{ end }}">` | 对象语法支持 |
| `<MyComp :title="t" />` | `{{ template "MyComp" . }}` | 需配合 `{{ define }}` |

---

## ️ 已知限制

为了保持轻量和兼容性，以下 Vue 特性**不支持**：

1.  **`v-model`**：服务端无双向绑定，请拆分为 `:value` + `@input` 事件处理。
2.  **`<slot>`**：请使用 `{{ define "SlotName" }}` + `{{ template "SlotName" . }}` 组合。
3.  **计算属性**：请在 Go 后端计算好数据后传入模板。

---

##  贡献

欢迎提交 Issue 或 PR！

- 发现转译 Bug？请提供 `.vugo` 源码和期望输出。
- 有新语法建议？请先开 Issue 讨论。

---

##  License

[MIT License](./LICENSE)

---

###  下一步建议

1.  **占位**：尽快去 GitHub 创建 `vugo` 仓库（如果名字被占，考虑 `vugo-tmpl`）。
2.  **License**：在根目录创建 `LICENSE` 文件，粘贴 MIT 文本。
3.  **核心逻辑**：先实现 `{{ msg }}` -> `{{ .msg }}` 和 `v-if` 这两个最基础的转换，跑通一个 E2E 测试。

需要我帮你把 **MIT License** 的文本也生成出来，或者写一个 **Go 测试用例** 来验证第一个转译逻辑吗？


