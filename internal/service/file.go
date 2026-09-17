package service

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/library/bytesize"
	"github.com/aichy126/knockbox/internal/library/idgen"
	"github.com/aichy126/knockbox/internal/library/urlsign"
	"github.com/aichy126/knockbox/internal/models"
	"golang.org/x/image/draw"
)

var ErrFileTooLarge = errors.New("attachment exceeds the size limit")

// orphanGrace 上传之后还没有被任何消息引用的附件，在这段时间内不回收。
//
// /upload 与随后的 /send 是两次请求——批量投递时一次上传对应 N 次发送，
// 间隔由调用方决定。没有宽限期的话，中间只要撞上一轮 GC，
// 消息就会引用到一个已经被扫掉的 blob。
const orphanGrace = 24 * time.Hour

// File 附件存储。按 sha256 内容寻址：同一张告警图反复发只占一份。
type File struct {
	d           *dao.DAO
	dir         string
	maxBytes    int64
	imageMaxPx  int   // 正文图长边上限
	thumbPx     int   // 通知小图标长边上限
	quality     int   // JPEG 质量
	keepAsIs    int64 // 尺寸已达标且小于它的图原样保留，不重新编码
	externalURL string
	ttl         time.Duration
	key         []byte
	keyOnce     sync.Once
}

func NewFile(d *dao.DAO, dir string, maxMB, imageMaxPx, thumbPx, quality int, externalURL string, ttlSec int64) *File {
	if maxMB <= 0 {
		maxMB = 20
	}
	if imageMaxPx <= 0 {
		imageMaxPx = 1600
	}
	if thumbPx <= 0 {
		thumbPx = 600
	}
	if quality <= 0 || quality > 100 {
		quality = 82
	}
	if ttlSec <= 0 {
		ttlSec = 7 * 24 * 3600
	}
	return &File{d: d, dir: dir, maxBytes: int64(maxMB) << 20,
		imageMaxPx: imageMaxPx, thumbPx: thumbPx, quality: quality, keepAsIs: 400 << 10,
		externalURL: strings.TrimRight(externalURL, "/"), ttl: time.Duration(ttlSec) * time.Second}
}

// MaxBytes 单个附件的大小上限（storage.max_upload_mb）。
// 暴露出来是为了让 HTTP 层在把请求体读进内存【之前】就能拒绝，
// 而不是读完再由 Store 判一次。
func (s *File) MaxBytes() int64 { return s.maxBytes }

// SignKey 首启随机生成并存进 kv。**绝不给默认值**——
// 开源项目带默认密钥等于全网可读任意附件。
func (s *File) SignKey() []byte {
	s.keyOnce.Do(func() {
		if v, ok, _ := s.d.KVGet(models.KVFileSignKey); ok && v != "" {
			if b, err := hex.DecodeString(v); err == nil && len(b) == 32 {
				s.key = b
				return
			}
		}
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		_ = s.d.KVSet(models.KVFileSignKey, hex.EncodeToString(b), time.Now().Unix())
		s.key = b
	})
	return s.key
}

// URL 拼出可直接下载的签名链接。服务端拼好给客户端，客户端不该自己组装。
func (s *File) URL(uid string, thumb bool) string {
	if uid == "" {
		return ""
	}
	return s.externalURL + "/api/v1/file/" + uid + "?" + urlsign.Sign(s.SignKey(), uid, thumb, s.ttl)
}

// Store 落盘并登记。
//
// 图片一律重新编码，不保留原图：这是通知服务不是网盘，图看得清即可。
// 存两份派生图——
//
//	· 正文图：长边 ≤ imageMaxPx，点开看的就是它
//	· 小图标：长边 ≤ thumbPx，进 APNs payload 给通知扩展下，
//	  通知上那块 app 图标换成它；扩展只有 24MB 预算，必须小
//
// 去重仍按【原始字节】的 sha256：同一张图反复发能命中已有行、不必重新编码。
// 所以 file.sha256 描述的是「上传的那份」，磁盘上放的却是派生图——
// 别拿 sha256 去校验磁盘文件。
func (s *File) Store(userID int64, name string, data []byte) (*models.File, error) {
	if int64(len(data)) > s.maxBytes {
		return nil, fmt.Errorf("%w (%s)", ErrFileTooLarge, bytesize.Human(s.maxBytes))
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	// 去重按 (user, sha256)：内容一样但属于别人的文件不能共用同一行，
	// 否则清空一个用户的消息会把另一个用户的引用计数也带下去。
	var exist models.File
	ok, err := s.d.Engine().Where("user_id = ? AND sha256 = ?", userID, hash).Get(&exist)
	if err != nil {
		return nil, err
	}
	if ok {
		return &exist, nil
	}

	f := &models.File{
		UID: idgen.ULID(), UserId: userID, SHA256: hash, Name: name,
		Storage: "local", Ctime: time.Now().Unix(),
	}
	body, mime := data, http.DetectContentType(data)
	ext := ""

	if src, _, derr := image.Decode(bytes.NewReader(data)); derr == nil {
		b := src.Bounds()
		f.Width, f.Height = b.Dx(), b.Dy()
		// 已经又小又省的图原样留着：把一张干净的 PNG 截图转成 JPEG
		// 只会让文字边缘发毛，还未必更小。
		if b.Dx() <= s.imageMaxPx && b.Dy() <= s.imageMaxPx && int64(len(data)) <= s.keepAsIs {
			body = data
		} else if enc, w, h, eerr := s.encode(src, s.imageMaxPx, s.quality); eerr == nil {
			body, mime, ext = enc, "image/jpeg", ".jpg"
			f.Width, f.Height = w, h
		}
		// 小图标从同一次解码出，不再读盘。质量再降一档：
		// 它在通知上只有几十 pt，省下的字节全是扩展那 24MB 预算里的余量。
		if tb, _, _, terr := s.encode(src, s.thumbPx, s.quality-12); terr == nil && len(tb) < len(body) {
			trel := filepath.Join(hash[:2], hash+".thumb.jpg")
			if err := os.MkdirAll(filepath.Join(s.dir, hash[:2]), 0o755); err == nil {
				if err := os.WriteFile(filepath.Join(s.dir, trel), tb, 0o644); err == nil {
					f.ThumbPath = trel
				}
			}
		}
	}

	rel := filepath.Join(hash[:2], hash+ext)
	abs := filepath.Join(s.dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(abs, body, 0o644); err != nil {
		return nil, err
	}
	f.Path, f.Mime, f.Size = rel, mime, int64(len(body))

	if _, err := s.d.Engine().Insert(f); err != nil {
		return nil, err
	}
	return f, nil
}

// encode 等比缩到 maxPx 以内并转 JPEG。
//
// 带 alpha 的图**必须先铺白底**：JPEG 没有 alpha 通道，直接编码会把透明区域变成黑块。
func (s *File) encode(src image.Image, maxPx, quality int) ([]byte, int, int, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, 0, 0, errors.New("empty image")
	}
	tw, th := w, h
	if w > maxPx || h > maxPx {
		if w >= h {
			tw, th = maxPx, h*maxPx/w
		} else {
			tw, th = w*maxPx/h, maxPx
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, 0, 0, err
	}
	return out.Bytes(), tw, th, nil
}

func (s *File) ByUID(uid string) (*models.File, error) {
	var f models.File
	ok, err := s.d.Engine().Where("uid = ?", uid).Get(&f)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("attachment not found")
	}
	return &f, nil
}

// Open 打开附件内容，**调用方负责 Close**。thumb=true 时优先给缩略图，没有就退回原图。
//
// 返回句柄而不是字节：附件最大 20 MB，读进内存再吐的话，并发下载时每一路
// 都常驻一份；交给 http.ServeContent 还顺带支持 Range 请求。
func (s *File) Open(f *models.File, thumb bool) (*os.File, string, error) {
	p, mime := f.Path, f.Mime
	if thumb && f.ThumbPath != "" {
		p, mime = f.ThumbPath, "image/jpeg"
	}
	fh, err := os.Open(filepath.Join(s.dir, p))
	if err != nil {
		return nil, "", err
	}
	return fh, mime, nil
}

// ReleaseUnused 把没人引用的 blob 真正删掉，返回删除个数。
//
// 引用过的附件降回 0 就立即回收；从未被引用过的要等过了 orphanGrace，
// 否则会在「上传完、消息还没发」的窗口里把附件扫掉。
func (s *File) ReleaseUnused() (int, error) {
	var list []models.File
	if err := s.d.Engine().
		Where("ref_count <= 0 AND (ever_referenced <> 0 OR created_at < ?)",
			time.Now().Add(-orphanGrace).Unix()).
		Find(&list); err != nil {
		return 0, err
	}
	n := 0
	for _, f := range list {
		// 先删行、再删文件。反过来的话，两步之间 Store 可能按 sha256 命中这一行，
		// 于是新消息引用到一个 blob 已经不在的附件。
		if _, err := s.d.Engine().ID(f.Id).Delete(&models.File{}); err != nil {
			continue
		}
		_ = os.Remove(filepath.Join(s.dir, f.Path))
		if f.ThumbPath != "" {
			_ = os.Remove(filepath.Join(s.dir, f.ThumbPath))
		}
		n++
	}
	return n, nil
}
