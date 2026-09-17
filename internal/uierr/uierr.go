// Package uierr 面向使用者的错误。
//
// 和面向调用方的错误（发送 API 那批，英文成句返回）不是一回事：这里的错误最终
// 显示在一个人眼前——iOS 客户端里的一句提示、浏览器打开附件时的一行字。
// 谁读它，就该用谁的语言，而服务端不知道那是谁。
//
// 所以服务端返回的是一个稳定的 code 加上插值参数，句子在两个地方成形：
// 服务端按请求的 Accept-Language 渲染一份放进 msg（老客户端直接显示它），
// 客户端也可以拿 code 自己组织措辞（新客户端该这么做，那才是真正的本地化）。
package uierr

import "errors"

// Error 一个能被本地化的错误。
type Error struct {
	// Code 稳定标识。改它等于改 API 契约——客户端按它分支。
	Code string
	// Args 句子里的插值，顺序与语料里的 %s / %d 一一对应。
	Args []any
	// Err 原始错误。它只进日志，永远不进响应：
	// 里面可能有表名、路径、库的内部措辞，对读的人没有意义。
	Err error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Err.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Err }

// New 造一个没有下层错误的。
func New(code string, args ...any) *Error {
	return &Error{Code: code, Args: args}
}

// Wrap 包住一个已有的错误，让它带上 code。
// 原错误仍在链上，errors.Is / errors.As 照常工作。
func Wrap(err error, code string, args ...any) *Error {
	return &Error{Code: code, Args: args, Err: err}
}

// As 从错误链里取出第一个 uierr。
// 拿不到说明这个错误没被归过类——调用方该当成 500 处理，
// 而不是把它的文本直接给人看。
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
