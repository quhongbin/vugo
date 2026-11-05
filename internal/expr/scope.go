package expr

// DotScope 表示变量绑定在当前点作用域上，即 html/template 的 `.`。
//
// `v-for="item in list"` 采用这种绑定，循环体内的 item.name 会被构建成 FieldNode `.name`。
const DotScope = ""

// frame 是作用域栈中的一层。
type frame struct {
	names map[string]string // 类 Vue 变量名 -> 模板前缀（DotScope 或 `$var`）
	vars  []string          // 本层声明的 html/template 变量（含 `$`）
}

// Scope 是类 Vue 变量名到 html/template 变量名的映射栈，同时记录当前可见的模板变量。
//
// 每一层对应一个 v-for 循环体或一个原生控制结构（if/range/with），查找时自内向外。
// 根变量 `$` 永远可见。Scope 不是并发安全的。
type Scope struct {
	frames []frame
}

// NewScope 创建一个只包含根层的作用域。
//
// 返回值:
//   - *Scope: 可立即调用 Push/Pop/Declare 的作用域；根层不会被 Pop 移除。
func NewScope() *Scope {
	return &Scope{frames: []frame{{}}}
}

// Push 进入一层新的 v-for 作用域。
//
// 参数:
//   - names: 循环变量名 -> 模板前缀。前缀以 `$` 开头时，该前缀同时登记为已声明的模板变量。
//
// 副作用: 修改作用域栈，需与 Pop 成对调用。
func (s *Scope) Push(names map[string]string) {
	f := frame{names: names}
	for _, prefix := range names {
		if prefix != DotScope {
			f.vars = append(f.vars, prefix)
		}
	}
	s.frames = append(s.frames, f)
}

// PushBlock 进入一层不绑定类 Vue 变量的作用域（原生 if/range/with 块）。
//
// 副作用: 修改作用域栈，需与 Pop 成对调用。
func (s *Scope) PushBlock() {
	s.frames = append(s.frames, frame{})
}

// Pop 退出最内层作用域；只剩根层时不做任何事。
//
// 副作用: 修改作用域栈。
func (s *Scope) Pop() {
	if len(s.frames) > 1 {
		s.frames = s.frames[:len(s.frames)-1]
	}
}

// Declare 在最内层登记一个 html/template 变量，如原生 `{{ $x := 1 }}` 声明的 `$x`。
//
// 参数:
//   - v: 变量名，含 `$` 前缀。
//
// 副作用: 修改最内层的变量表。
func (s *Scope) Declare(v string) {
	top := &s.frames[len(s.frames)-1]
	top.vars = append(top.vars, v)
}

// HasVar 判断模板变量当前是否可见。
//
// 参数:
//   - v: 变量名，含 `$` 前缀，如 `$item`。
//
// 返回值:
//   - bool: `$` 或已声明的变量返回 true。
func (s *Scope) HasVar(v string) bool {
	if v == "$" {
		return true
	}
	for i := len(s.frames) - 1; i >= 0; i-- {
		for _, d := range s.frames[i].vars {
			if d == v {
				return true
			}
		}
	}
	return false
}

// Vars 返回当前可见的全部模板变量（不含根变量 `$`），按声明顺序去重。
//
// 返回值:
//   - []string: 变量名列表，含 `$` 前缀；调用方可自由修改返回的切片。
func (s *Scope) Vars() []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range s.frames {
		for _, v := range f.vars {
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	return out
}

// Resolve 自内向外查找类 Vue 变量名。
//
// 参数:
//   - name: 类 Vue 变量名，如 `item`。
//
// 返回值:
//   - string: 模板前缀，DotScope 表示 `.`，否则为 `$var`。
//   - bool:   是否命中；未命中说明它是根数据字段。
func (s *Scope) Resolve(name string) (string, bool) {
	for i := len(s.frames) - 1; i >= 0; i-- {
		if prefix, ok := s.frames[i].names[name]; ok {
			return prefix, true
		}
	}
	return "", false
}
