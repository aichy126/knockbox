package service

import (
	"context"
	"fmt"
	"time"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
)

// GC 保留策略与孤儿附件回收。
//
// **不设保留期是默认值**：完整归档是这个产品的取舍之一，新设备要能拉到全部历史。
//
// 保留期有两个来源，含义不同：
//
//	retention.days         实例级。对【所有人】生效，包括被豁免配额的成员，
//	                       也包括自建模式。这是运营者对自己那块磁盘的决定。
//	public.retention_days  公共实例给陌生人的每用户上限，只在公共模式下生效，
//	                       且对 unlimited 的成员不适用。
//
// 两者都设时按更严的算；都是 0 就一条消息都不动。
type GC struct {
	d        *dao.DAO
	files    *File
	settings *Settings
	// retentionDays 来自 config.toml 的 retention.days，不在后台里改。
	retentionDays int
	every         time.Duration
}

func NewGC(d *dao.DAO, files *File, st *Settings, retentionDays int) *GC {
	return &GC{d: d, files: files, settings: st, retentionDays: retentionDays, every: time.Hour}
}

// Run 后台循环。启动时先跑一次——服务可能停了很久，回来时该补上。
func (g *GC) Run(ctx context.Context) {
	t := time.NewTicker(g.every)
	defer t.Stop()
	g.Once()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			g.Once()
		}
	}
}

// Once 跑一轮。
func (g *GC) Once() {
	// 过期会话与过期配对码。这和保留策略无关，自建模式也要清：
	// 不清的话，接入页每被打开一次就在 pair_code 里留一行，永远不减。
	if err := NewSession(g.d).GC(); err != nil {
		log.Warn("cleaning up expired sessions and pairing codes failed", log.Any("error", err.Error()))
	}

	if n := g.pruneMessages(); n > 0 {
		log.Info("retention policy applied", log.Any("deleted", n))
	}
	g.sweepFiles()
}

// retentionFor 这个成员的消息该留多少天。0 = 不限。
func (g *GC) retentionFor(unlimited int, publicDays int) int {
	days := g.retentionDays
	// 公共实例的每用户上限只约束没被豁免的成员；与实例级取更严的那个。
	if publicDays > 0 && unlimited == 0 && (days <= 0 || publicDays < days) {
		days = publicDays
	}
	return days
}

// pruneMessages 按保留策略物理删除过期消息，返回删除条数。
func (g *GC) pruneMessages() int {
	publicDays := 0
	if g.settings.PublicEnabled() {
		publicDays = g.settings.RetentionDays()
	}
	if g.retentionDays <= 0 && publicDays <= 0 {
		return 0
	}

	// 逐个用户处理。一条 SQL 也能写完，但那样就没法在日志里说清「谁被清了多少」——
	// 数据被删掉这件事必须留下可追溯的痕迹；而且各人的保留期本来就可能不同。
	rows, err := g.d.Engine().QueryString("SELECT id, unlimited FROM user")
	if err != nil {
		log.Warn("GC: listing users failed", log.Any("error", err.Error()))
		return 0
	}
	total := 0
	var maxRev int64
	now := time.Now()
	for _, r := range rows {
		var uid int64
		var unlimited int
		_, _ = fmt.Sscan(r["id"], &uid)
		_, _ = fmt.Sscan(r["unlimited"], &unlimited)
		days := g.retentionFor(unlimited, publicDays)
		if days <= 0 {
			continue
		}
		n, rev := g.purgeUser(uid, now.AddDate(0, 0, -days).Unix())
		total += n
		if rev > maxRev {
			maxRev = rev
		}
	}
	if total == 0 {
		return 0
	}

	// 下面三件事【每轮只做一次】，不能放进上面的循环。
	// 反连接是全表扫描，而 checkpoint + vacuum 是整库操作：
	// 放在循环里的话，成本会随成员数线性放大，而效果和做一次完全一样。
	if _, err := g.d.Engine().Exec(
		"DELETE FROM push_log WHERE message_id NOT IN (SELECT id FROM message)"); err != nil {
		log.Warn("cleaning up orphaned delivery records failed", log.Any("error", err.Error()))
	}
	// gc_watermark：**这一步不能省**。
	// 硬删之后那些行不存在了，离线很久的设备永远学不到它们没了；
	// 同步时发现 since < watermark 就让客户端整体重载。
	// 少了它，那台设备会永久缺一块数据而且毫无察觉。
	if maxRev > 0 {
		_ = g.d.KVSet(models.KVGCWatermark, fmt.Sprint(maxRev), now.Unix())
	}
	NewSync(g.d).scrub()
	return total
}

// purgeUser 删掉一个用户超期的消息，返回删除条数与被删行里最大的 rev。
func (g *GC) purgeUser(uid, cutoff int64) (int, int64) {
	var maxRev int64
	_, _ = g.d.Engine().SQL(
		"SELECT COALESCE(MAX(rev),0) FROM message WHERE user_id=? AND created_at<?",
		uid, cutoff).Get(&maxRev)
	if maxRev == 0 {
		return 0, 0
	}
	// 附件引用先减，再删消息：顺序反了就找不到该减哪一个了。
	// 与删除、清空走同一条路，都认 message.file_id 这一个来源；
	// 各自去解析 extra 里的 JSON 会让三条路依赖三个字段，任一写入端漏填即静默失效。
	_, _ = g.d.Engine().Exec(`
		UPDATE file SET ref_count = MAX(ref_count - 1, 0) WHERE id IN (
			SELECT file_id FROM message
			WHERE user_id=? AND created_at<? AND file_id <> 0)`, uid, cutoff)

	res, err := g.d.Engine().Exec(
		"DELETE FROM message WHERE user_id=? AND created_at<?", uid, cutoff)
	if err != nil {
		log.Warn("GC: deleting messages failed", log.Any("user", uid), log.Any("error", err.Error()))
		return 0, 0
	}
	n, _ := res.RowsAffected()
	// 投递记录、水位线和抹除由 pruneMessages 在所有成员处理完之后统一做一次。
	return int(n), maxRev
}

// sweepFiles 回收没人引用的附件。
// **必须靠引用计数判断**：sha256 内容寻址意味着同一张图可能被多条消息引用，
// 看到就删会误伤。
func (g *GC) sweepFiles() {
	if g.files == nil {
		return
	}
	if n, err := g.files.ReleaseUnused(); err == nil && n > 0 {
		log.Info("reclaimed orphaned attachments", log.Any("count", n))
	}
}
