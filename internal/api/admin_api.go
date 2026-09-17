package api

import (
	"strconv"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/uierr"
	"github.com/gin-gonic/gin"
)

// 后台自用的 JSON 接口。
//
// 【这不是公开契约】。/api/v1 是对外承诺的那一套（发送、以及 iOS 客户端用的
// 那些）；/admin/api 只服务这个仓库自带的后台界面，会随版本改动，不保证兼容。
// README 里写明了这条边界。
//
// 信封沿用 /api/v1 的：成功 {"code":0,"msg":"success","data":…}，
// 失败走 s.fail → {"code":1,"msg":"<按 Accept-Language 渲染>","data":{"error_code":…}}。
// 失败句子因此不用多写一行就有中文——s.fail 复用的是同一个 reqLang。
//
// HTTP 状态码一律 200，【只有鉴权失败是 401】：前端需要一个便宜的判断来决定
// 什么时候弹回登录页，而那也正是 AdminAuth 现在的行为。

// List 所有列表端点的信封。
//
// 不用 res.Rlist：它的形状是 data:{total,items}，塞不下 has_more 与 next_cursor，
// 还强迫每个列表都算一个 total——消息搜索算一次是叠着三个 LIKE '%..%' 的全表扫。
type List[T any] struct {
	Items   []T  `json:"items"`
	HasMore bool `json:"has_more"`
	// NextCursor 不透明串，原样回传即可。实现上是十进制 id，
	// 声明成不透明是为了以后换排序键时不用改契约。
	NextCursor string `json:"next_cursor,omitempty"`
	// Total 只在便宜且有意义时给。
	Total *int64 `json:"total,omitempty"`
}

// listOf 把「多查一条」的结果切回一页，并算出游标。
func listOf[T any](rows []T, limit int, cursor func(T) int64) List[T] {
	out := List[T]{Items: []T{}}
	if len(rows) > limit {
		rows = rows[:limit]
		out.HasMore = true
	}
	out.Items = rows
	if out.HasMore && len(rows) > 0 {
		out.NextCursor = strconv.FormatInt(cursor(rows[len(rows)-1]), 10)
	}
	return out
}

// 分页参数。上限挡住的是「?limit=999999 把库拖死」，
// 钳制方式和 /api/v1/messages 一致：不合法就取默认值，不报错。
func pageArgs(c *gin.Context, def int) (cursor int64, limit int) {
	cursor, _ = strconv.ParseInt(c.Query("cursor"), 10, 64)
	limit, _ = strconv.Atoi(c.Query("limit"))
	if limit <= 0 || limit > 200 {
		limit = def
	}
	return cursor, limit
}

func pathID(c *gin.Context) int64 {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return id
}

// adminFail 归过类的按 code 回，没归过类的记日志并回一句通用的。
//
// 内部错误不进 uierr：那批 code 是按「用户的下一步不同」分的，而一个
// SQL 故障没有下一步。原文只进日志——里面有表名和库的内部措辞。
func (s *Server) adminFail(c *gin.Context, what string, err error) {
	if _, ok := uierr.As(err); ok {
		s.fail(c, err)
		return
	}
	log.Error("admin api: "+what, log.Any("error", err.Error()))
	res.Rfail(c, "the server could not complete this request")
}
