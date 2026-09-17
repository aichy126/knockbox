package bytesize

import "testing"

func TestHuman(t *testing.T) {
	cases := map[int64]string{
		0:             "0 bytes",
		1:             "1 byte",
		64:            "64 bytes",
		1 << 10:       "1 KB",
		512 << 10:     "512 KB",
		1 << 20:       "1 MB",
		10 << 20:      "10 MB",
		(1 << 20) - 1: "1023 KB",
	}
	for n, want := range cases {
		if got := Human(n); got != want {
			t.Errorf("Human(%d) = %q, 想要 %q", n, got, want)
		}
	}
}

// Decimal 是从管理后台搬过来的，它显示的是「已经用了多少」。
// 这些断言锁的是搬家前后一模一样的输出——后台上那一列数字不该因为
// 换了个函数就变样。
func TestDecimal(t *testing.T) {
	cases := map[int64]string{
		0:             "0 B",
		999:           "999 B",
		1 << 10:       "1 KB",
		1536:          "2 KB", // KB 档不留小数，四舍五入
		512 << 10:     "512 KB",
		1 << 20:       "1.0 MB",
		(3 << 20) / 2: "1.5 MB",
		10 << 20:      "10.0 MB",
		1 << 30:       "1.0 GB",
		(5 << 30) / 2: "2.5 GB",
	}
	for n, want := range cases {
		if got := Decimal(n); got != want {
			t.Errorf("Decimal(%d) = %q, 想要 %q", n, got, want)
		}
	}
}
