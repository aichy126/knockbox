package web

import (
	"encoding/json"
	"strings"
	"testing"
)

// 语料是从 locales/ 读进来的，读不到或读坏了会在 init 里 panic，
// 所以这里能跑起来本身就说明加载成功了。这些断言管的是加载之后的行为。

// 英文是所有语言的兜底，缺一个 key 就意味着页面上会出现一处空白。
// 每一门语言的 key 集合都必须是英文的子集，多出来的 key 一定是拼错或改名没跟上。
func TestLocalesCoverEnglish(t *testing.T) {
	enKeys := localeKeys(t, "locales/en.json")
	if len(enKeys) < 50 {
		t.Fatalf("英文语料只有 %d 个 key，不像是完整的", len(enKeys))
	}
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") || e.Name() == "en.json" {
			continue
		}
		for k := range localeKeys(t, "locales/"+e.Name()) {
			if !enKeys[k] {
				t.Errorf("%s 里的 %q 在 en.json 里没有——拼错了，还是改名之后没跟上？", e.Name(), k)
			}
		}
	}
}

func localeKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	b, err := localeFS.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s 不是合法 JSON: %v", path, err)
	}
	keys := make(map[string]bool, len(m))
	for k := range m {
		keys[k] = true
	}
	return keys
}

// 没翻的 key 必须回落成英文，而不是留一片空白：
// 这样一门只翻了一半的语言也能先合进来。
func TestUntranslatedFallsBackToEnglish(t *testing.T) {
	en, zh := T(LangEN), T(LangZH)
	if en.JoinTitle == "" {
		t.Fatal("英文的 join_title 是空的")
	}
	if zh.JoinTitle == "" {
		t.Error("中文的 join_title 是空的——回落没生效")
	}
	// 未知语言整份回落英文
	if T(Lang("xx")).JoinTitle != en.JoinTitle {
		t.Error("未知语言应当整份回落英文")
	}
}

// 语言选择：查询参数优先，其次 Accept-Language，最后英文。
// 地区变体要能落到主语言上——否则 zh-CN 的浏览器会拿到英文。
func TestPickLang(t *testing.T) {
	cases := []struct {
		name, query, accept string
		want                Lang
	}{
		{"什么都没有就是英文", "", "", LangEN},
		{"查询参数说了算", "zh", "en-US,en;q=0.9", LangZH},
		{"查询参数优先于 Accept-Language", "en", "zh-CN,zh;q=0.9", LangEN},
		{"地区变体落到主语言", "zh-CN", "", LangZH},
		{"大小写不敏感", "ZH-Hans", "", LangZH},
		{"Accept-Language 按顺序取第一个认识的", "", "zh-CN,zh;q=0.9,en;q=0.8", LangZH},
		{"不认识的排在前面就跳过", "", "fr-FR,fr;q=0.9,zh;q=0.8", LangZH},
		{"全都不认识就英文", "", "fr-FR,de;q=0.9", LangEN},
		{"拼错的 lang 不该把页面打没", "zzz", "", LangEN},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PickLang(c.query, c.accept); got != c.want {
				t.Errorf("PickLang(%q, %q) = %q，想要 %q", c.query, c.accept, got, c.want)
			}
		})
	}
}

// <html lang> 用的是语料里声明的那个，不是文件名：中文要分简繁。
func TestLangAttr(t *testing.T) {
	if got := LangZH.Attr(); got != "zh-CN" {
		t.Errorf("中文的 html lang 应当是 zh-CN，得到 %q", got)
	}
	if got := LangEN.Attr(); got != "en" {
		t.Errorf("英文的 html lang 应当是 en，得到 %q", got)
	}
}

// 语言开关列出除当前之外的每一种语言，且不会把自己列进去。
func TestLangSwitch(t *testing.T) {
	out := LangSwitch(LangEN)
	if !strings.Contains(out, `href="?lang=zh"`) {
		t.Errorf("英文页的开关里应当有中文：%s", out)
	}
	if strings.Contains(out, `href="?lang=en"`) {
		t.Errorf("开关不该列出当前语言：%s", out)
	}
	if got := len(Langs()); got != 2 {
		t.Errorf("现在应当注册了 2 门语言，得到 %d", got)
	}
}
