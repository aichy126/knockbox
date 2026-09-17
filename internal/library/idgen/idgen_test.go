package idgen

import (
	"sort"
	"strings"
	"testing"
	"time"
)

// ULID 的字典序必须等于时间序——分页游标和排序都建立在这一点上，
// 一旦不成立，翻历史会跳条目，而且不会有任何报错。
func TestULIDSortsByTime(t *testing.T) {
	var ids []string
	for i := 0; i < 5; i++ {
		ids = append(ids, ULID())
		time.Sleep(2 * time.Millisecond)
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	for i := range ids {
		if ids[i] != sorted[i] {
			t.Fatalf("字典序与生成顺序不一致：\n生成 %v\n排序 %v", ids, sorted)
		}
	}
}

func TestULIDShapeAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		id := ULID()
		if len(id) != 26 {
			t.Fatalf("ULID 应当是 26 字符，得到 %d：%s", len(id), id)
		}
		for _, r := range id {
			if !strings.ContainsRune(crockford, r) {
				t.Fatalf("出现了 Crockford Base32 之外的字符 %q：%s", r, id)
			}
		}
		if seen[id] {
			t.Fatalf("重复的 ULID：%s", id)
		}
		seen[id] = true
	}
}

func TestTokenCarriesPrefixAndEnoughEntropy(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		tok := Token("ch_")
		if !strings.HasPrefix(tok, "ch_") {
			t.Fatalf("前缀丢了：%s", tok)
		}
		body := strings.TrimPrefix(tok, "ch_")
		// 256 位按 5 位一组编码，至少 52 个字符
		if len(body) < 52 {
			t.Fatalf("凭据太短，熵不够：%s", tok)
		}
		if seen[tok] {
			t.Fatalf("重复的凭据：%s", tok)
		}
		seen[tok] = true
	}
	// 设备配对时会取 token[:11] 当前缀展示，短于它会直接 panic
	if len(Token("dv_")) < 11 {
		t.Fatal("凭据长度不足 11，device.auth_prefix 会越界")
	}
}

// 配对码要人手输。Crockford 排除了易混字符，输入端必须把用户打出来的
// O/I/L/U 归一回去，否则「码不对」这种错误完全无从解释。
func TestNormalizePairCodeFoldsConfusableCharacters(t *testing.T) {
	cases := []struct{ in, want string }{
		{"K7M2-9XQP", "K7M29XQP"},
		{"k7m2 9xqp", "K7M29XQP"},
		{"k7m2_9xqp", "K7M29XQP"},
		{"  K7M29XQP  ", "K7M29XQP"},
		{"OILU", "011V"},
		{"o1l0", "0110"},
	}
	for _, tc := range cases {
		if got := NormalizePairCode(tc.in); got != tc.want {
			t.Errorf("NormalizePairCode(%q) = %q，期望 %q", tc.in, got, tc.want)
		}
	}
}

func TestPairCodeShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		c := PairCode()
		if len(c) != 8 {
			t.Fatalf("配对码应当是 8 字符，得到 %d：%s", len(c), c)
		}
		for _, r := range c {
			if !strings.ContainsRune(crockford, r) {
				t.Fatalf("出现了易混字符 %q：%s", r, c)
			}
		}
		seen[c] = true
	}
	// 40 位随机，500 个里出现碰撞说明熵有问题
	if len(seen) != 500 {
		t.Fatalf("500 个配对码里只有 %d 个不同", len(seen))
	}
}

func TestFormatPairCode(t *testing.T) {
	if got := FormatPairCode("K7M29XQP"); got != "K7M2-9XQP" {
		t.Errorf("得到 %q", got)
	}
	// 长度不对时原样返回，不能把展示逻辑变成一个会崩的地方
	if got := FormatPairCode("SHORT"); got != "SHORT" {
		t.Errorf("长度不符应当原样返回，得到 %q", got)
	}
}
