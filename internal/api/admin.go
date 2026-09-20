package api

import (
	"encoding/json"
	"strings"

	"github.com/aichy126/knockbox/internal/service"
)

// 后台接口层共用的几样。页面本身在 admin/（Vue），这里只剩数据侧。

// channelName 从不透明 meta 里取名字。服务端平时不解析 meta，
// 只有管理界面例外——给人看的列表里显示一串 ULID 是没法用的。
//
// 必须真的按 JSON 解析。用字符串查找 "name" 的话，值里出现这几个字符
// （或者值本身带转义引号）就会取到错误的内容。
func channelName(meta, id string) string {
	var m struct {
		Name string `json:"name"`
	}
	if json.Unmarshal([]byte(meta), &m) == nil {
		if n := strings.TrimSpace(m.Name); n != "" {
			return n
		}
	}
	if len(id) > 6 {
		return id[:6]
	}
	return id
}

// extraFileUID 从 extra 里取附件 uid。不引 json 解码器是因为这里只要一个字段，
// 而 extra 的形状由服务端自己写入，不是外部输入。
func extraFileUID(extra string) string {
	i := strings.Index(extra, `"file"`)
	if i < 0 {
		return ""
	}
	rest := extra[i+6:]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	rest = rest[j+1:]
	k := strings.Index(rest, `"`)
	if k <= 0 {
		return ""
	}
	return rest[:k]
}

// admin 后台的查询与写操作。无状态，每次现造。
func (s *Server) admin() *service.Admin { return service.NewAdmin(s.DAO) }

// quota 每次用当前设置构造——设置在后台改完要立刻生效，不能等重启。
func (s *Server) quota() *service.Quota {
	return service.NewQuota(s.DAO, s.Settings.Limits())
}
