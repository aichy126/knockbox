package service

import "fmt"

// fmtSscan / fmtSscanInt：xorm 的 QueryString 返回的都是字符串，
// 这两个薄封装把它们读成数字。
func fmtSscan(s string, v *int64) (int, error)  { return fmt.Sscan(s, v) }
func fmtSscanInt(s string, v *int) (int, error) { return fmt.Sscan(s, v) }
