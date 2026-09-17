package api

import (
	"encoding/json"
	"path/filepath"
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/migrate"
	"github.com/aichy126/knockbox/internal/service"
	"xorm.io/xorm"
)

// 客户端要在图下载完之前就把占位框画成正确的比例，否则图换进去时整页重排，
// 进频道肉眼可见地抖一下。尺寸只有服务端知道（它转码时量过），所以必须随
// 签名链接一起下发——这条测试守的就是「别哪天重构把宽高弄丢了」。
func TestFileURLsCarryPixelSize(t *testing.T) {
	dir := t.TempDir()
	e, err := xorm.NewEngine("sqlite3", filepath.Join(dir, "t.db")+"?"+migrate.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()
	if err := migrate.Run(e); err != nil {
		t.Fatal(err)
	}
	d := dao.New(e)
	files := service.NewFile(d, filepath.Join(dir, "blobs"), 64, 1600, 600, 70, "http://x", 3600)

	f, err := files.Store(1, "wide.png", testPNG(200, 120))
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{DAO: d, Files: files}

	raw, _ := json.Marshal(map[string]any{"file": f.UID})
	var out map[string]any
	if err := json.Unmarshal([]byte(s.withFileURLs(string(raw))), &out); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"file_url", "thumb_url", "file_w", "file_h"} {
		if out[k] == nil {
			t.Fatalf("extra 里缺 %s：%v", k, out)
		}
	}
	w, h := out["file_w"].(float64), out["file_h"].(float64)
	if w <= 0 || h <= 0 {
		t.Fatalf("尺寸不合法：%v x %v", w, h)
	}
	// 200x120 没超过 imageMaxPx，应当原样；比例无论如何都要是宽图
	if w <= h {
		t.Errorf("宽图的 file_w 应当大于 file_h，得到 %v x %v", w, h)
	}
}
