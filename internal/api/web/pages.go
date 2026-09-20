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
<html lang="{{.HTMLLang}}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<title>{{.Title}} · {{.ServerName}}</title>` + IconLinks + `
<style>` + Style + LangCSS + `</style></head><body>{{.LangSwitch}}{{end}}

{{define "brand"}}<div class="brand"><div class="brand-mark">` + Mark + `</div>
<div><div class="brand-name">Knockbox</div><div class="brand-sub">{{.ServerName}}</div></div></div>{{end}}

{{define "login"}}{{template "head" .}}
<div class="wrap login">
  {{template "brand" .}}
  <form class="card" method="post" action="/login">
    {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
    {{if .Notice}}<div class="note"><p>{{.Notice}}</p></div>{{end}}
    <div class="field"><label for="u">{{.T.Username}}</label><input id="u" name="username" autocomplete="username" autofocus required></div>
    <div class="field"><label for="p">{{.T.Password}}</label><input id="p" name="password" type="password" autocomplete="current-password" required></div>
    <button class="btn primary" type="submit">{{.T.Title}}</button>
    <div class="note"><p>{{.T.NoSignup}}</p></div>
  </form>
</div></body></html>{{end}}

`))

type Data struct {
	Title      string
	ServerName string
	// HTMLLang 进 <html lang="…">。屏幕阅读器和浏览器的翻译提示都读它。
	HTMLLang string
	// LangSwitch 右上角的语言开关，已经渲染好的 HTML。
	//
	// 登录页也要能切：它是陌生人看到的第一屏，而这时还没有任何偏好可循。
	// 类型是 template.HTML 而不是 string —— 它是我们自己生成的标记，
	// 不是外部输入；用 string 的话 html/template 会把它整段转义掉。
	LangSwitch template.HTML
	// T 这一语的管理界面文案。
	T     AdminLogin
	Error string
	// Notice 中性提示，和 Error 分开：密码改完跳回登录页时要说一句
	// 「已经改好了，用新的登录」，那不是错误。
	Notice string
}

// Render 渲染一页。语言相关的三个字段在这里一次填好，
// 免得每个 handler 都记得传——漏一个就是一页英文里夹着中文。
func Render(name string, l Lang, d Data) (string, error) {
	d.HTMLLang = l.Attr()
	d.LangSwitch = template.HTML(LoginLangSwitch(l))
	d.T = T(l).Admin.Login
	var b strings.Builder
	if err := tpl.ExecuteTemplate(&b, name, d); err != nil {
		return "", err
	}
	return b.String(), nil
}
