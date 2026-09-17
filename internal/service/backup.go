package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/knockbox/internal/dao"
)

// Backup 自动备份。
//
// 用 `VACUUM INTO`：它**一句话拿到一致性快照，而且不锁写**——
// 直接 cp 数据库文件是错的，WAL 里的内容不在那个文件里，拷出来的可能是半截状态。
type Backup struct {
	d     *dao.DAO
	dir   string
	keep  int
	every time.Duration
}

func NewBackup(d *dao.DAO, dir string, keep int, everyHours int) *Backup {
	if keep <= 0 {
		keep = 7
	}
	if everyHours <= 0 {
		everyHours = 24
	}
	return &Backup{d: d, dir: dir, keep: keep, every: time.Duration(everyHours) * time.Hour}
}

func (b *Backup) Run(ctx context.Context) {
	t := time.NewTicker(b.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := b.Once(); err != nil {
				log.Warn("backup failed", log.Any("error", err.Error()))
			}
		}
	}
}

// Once 做一份备份并按保留数清理旧的。
func (b *Backup) Once() error {
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return err
	}
	// VACUUM INTO 的目标文件必须【不存在】，否则直接报错。
	// 正常节奏下一天一份不会撞，但手动触发或者改小间隔就会——
	// 与其让它报错，不如让名字自己躲开。
	base := filepath.Join(b.dir, "knockbox-"+time.Now().Format("20060102-150405"))
	name := base + ".db"
	for i := 1; ; i++ {
		if _, err := os.Stat(name); os.IsNotExist(err) {
			break
		}
		if i > 50 {
			return fmt.Errorf("backup file name keeps colliding: %s", name)
		}
		name = fmt.Sprintf("%s-%d.db", base, i)
	}
	if _, err := b.d.Engine().Exec("VACUUM INTO ?", name); err != nil {
		return err
	}
	info, err := os.Stat(name)
	if err != nil {
		return err
	}
	log.Info("backup done", log.Any("file", filepath.Base(name)), log.Any("bytes", info.Size()))
	return b.prune()
}

// prune 只留最近 keep 份。
// 按文件名排序而不是按修改时间：名字里带的是【备份那一刻】的时间，
// 而 mtime 会被复制、同步、rsync 之类的操作改掉。
func (b *Backup) prune() error {
	ents, err := os.ReadDir(b.dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "knockbox-") && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	if len(names) <= b.keep {
		return nil
	}
	sort.Strings(names)
	for _, n := range names[:len(names)-b.keep] {
		if err := os.Remove(filepath.Join(b.dir, n)); err != nil {
			log.Warn("deleting an old backup failed", log.Any("file", n), log.Any("error", err.Error()))
		}
	}
	return nil
}
