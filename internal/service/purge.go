package service

import (
	"time"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

type PurgeResult struct {
	ChannelID   string `json:"channel"`
	BeforeMsgID int64  `json:"before_msg_id"`
	Deleted     int64  `json:"deleted"`
	Rev         int64  `json:"rev"`
}

// PurgeChannel 清空一个频道的消息，物理删除。
//
// 两件事必须一起做，缺一个这功能就是假的：
//
//  1. 别的设备要学得到。硬删之后那些行不存在了，墓碑机制失效，
//     所以写一条频道级的 purge_log，其它设备照着在本地硬删。
//     一条记录顶掉 N 个墓碑，反而比墓碑更省。
//
//  2. 真的抹掉。SQLite 的 DELETE 只把页标记为空闲，内容仍然留在文件里能被读出来。
//     要真抹掉需要 secure_delete（DSN 上开的，启动时断言过）+ 删除 +
//     wal_checkpoint（WAL 里还留着旧页镜像）+ vacuum（回收并重写页）。
//
// 删的是「id <= 快照值」而不是「全部」：清空过程中恰好到达的新消息不会被静默吃掉。
func (s *Sync) PurgeChannel(userID int64, channelID string, beforeID int64) (*PurgeResult, error) {
	ch, err := NewChannel(s.d).Get(userID, channelID)
	if err != nil {
		return nil, err
	}

	out := &PurgeResult{ChannelID: ch.Id}
	err = s.d.Tx(func(sess *xorm.Session) error {
		// 不指定就取当前最大 id 做快照。
		if beforeID <= 0 {
			rows, err := sess.QueryString(
				"SELECT COALESCE(MAX(id), 0) AS n FROM message WHERE channel_id = ?", ch.Id)
			if err != nil {
				return err
			}
			for _, v := range rows[0] {
				_, _ = fmtSscan(v, &beforeID)
			}
		}
		out.BeforeMsgID = beforeID
		if beforeID == 0 {
			return nil
		}

		// 附件引用先减，否则行删掉就找不到该减谁了。
		if _, err := sess.Exec(`UPDATE file SET ref_count = MAX(ref_count - 1, 0)
			WHERE id IN (SELECT file_id FROM message WHERE channel_id = ? AND id <= ? AND file_id <> 0)`,
			ch.Id, beforeID); err != nil {
			return err
		}
		// 推送记录一并删掉：「清空」就该是这个频道的痕迹没了。
		if _, err := sess.Exec(`DELETE FROM push_log WHERE message_id IN
			(SELECT id FROM message WHERE channel_id = ? AND id <= ?)`, ch.Id, beforeID); err != nil {
			return err
		}
		res, err := sess.Exec("DELETE FROM message WHERE channel_id = ? AND id <= ?", ch.Id, beforeID)
		if err != nil {
			return err
		}
		out.Deleted, _ = res.RowsAffected()

		rev, err := dao.NextRev(sess)
		if err != nil {
			return err
		}
		out.Rev = rev
		now := time.Now().Unix()
		if _, err := sess.Insert(&models.PurgeLog{
			UserId: userID, ChannelId: ch.Id, BeforeMsgId: beforeID,
			DeletedCount: out.Deleted, Rev: rev, Ctime: now,
		}); err != nil {
			return err
		}
		rows, err := sess.QueryString(
			"SELECT COUNT(*) AS n FROM message WHERE channel_id = ? AND deleted_at = 0", ch.Id)
		if err != nil {
			return err
		}
		var left int64
		for _, v := range rows[0] {
			_, _ = fmtSscan(v, &left)
		}
		_, err = sess.Exec("UPDATE channel SET msg_count = ?, updated_at = ? WHERE id = ?", left, now, ch.Id)
		return err
	})
	if err != nil {
		return nil, err
	}

	if out.Deleted > 0 {
		s.scrub()
	}
	return out, nil
}

// scrub 把删掉的内容真正从文件里抹掉。
//
// 缺了 wal_checkpoint 这一步等于没删：WAL 文件里还留着旧页的镜像，
// 直接 grep 数据库目录仍然能把「删掉」的正文搜出来。
func (s *Sync) scrub() {
	for _, stmt := range []string{
		"PRAGMA wal_checkpoint(TRUNCATE)",
		"PRAGMA incremental_vacuum(2000)",
	} {
		if _, err := s.d.Engine().Exec(stmt); err != nil {
			log.Warn("vacuuming failed: deleted content may still sit in the database file",
				log.Any("stmt", stmt), log.Any("error", err.Error()))
		}
	}
}
