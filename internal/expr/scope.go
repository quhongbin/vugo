package expr

// DotScope 表示变量绑定在当前点作用域上，即 html/template 的 `.`。
//
// `v-for="item in list"` 采用这种绑定，循环体内的 item.name 会被渲染成 .name。
const DotScope = ""

// Scope 是类 Vue 变量名到 html/template 变量名的映射栈。
// 每一层对应一个 v-for 循环体，查找时自内向外。
type Scope struct {
	frames []map[string]string
}

// NewScope 创建一个空作用域。
func NewScope() *Scope {
	return &Scope{}
}

// Push 进入一层新的作用域（进入 v-for 循环体时调用）。
func (s *Scope) Push(frame map[string]string) {
	s.frames = append(s.frames, frame)
}

// Pop 退出最内层作用域（离开 v-for 循环体时调用）。
func (s *Scope) Pop() {
	if len(s.frames) > 0 {
		s.frames = s.frames[:len(s.frames)-1]
	}
}

// Resolve 自内向外查找变量名，返回其模板前缀与是否命中。
func (s *Scope) Resolve(name string) (string, bool) {
	for i := len(s.frames) - 1; i >= 0; i-- {
		if prefix, ok := s.frames[i][name]; ok {
			return prefix, true
		}
	}
	return "", false
}
