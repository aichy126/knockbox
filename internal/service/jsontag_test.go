package service

import (
	"reflect"
	"testing"
)

// 会直接序列化进 /admin/api 响应的结构体，每个字段都要有 json tag。
//
// 少了它，字段名就是 Go 的大驼峰（"HTTPStatus"、"Utime"），前端读
// h.http_status 得到 undefined——界面照常渲染，只是投递记录每一格都空着、
// 时间显示成 NaN，不报任何错。这一条盯的是「有没有 tag」本身，
// 不依赖造数据，所以不会因为表结构变了就跳过。
func TestResponseRowsHaveJSONTags(t *testing.T) {
	for _, v := range []any{FailureRow{}, PushLogRow{}, ReplyHookRow{}, Usage{}} {
		rt := reflect.TypeOf(v)
		t.Run(rt.Name(), func(t *testing.T) {
			for i := range rt.NumField() {
				f := rt.Field(i)
				if !f.IsExported() {
					continue
				}
				tag, ok := f.Tag.Lookup("json")
				if !ok || tag == "" {
					t.Errorf("%s.%s 没有 json tag —— 前端会读到 undefined", rt.Name(), f.Name)
					continue
				}
				if name := tag; name != "-" && name[0] >= 'A' && name[0] <= 'Z' {
					t.Errorf("%s.%s 的 json 名是 %q，应当是下划线小写", rt.Name(), f.Name, name)
				}
			}
		})
	}
}
