package api

import (
	"errors"
	"net/http"

	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

// channelView 给 app 的频道形状。
// curl_example 由服务端拼好：客户端不该自己拼 URL，拼错了用户才发现。
type channelView struct {
	ID          string `json:"id"`
	Token       string `json:"token"`
	Muted       bool   `json:"muted"`
	MuteUntil   int64  `json:"mute_until"`
	Sound       string `json:"sound"`
	Level       string `json:"level"`
	Meta        string `json:"meta"`
	MsgCount    int64  `json:"msg_count"`
	LastMsgID   int64  `json:"last_msg_id"`
	LastMsgAt   int64  `json:"last_msg_at"`
	CurlExample string `json:"curl_example"`
}

func (s *Server) view(ch *models.Channel) channelView {
	return channelView{
		ID: ch.Id, Token: ch.Token, Muted: ch.Muted != 0, MuteUntil: ch.MuteUntil, Sound: ch.Sound,
		Level: ch.Level, Meta: ch.Meta, MsgCount: ch.MsgCount,
		LastMsgID: ch.LastMsgId, LastMsgAt: ch.LastMsgAt,
		CurlExample: `curl -d "your message" ` + s.ExternalURL + "/api/v1/send/" + ch.Token,
	}
}

func (s *Server) listChannels(c *gin.Context) {
	out, err := service.NewChannel(s.DAO).List(middleware.UserID(c))
	if err != nil {
		s.fail(c, err)
		return
	}
	rows := make([]channelView, 0, len(out))
	for i := range out {
		rows = append(rows, s.view(&out[i]))
	}
	res.Rsucc(c, gin.H{"list": rows, "count": len(rows)})
}

func (s *Server) createChannel(c *gin.Context) {
	var in service.ChannelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		res.Rfail(c, "cannot parse the request: "+err.Error())
		return
	}
	if err := s.quota().CheckChannel(middleware.UserID(c)); err != nil {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 1, "msg": err.Error()})
		return
	}
	ch, err := service.NewChannel(s.DAO).Create(middleware.UserID(c), in)
	if err != nil {
		s.fail(c, err)
		return
	}
	res.Rsucc(c, s.view(ch))
}

func (s *Server) updateChannel(c *gin.Context) {
	var in service.ChannelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		res.Rfail(c, "cannot parse the request: "+err.Error())
		return
	}
	ch, err := service.NewChannel(s.DAO).Update(middleware.UserID(c), c.Param("id"), in)
	if err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			s.fail(c, service.ErrChannelNotFound)
			return
		}
		s.fail(c, err)
		return
	}
	res.Rsucc(c, s.view(ch))
}

// deleteChannel 删掉一个频道。
//
// `?purge=1` 连同它的消息一起物理删除。不带这个参数时只删频道本身，
// 消息留在库里——用于「这个频道我不用了，但记录还要留着」。
func (s *Server) deleteChannel(c *gin.Context) {
	uid := middleware.UserID(c)
	id := c.Param("id")
	if c.Query("purge") == "1" {
		if _, err := service.NewSync(s.DAO).PurgeChannel(uid, id, 0); err != nil {
			s.fail(c, err)
			return
		}
	}
	if err := service.NewChannel(s.DAO).Delete(uid, id); err != nil {
		s.fail(c, err)
		return
	}
	res.Rsucc(c, gin.H{"id": id})
}

func (s *Server) rotateChannelToken(c *gin.Context) {
	ch, err := service.NewChannel(s.DAO).RotateToken(middleware.UserID(c), c.Param("id"))
	if err != nil {
		s.fail(c, err)
		return
	}
	res.Rsucc(c, s.view(ch))
}
