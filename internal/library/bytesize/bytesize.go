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

// Decimal 带一位小数的大小，给「已经用了多少」这类展示用：1.5 MB 比 1 MB 有信息量。
//
// KB 档刻意不给小数——附件一共占了 3.4 KB 还是 3 KB，对看的人没有区别，
// 而多出来的那一位会让一列数字读起来更费劲。
func Decimal(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
