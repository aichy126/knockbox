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
