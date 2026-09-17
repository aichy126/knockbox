package models

import "testing"

// 静音判定放在读侧，定时静音过期后自动恢复，不需要任何清理任务。
//
// Muted 和 MuteUntil 是两件事：前者是长期意图（我关掉了这个频道），
// 后者会自然过期（我现在忙一小时）。合成一个之后界面上就分不出来了。
func TestChannelSilent(t *testing.T) {
	const now = 1_000_000

	cases := []struct {
		name      string
		muted     int
		muteUntil int64
		want      bool
	}{
		{"都没设", 0, 0, false},
		{"长期静音", 1, 0, true},
		{"定时静音还没到点", 0, now + 60, true},
		{"定时静音正好到点", 0, now, false},
		{"定时静音已过期", 0, now - 60, false},
		{"长期静音 + 定时已过期：长期的说了算", 1, now - 60, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Channel{Muted: tc.muted, MuteUntil: tc.muteUntil}
			if got := c.Silent(now); got != tc.want {
				t.Errorf("Silent(%d) = %v，期望 %v", now, got, tc.want)
			}
		})
	}
}
