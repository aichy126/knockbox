package web

import (
	"html/template"
	"strings"
)

// Data 页面数据。

// 用 html/template 而不是字符串拼接：这几页会渲染用户可控的内容
// （服务器名、账号名），拼字符串迟早会拼出一个 XSS。
var tpl = template.Must(template.New("").Parse(`
{{define "head"}}<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>{{.Title}} · {{.ServerName}}</title>
<style>` + Style + `</style></head><body>{{end}}

{{define "brand"}}<div class="brand"><div class="brand-mark">` + Mark + `</div>
<div><div class="brand-name">Knockbox</div><div class="brand-sub">{{.ServerName}}</div></div></div>{{end}}

{{define "login"}}{{template "head" .}}
<div class="wrap login">
  {{template "brand" .}}
  <form class="card" method="post" action="/login">
    {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
    {{if .Notice}}<div class="note"><p>{{.Notice}}</p></div>{{end}}
    <div class="field"><label for="u">用户名</label><input id="u" name="username" autocomplete="username" autofocus required></div>
    <div class="field"><label for="p">密码</label><input id="p" name="password" type="password" autocomplete="current-password" required></div>
    <button class="btn primary" type="submit">登录</button>
    <div class="note"><p>没有注册入口。第一个账号在服务器首次启动时自动创建，用户名和密码打印在启动日志里。</p></div>
  </form>
</div></body></html>{{end}}

`))

type Data struct {
	Title      string
	ServerName string
	Error      string
	// Notice 中性提示，和 Error 分开：密码改完跳回登录页时要说一句
	// 「已经改好了，用新的登录」，那不是错误。
	Notice string
}

func Render(name string, d Data) (string, error) {
	var b strings.Builder
	if err := tpl.ExecuteTemplate(&b, name, d); err != nil {
		return "", err
	}
	return b.String(), nil
}
