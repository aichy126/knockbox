// Package urlsign 给附件出短期有效的签名 URL。
//
// 为什么不用 DeviceAuth 直接保护附件：**通知服务扩展要下这张图**，
// 而扩展进程里拿 Keychain 里的设备 token 是可行但脆弱的（首次解锁前读不到）。
// 签名 URL 把凭据放进链接本身，扩展只要拿到 payload 就能下图，不依赖任何本地状态。
package urlsign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

var ErrBadSignature = errors.New("附件链接无效或已过期")

// Sign 返回带 e（过期时间）与 s（签名）的查询串。
func Sign(key []byte, uid string, thumb bool, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	q := url.Values{}
	q.Set("e", strconv.FormatInt(exp, 10))
	if thumb {
		q.Set("t", "1")
	}
	q.Set("s", mac(key, uid, exp, thumb))
	return q.Encode()
}

// Verify 校验签名与有效期。
func Verify(key []byte, uid string, q url.Values) error {
	exp, err := strconv.ParseInt(q.Get("e"), 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	if time.Now().Unix() > exp {
		return fmt.Errorf("%w：链接已过期", ErrBadSignature)
	}
	want := mac(key, uid, exp, q.Get("t") == "1")
	// 定长比较：签名校验不能用 == ，那会泄露前缀匹配长度
	if !hmac.Equal([]byte(want), []byte(q.Get("s"))) {
		return ErrBadSignature
	}
	return nil
}

func mac(key []byte, uid string, exp int64, thumb bool) string {
	h := hmac.New(sha256.New, key)
	// 写进 hash.Hash 永远不会失败，这是 hash.Hash 的约定。
	_, _ = fmt.Fprintf(h, "%s|%d|%t", uid, exp, thumb)
	return hex.EncodeToString(h.Sum(nil))
}
