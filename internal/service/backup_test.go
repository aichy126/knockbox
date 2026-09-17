package service

import (
	"os"
	"testing"
)

// 备份必须是【可用的】数据库，不是一堆字节。测的就是这一点。
func TestBackupProducesUsableSnapshot(t *testing.T) {
	d, dir := newTestDAO(t)
	uid := mustUser(t, d)
	_ = mustChannel(t, d, uid)

	b := NewBackup(d, dir+"/bk", 2, 24)
	for i := 0; i < 3; i++ {
		if err := b.Once(); err != nil {
			t.Fatal(err)
		}
	}
	// keep=2：三次之后只该剩两份
	ents, _ := os.ReadDir(dir + "/bk")
	if len(ents) != 2 {
		t.Fatalf("保留数不对，剩 %d 份", len(ents))
	}
}
