package lexer

import "testing"

func TestRun(t *testing.T) {
	src := "<h1>{{ title }}</h1><br />\n"
	tokens, err := New(src).Run()
	if err != nil {
		t.Fatal(err)
	}

	want := []Token{
		{Type: OpenTag, Name: "h1"},
		{Type: Mustache, Raw: "title"},
		{Type: CloseTag, Name: "h1"},
		{Type: SelfCloseTag, Name: "br"},
		{Type: Text, Raw: "\n"},
		{Type: EOF},
	}
	if len(tokens) != len(want) {
		t.Fatalf("token 数量 = %d, 期望 %d: %+v", len(tokens), len(want), tokens)
	}
	for i, tk := range tokens {
		if tk.Type != want[i].Type || tk.Name != want[i].Name || tk.Raw != want[i].Raw {
			t.Errorf("token[%d] = %+v, 期望 %+v", i, tk, want[i])
		}
	}
}

func TestAttrValueKeepsGtAndBraces(t *testing.T) {
	// 属性值中的 > 与 {{ 不应干扰标签扫描
	tokens, err := New(`<div title="a > b">{{ msg }}</div>`).Run()
	if err != nil {
		t.Fatal(err)
	}
	var kinds []TokenType
	var raws []string
	for _, tk := range tokens {
		kinds = append(kinds, tk.Type)
		raws = append(raws, tk.Raw)
	}
	wantKinds := []TokenType{OpenTag, Mustache, CloseTag, EOF}
	for i, k := range wantKinds {
		if kinds[i] != k {
			t.Errorf("token[%d].Type = %v, 期望 %v", i, kinds[i], k)
		}
	}
	if raws[1] != "msg" {
		t.Errorf("mustache 内容 = %q, 期望 %q", raws[1], "msg")
	}
}

func TestUnclosedMustache(t *testing.T) {
	if _, err := New("{{ msg").Run(); err == nil {
		t.Error("未闭合的 {{ 应当报错")
	}
}

func TestDoctypePassthrough(t *testing.T) {
	tokens, err := New("<!DOCTYPE html>\n<p>{{ msg }}</p>").Run()
	if err != nil {
		t.Fatal(err)
	}
	if tokens[0].Type != Text || tokens[0].Raw != "<!DOCTYPE html>" {
		t.Errorf("DOCTYPE 应按文本透传, 实际 = %+v", tokens[0])
	}
}

func TestLessThanInText(t *testing.T) {
	tokens, err := New("a < b {{ x }}").Run()
	if err != nil {
		t.Fatal(err)
	}
	if tokens[0].Type != Text || tokens[0].Raw != "a < b " {
		t.Errorf("文本中的 < 不应被当作标签, 实际 = %+v", tokens[0])
	}
}
