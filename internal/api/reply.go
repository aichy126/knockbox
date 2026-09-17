package api

import (
	"errors"
	"net/http"

	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

// reply 用户回一条可回复的消息。
//
// 走 DeviceAuth，不是 SendAuth。**三套鉴权的边界在这里没有松动**：
// 回复是「读这条消息、往它上面写一个答案」，是本人对自己收到的消息的操作，
// 正好落在 DeviceAuth 的范围里。频道 token 仍然只写、仍然读不到任何东西 ——
// 它连自己发出去的那条消息被回了什么都读不到，答案是【推】给它的回调地址的。
func (s *Server) reply(c *gin.Context) {
	var b struct {
		Reply string `json:"reply"`
	}
	if err := c.ShouldBindJSON(&b); err != nil {
		res.Rfail(c, "解析请求失败: "+err.Error())
		return
	}
	out, err := service.NewReply(s.DAO).Submit(middleware.UserID(c), c.Param("uid"), b.Reply)
	if err != nil {
		// 已经回过了不是错误，是一个客户端必须知道结果的状态：
		// 另一台设备回的，这台要把界面切成「已回复 · X」而不是弹一句失败。
		// 409 而不是 200，是为了让客户端的错误分支能认出它；
		// 答案照样带在 body 里。
		var dup *service.ErrAlreadyReplied
		if errors.As(err, &dup) {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"code": 1, "msg": dup.Error(),
				"data": gin.H{"reply": dup.Reply, "replied_at": dup.At},
			})
			return
		}
		res.Rfail(c, err.Error())
		return
	}
	// 回复已经落库，回调也已经和它在同一个事务里排好队。
	// 叫醒 worker 只是让它别等下一轮轮询 —— 漏叫一次也不会丢，兜底扫描会捡起来。
	if s.NotifyHook != nil {
		s.NotifyHook()
	}
	res.Rsucc(c, out)
}
