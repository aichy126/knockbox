package api

import (
	"strconv"

	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) sync(c *gin.Context) {
	since, _ := strconv.ParseInt(c.Query("since"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))
	out, err := service.NewSync(s.DAO).Since(middleware.UserID(c), since, limit)
	if err != nil {
		res.Rfail(c, err.Error())
		return
	}
	s.fillFileURLs(out.Messages)
	// 顺手记下这台设备同步到哪了：GC 要靠它判断墓碑什么时候能回收。
	if dev := middleware.Device(c); dev != nil {
		_, _ = s.DAO.Engine().Exec("UPDATE device SET sync_rev = ?, last_seen_at = strftime('%s','now') WHERE id = ?",
			out.Rev, dev.Id)
	}
	res.Rsucc(c, out)
}

type readBody struct {
	UpToID int64    `json:"up_to_id"`
	UIDs   []string `json:"uids"`
	All    bool     `json:"all"`
}

func (s *Server) markRead(c *gin.Context) {
	var b readBody
	if err := c.ShouldBindJSON(&b); err != nil {
		res.Rfail(c, "解析请求失败: "+err.Error())
		return
	}
	sync := service.NewSync(s.DAO)
	upTo := b.UpToID
	if b.All {
		// 全部已读走水位线，一次 UPDATE，不推高任何消息的 rev。
		rows, err := s.DAO.Engine().QueryString(
			"SELECT COALESCE(MAX(id), 0) AS n FROM message WHERE user_id = ?", middleware.UserID(c))
		if err != nil {
			res.Rfail(c, err.Error())
			return
		}
		for _, v := range rows[0] {
			upTo, _ = strconv.ParseInt(v, 10, 64)
		}
	}
	if err := sync.MarkRead(middleware.UserID(c), upTo, b.UIDs); err != nil {
		res.Rfail(c, err.Error())
		return
	}
	badge, _ := sync.Badge(middleware.UserID(c))
	res.Rsucc(c, gin.H{"badge": badge})
}

func (s *Server) deleteMessages(c *gin.Context) {
	var b struct {
		UIDs []string `json:"uids"`
	}
	if err := c.ShouldBindJSON(&b); err != nil {
		res.Rfail(c, "解析请求失败: "+err.Error())
		return
	}
	n, err := service.NewSync(s.DAO).Delete(middleware.UserID(c), b.UIDs)
	if err != nil {
		res.Rfail(c, err.Error())
		return
	}
	res.Rsucc(c, gin.H{"deleted": n})
}

func (s *Server) purgeChannel(c *gin.Context) {
	var b struct {
		Before int64 `json:"before"`
	}
	_ = c.ShouldBindJSON(&b)
	out, err := service.NewSync(s.DAO).PurgeChannel(middleware.UserID(c), c.Param("id"), b.Before)
	if err != nil {
		res.Rfail(c, err.Error())
		return
	}
	res.Rsucc(c, out)
}

// messages 频道内倒序分页，给「往回翻历史」用。
// 增量同步走 /sync，这里是浏览。
func (s *Server) messages(c *gin.Context) {
	before, _ := strconv.ParseInt(c.Query("before"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.DAO.Engine().Where("user_id = ? AND deleted_at = 0", middleware.UserID(c))
	if ch := c.Query("channel"); ch != "" {
		q = q.And("channel_id = ?", ch)
	}
	// 按 uid 取单条：通知扩展要补正文时只要这一条。
	// 没有它的话扩展只能拉一整页回来在本地筛——把二十条完整正文下载进一个
	// 只有 24MB 预算的进程，而且目标不在这一页时还会失败。
	if uid := c.Query("uid"); uid != "" {
		q = q.And("uid = ?", uid)
	}
	if before > 0 {
		q = q.And("id < ?", before)
	}
	var out []service.MessageView
	rows, err := q.OrderBy("id DESC").Limit(limit).QueryString()
	if err != nil {
		res.Rfail(c, err.Error())
		return
	}
	for _, r := range rows {
		id, _ := strconv.ParseInt(r["id"], 10, 64)
		rev, _ := strconv.ParseInt(r["rev"], 10, 64)
		ct, _ := strconv.ParseInt(r["created_at"], 10, 64)
		readAt, _ := strconv.ParseInt(r["read_at"], 10, 64)
		repliedAt, _ := strconv.ParseInt(r["replied_at"], 10, 64)
		replyUntil, _ := strconv.ParseInt(r["reply_until"], 10, 64)
		out = append(out, service.MessageView{
			UID: r["uid"], Rev: rev, ID: id, ChannelID: r["channel_id"], Type: r["type"],
			Title: r["title"], Summary: r["summary"], Body: r["body"], Extra: r["extra"],
			CreatedAt: ct, Read: readAt != 0,
			// reply_webhook 只用来判断「能不能回」，它本身不下发 ——
			// 那是发送方的内部端点，配对过的设备没有理由看到。
			Replyable: r["reply_webhook"] != "", ReplyUntil: replyUntil,
			Reply: r["reply"], RepliedAt: repliedAt,
		})
	}
	if out == nil {
		out = []service.MessageView{}
	}
	s.fillFileURLs(out)
	res.Rsucc(c, gin.H{"list": out, "count": len(out), "has_more": len(out) == limit})
}
