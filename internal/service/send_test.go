package service

import (
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/migrate"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

// 测试库和线上用同一串 pragma。手抄一份迟早抄漏，
// 而漏掉的那条要么让 bootstrap 断言失败、要么在并发写时随机报错。
const pragmas = migrate.DSN

func newDAO(t *testing.T) *dao.DAO {
	t.Helper()
	e, err := xorm.NewEngine("sqlite3", filepath.Join(t.TempDir(), "t.db")+"?"+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if err := migrate.Run(e); err != nil {
		t.Fatal(err)
	}
	return dao.New(e)
}

// fixture 一个成员 + n 台能收推送的设备 + 一个频道。
func fixture(t *testing.T, d *dao.DAO, devices int, muted bool) *models.Channel {
	t.Helper()
	acc := NewAccount(d)
	u, err := acc.Create("owner", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < devices; i++ {
		p, err := NewPair(d).Issue(u.Id, "https://x.test", "test", 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := NewDevice(d).Pair(RegisterInput{
			PairCode: p.Code, UUID: string(rune('A' + i)), APNsToken: "tok" + string(rune('A'+i)),
		}, "t", "v"); err != nil {
			t.Fatal(err)
		}
	}
	ch, err := NewChannel(d).Create(u.Id, ChannelInput{Muted: &muted})
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func count(t *testing.T, d *dao.DAO, sql string) int {
	t.Helper()
	rows, err := d.Engine().QueryString(sql)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, v := range rows[0] {
		_, _ = fmtSscanInt(v, &n)
	}
	return n
}

// 静音的频道照常落库、照常进历史，但【一条推送都不能排】。
// 这必须在服务端做：通知扩展拦不住一条通知的展示，放在客户端做静音等于没静音。
func TestMutedChannelStoresButNeverQueues(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 2, true)
	res, err := NewSend(d).Deliver(ch, SendInput{Title: "x", Body: "y"}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Muted || res.Queued {
		t.Errorf("静音频道应 Muted=true Queued=false，得到 %+v", res)
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM message"); n != 1 {
		t.Errorf("消息应照常落库，得到 %d 条", n)
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM push_log"); n != 0 {
		t.Errorf("静音频道不该产生推送条目，得到 %d 条", n)
	}
}

// 一条消息扇出到该频道所属成员的【全部】可推送设备。
func TestFanOutToAllDevicesOfOwner(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 3, false)
	res, err := NewSend(d).Deliver(ch, SendInput{Body: "hi"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Devices != 3 {
		t.Errorf("应扇出 3 台，得到 %d", res.Devices)
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM push_log"); n != 3 {
		t.Errorf("推送条目应为 3，得到 %d", n)
	}
}

// 上游重试不该造出第二条消息，更不该推第二遍。
func TestIdempotentKeyReusesMessage(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	s := NewSend(d)
	a, err := s.Deliver(ch, SendInput{Body: "一", IdemKey: "k1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Deliver(ch, SendInput{Body: "二", IdemKey: "k1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if a.UID != b.UID || !b.Dedup || b.Queued {
		t.Errorf("重复的 idem_key 应复用同一条且不再排队：a=%+v b=%+v", a, b)
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM message"); n != 1 {
		t.Errorf("应只有 1 条消息，得到 %d", n)
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM push_log"); n != 1 {
		t.Errorf("应只排 1 条推送，得到 %d", n)
	}
}

// rev 是增量同步的唯一游标，一旦出现空洞，游标越过去的客户端就永远拉不回那段。
func TestRevIsGapless(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	s := NewSend(d)
	for i := 0; i < 5; i++ {
		if _, err := s.Deliver(ch, SendInput{Body: "x"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	rows, _ := d.Engine().QueryString("SELECT rev FROM message ORDER BY rev")
	for i, r := range rows {
		var got int
		_, _ = fmtSscanInt(r["rev"], &got)
		if got != i+1 {
			t.Fatalf("rev 应连续，第 %d 条是 %d", i+1, got)
		}
	}
}

// 部分失败要逐条报告，不能让一个坏 token 把整批打回。
func TestBatchReportsPerToken(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	out := NewSend(d).Batch([]string{ch.Token, "ch_NOPE", ch.Token}, SendInput{Body: "x"}, "", nil)
	if len(out) != 3 {
		t.Fatalf("应有 3 条结果，得到 %d", len(out))
	}
	if !out[0].OK || out[1].OK || !out[2].OK {
		t.Errorf("好坏应各自独立：%+v", out)
	}
	if out[1].Error == "" {
		t.Error("失败的那条应带原因")
	}
}

// NSE 被系统跳过时，payload 里的摘要就是用户能看到的全部，所以它不能是 markdown 原文。
func TestSummarizeStripsMarkdown(t *testing.T) {
	got := Summarize("## 构建失败\n\n- main 分支\n- `exit 1`\n", 300)
	want := "构建失败 · main 分支 · exit 1"
	if got != want {
		t.Errorf("摘要 = %q, 期望 %q", got, want)
	}
	if n := len([]rune(Summarize(longText(400), 300))); n > 300 {
		t.Errorf("摘要应截断到 300 字符，得到 %d", n)
	}
}

func longText(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = '字'
	}
	return string(b)
}

// 摘要会进 APNs 的 alert.body；通知扩展被跳过时它就是锁屏上的全部内容。
// 所以代码块、表格记号绝不能出现在里面。
func TestSummarizeSkipsCodeBlocksAndTables(t *testing.T) {
	body := "### 失败原因\n**main** 分支 · 42s · 提交 `42bde07`\n\n" +
		"- step **build** 以 exit 1 结束\n\n" +
		"```go\nfunc Execute() {\n    root.AddCommand()\n}\n```\n\n" +
		"| 步骤 | 状态 | 耗时 |\n|---|---|---|\n| build | 失败 | 18s |\n\n" +
		"查看构建：[看日志](https://ci.example.com/1832)\n"
	got := Summarize(body, 300)

	for _, bad := range []string{"```", "|---|", "func Execute", "root.AddCommand", "https://"} {
		if strings.Contains(got, bad) {
			t.Errorf("摘要里不该出现 %q：\n  %s", bad, got)
		}
	}
	for _, want := range []string{"失败原因", "main 分支", "42bde07", "看日志"} {
		if !strings.Contains(got, want) {
			t.Errorf("摘要里应该保留 %q：\n  %s", want, got)
		}
	}
	// 表格数据行要变成人能读的文字
	if !strings.Contains(got, "步骤 状态 耗时") {
		t.Errorf("表格行应折成空格分隔：\n  %s", got)
	}
}
