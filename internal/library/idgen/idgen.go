// Package idgen 生成对外 ID 与各类凭据。
//
// 统一用 Crockford Base32：它排除了 I / L / O / U 四个易混字符，
// 配对码要用户手输，这一点是刚需。
package idgen

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ULID 返回 26 字符的 ULID：48 位毫秒时间戳 + 80 位随机。
// 选它而不是 UUID 是因为字典序 == 时间序，做游标和排序时不用额外的时间列。
func ULID() string {
	var buf [16]byte
	ms := uint64(time.Now().UnixMilli())
	buf[0] = byte(ms >> 40)
	buf[1] = byte(ms >> 32)
	buf[2] = byte(ms >> 24)
	buf[3] = byte(ms >> 16)
	buf[4] = byte(ms >> 8)
	buf[5] = byte(ms)
	if _, err := rand.Read(buf[6:]); err != nil {
		panic("idgen: the system entropy source is unavailable: " + err.Error())
	}

	out := make([]byte, 26)
	// 128 位按 5 位一组编码成 26 个字符，最高位那组只有 3 位有效。
	out[0] = crockford[(buf[0]&224)>>5]
	out[1] = crockford[buf[0]&31]
	bits := uint(0)
	acc := uint32(0)
	i := 2
	for _, b := range buf[1:] {
		acc = acc<<8 | uint32(b)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out[i] = crockford[(acc>>bits)&31]
			i++
		}
	}
	return string(out)
}

// Token 生成 256 位随机凭据，形如 "ch_3F7K...".
// 用于频道的发送 token 和设备 token——两者都是高熵随机串，
// 校验时直接比 sha256 即可，不需要 bcrypt 那种抗字典的开销。
func Token(prefix string) string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("idgen: the system entropy source is unavailable: " + err.Error())
	}
	return prefix + encode(buf[:])
}

// PairCode 生成 8 字符（40 位）的一次性配对码，形如 "K7M2-9XQP"。
//
// 40 位配合「单次使用 + 10 分钟过期 + 每 IP 每分钟 5 次」足够；
// 不做更长是因为它要被人念出来或手输。
func PairCode() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("idgen: the system entropy source is unavailable: " + err.Error())
	}
	n := binary.BigEndian.Uint64(buf[:])
	out := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		out[i] = crockford[n&31]
		n >>= 5
	}
	return string(out)
}

// NormalizePairCode 去掉分隔符、转大写，并把 Crockford 的等价字符归一
// （用户很容易把 0 打成 O、1 打成 I 或 L）。
func NormalizePairCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		switch r {
		case '-', ' ', '_':
			continue
		case 'O':
			b.WriteRune('0')
		case 'I', 'L':
			b.WriteRune('1')
		case 'U':
			b.WriteRune('V')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FormatPairCode 按 4 位分组，只用于展示。
func FormatPairCode(code string) string {
	if len(code) != 8 {
		return code
	}
	return fmt.Sprintf("%s-%s", code[:4], code[4:])
}

func encode(b []byte) string {
	var out strings.Builder
	bits, acc := uint(0), uint32(0)
	for _, v := range b {
		acc = acc<<8 | uint32(v)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out.WriteByte(crockford[(acc>>bits)&31])
		}
	}
	if bits > 0 {
		out.WriteByte(crockford[(acc<<(5-bits))&31])
	}
	return out.String()
}
