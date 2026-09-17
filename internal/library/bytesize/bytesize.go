// Package bytesize 把字节数写成给人看的大小。
package bytesize

import "fmt"

// Human 按能整除的最大单位格式化。
//
// 单位不是固定的 MB：上限被配成 512 KB 时,「附件超过大小上限（0 MB）」
// 既没告诉他限制是多少，又像是服务坏了。小于一个单位就退到下一档。
func Human(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n>>20)
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	case n == 1:
		return "1 byte"
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}
