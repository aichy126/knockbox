package api

import "testing"

// 频道的 meta 是 app 写的不透明 JSON，只有管理界面为了显示名字才去解析它。
//
// 原来是按字符串找 `"name"` 再截两个引号之间的内容，几种常见写法都会取错：
// 别的字段的值里出现这个词、名字本身带转义引号、或者 name 排在后面。
func TestChannelNameParsesMetaAsJSON(t *testing.T) {
	cases := []struct {
		name string
		meta string
		id   string
		want string
	}{
		{"常规", `{"name":"构建通知","icon":"hammer"}`, "01HQ8ZK9", "构建通知"},
		{"name 不在最前", `{"icon":"bell","color":"#f00","name":"告警"}`, "01HQ8ZK9", "告警"},
		{"别的字段的值里有 name", `{"desc":"put the \"name\" here","name":"真名"}`, "01HQ8ZK9", "真名"},
		{"名字里带转义引号", `{"name":"他说\"好\""}`, "01HQ8ZK9", `他说"好"`},
		{"没有 name", `{"icon":"bell"}`, "01HQ8ZK9AB", "01HQ8Z"},
		{"name 是空串", `{"name":"   "}`, "01HQ8ZK9AB", "01HQ8Z"},
		{"meta 为空", "", "01HQ8ZK9AB", "01HQ8Z"},
		{"meta 不是合法 JSON", `name=告警`, "01HQ8ZK9AB", "01HQ8Z"},
		{"短 id 原样返回", "", "abc", "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := channelName(tc.meta, tc.id); got != tc.want {
				t.Errorf("channelName(%q, %q) = %q，期望 %q", tc.meta, tc.id, got, tc.want)
			}
		})
	}
}
