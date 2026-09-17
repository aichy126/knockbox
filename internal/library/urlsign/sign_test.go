package urlsign

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

var key = []byte("0123456789abcdef0123456789abcdef")

func signed(t *testing.T, uid string, thumb bool, ttl time.Duration) url.Values {
	t.Helper()
	q, err := url.ParseQuery(Sign(key, uid, thumb, ttl))
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestSignedLinkVerifies(t *testing.T) {
	for _, thumb := range []bool{false, true} {
		q := signed(t, "file-1", thumb, time.Hour)
		if err := Verify(key, "file-1", q); err != nil {
			t.Errorf("thumb=%v 自己签的链接验不过: %v", thumb, err)
		}
	}
}

// 签名覆盖的每一项被改动后都必须验不过。
// 漏掉任何一项，持有一条合法链接的人就能改出另一条——
// 改 uid 等于能下载任意附件，改 e 等于链接永不过期。
func TestVerifyRejectsTampering(t *testing.T) {
	q := signed(t, "file-1", false, time.Hour)

	t.Run("换成别人的 uid", func(t *testing.T) {
		if err := Verify(key, "file-2", q); !errors.Is(err, ErrBadSignature) {
			t.Errorf("改 uid 应当验不过，得到 %v", err)
		}
	})
	t.Run("把过期时间往后推", func(t *testing.T) {
		bad := url.Values{}
		for k, v := range q {
			bad[k] = v
		}
		bad.Set("e", strconv.FormatInt(time.Now().Add(100*time.Hour).Unix(), 10))
		if err := Verify(key, "file-1", bad); !errors.Is(err, ErrBadSignature) {
			t.Errorf("改过期时间应当验不过，得到 %v", err)
		}
	})
	t.Run("把原图换成缩略图标记", func(t *testing.T) {
		bad := url.Values{}
		for k, v := range q {
			bad[k] = v
		}
		bad.Set("t", "1")
		if err := Verify(key, "file-1", bad); !errors.Is(err, ErrBadSignature) {
			t.Errorf("改 thumb 标记应当验不过，得到 %v", err)
		}
	})
	t.Run("换一把密钥", func(t *testing.T) {
		other := []byte("ffffffffffffffffffffffffffffffff")
		if err := Verify(other, "file-1", q); !errors.Is(err, ErrBadSignature) {
			t.Errorf("别的密钥签不出同样的结果，得到 %v", err)
		}
	})
	t.Run("签名字段被截断", func(t *testing.T) {
		bad := url.Values{}
		for k, v := range q {
			bad[k] = v
		}
		bad.Set("s", q.Get("s")[:20])
		if err := Verify(key, "file-1", bad); !errors.Is(err, ErrBadSignature) {
			t.Errorf("截断的签名应当验不过，得到 %v", err)
		}
	})
}

func TestVerifyRejectsExpiredLink(t *testing.T) {
	q := signed(t, "file-1", false, -time.Minute)
	err := Verify(key, "file-1", q)
	if !errors.Is(err, ErrBadSignature) {
		t.Fatalf("过期链接应当验不过，得到 %v", err)
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("过期和签名错要能分辨，当前提示：%v", err)
	}
}

func TestVerifyRejectsMissingFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		q    url.Values
	}{
		{"什么都没带", url.Values{}},
		{"只有过期时间没有签名", url.Values{"e": {strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)}}},
		{"过期时间不是数字", url.Values{"e": {"soon"}, "s": {"deadbeef"}}},
	} {
		if err := Verify(key, "file-1", tc.q); !errors.Is(err, ErrBadSignature) {
			t.Errorf("%s 应当验不过，得到 %v", tc.name, err)
		}
	}
}
