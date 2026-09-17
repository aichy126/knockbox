package web

import (
	"fmt"
	"strconv"
	"strings"
)

type JoinData struct {
	SiteName, Host, Code, QR, DocsURL, Notice, StatusURL string
	// TTLMinutes 配对码还能用多久，直接显示给用户。写死的话改了配置页面还是老说法。
	TTLMinutes int
	// Link 二维码里那条 knockbox:// 深链。同一台手机上直接点它就能接入，
	// 不用再想办法扫自己屏幕上的码。
	Link string
	Lang Lang
}

// JoinPage 扫码接入页。**这是对外的第一屏**，所以：
//   - 二维码最大、最先看到
//   - 接入之后要做什么写在右边，不用翻页
//   - 「没有账号密码、设备全丢就找不回」这句必须在页面上，不能让用户事后才发现
//
// 模板用 Replacer 而不是 Sprintf：这一页里既有 CSS 的 `100%` 也有 JS 字符串，
// 占位符按名字替换才不会因为多插一句文案就把后面的参数全排错位。
func JoinPage(d JoinData) string {
	t := T(d.Lang)

	notice := ""
	if d.Notice != "" {
		notice = `<div class="note">` + Svg("alert", 14) + `<div>` + E(d.Notice) + `</div></div>`
	}
	docs := ""
	if d.DocsURL != "" {
		docs = `<a class="btn outline" href="` + E(d.DocsURL) + `">` + Svg("msg", 15) + E(t.JoinHowTo) + `</a>`
	}

	return strings.NewReplacer(
		"{{LANG}}", d.Lang.Attr(),
		"{{STYLE}}", Style,
		"{{LANGCSS}}", LangCSS,
		"{{LANGSW}}", LangSwitch(d.Lang),
		"{{TITLE}}", E(fmt.Sprintf(t.JoinTitle, d.SiteName)),
		"{{HEADING}}", E(fmt.Sprintf(t.JoinHeading, d.SiteName)),
		"{{LEDE}}", t.JoinLede,
		"{{NOTICE}}", notice,
		"{{QR}}", d.QR,
		"{{CODE}}", E(strings.ToUpper(d.Code)),
		"{{HOST}}", E(d.Host),
		"{{VALIDFOR}}", E(fmt.Sprintf(t.JoinValidFor, d.TTLMinutes)),
		"{{LINK}}", E(d.Link),
		"{{QRICON}}", Svg("qr", 14),
		"{{OPENHERE}}", E(t.JoinOpenHere),
		"{{OPENHINT}}", E(t.JoinOpenHint),
		"{{AFTERSCAN}}", E(t.JoinAfterScan),
		"{{STEP1}}", t.JoinStep1,
		"{{STEP2}}", t.JoinStep2,
		"{{STEP3}}", t.JoinStep3,
		"{{ALERT}}", Svg("alert", 14),
		"{{NOACCOUNT}}", t.JoinNoAccount,
		"{{DOCS}}", docs,
		"{{STATUSURL}}", jsStr(d.StatusURL),
		"{{JS_COPY}}", jsStr(t.JoinCopy),
		"{{JS_COPIED}}", jsStr(t.JoinCopied),
		"{{JS_PAIRED}}", jsStr(t.JoinPaired),
		"{{JS_PAIREDHINT}}", jsStr(t.JoinPairedHint),
		"{{JS_YOURURL}}", jsStr(t.JoinYourURL),
		"{{JS_YOURURLSUB}}", jsStr(t.JoinYourURLSub),
		"{{JS_OPENSEND}}", jsStr(t.JoinOpenSend),
		"{{JS_KEEPSECRET}}", jsStr(t.JoinKeepSecret),
		"{{JS_ADDDEVICE}}", jsStr(t.JoinAddDevice),
	).Replace(joinTpl)
}

// jsStr 把一段文案变成 JS 字面量。用 strconv.Quote 而不是自己拼引号：
// 文案里出现一个引号或反斜杠就能把整段脚本打坏，而这些字是要被翻译的，
// 将来谁加一句带引号的都不该是一次故障。
func jsStr(s string) string { return strconv.Quote(s) }

const joinTpl = `<!doctype html><html lang="{{LANG}}"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{TITLE}}</title><style>{{STYLE}}{{LANGCSS}}
.join{max-width:1040px;margin:0 auto;padding:48px 24px}
.join h1{font-size:30px;margin:0 0 6px;letter-spacing:-0.02em}
.join .lede{color:var(--muted-fg);margin:0 0 28px;font-size:15px;line-height:1.6;max-width:60ch}
.join .cols{display:grid;grid-template-columns:320px 1fr;gap:20px;align-items:start}
@media (max-width:820px){.join .cols{grid-template-columns:1fr}}
.qrbox{text-align:center;padding:22px}
#qrwrap svg{width:248px;height:248px;border-radius:12px;display:block;margin:0 auto}
.codetext{font-family:ui-monospace,Menlo,monospace;font-size:26px;font-weight:600;letter-spacing:.08em;margin:14px 0 2px}
.steps{counter-reset:s;display:flex;flex-direction:column;gap:14px;padding:4px 0 0}
.steps li{list-style:none;display:flex;gap:12px;align-items:flex-start}
/* 必须是 li 的【直接】子元素且是第一个：写成 .steps b 会把步骤正文里
   任何一个 <b> 也套成 22px 的圆形徽标，一个字一行竖着排下来。 */
.steps li>b:first-child{flex-shrink:0;width:22px;height:22px;border-radius:999px;background:var(--accent-soft);color:var(--primary);font-size:12px;display:flex;align-items:center;justify-content:center;margin-top:1px}
.ok-state{display:none;padding:20px;text-align:center}
.joinurl{display:flex;gap:8px;align-items:flex-start;background:var(--muted);
  border:1px solid var(--line);border-radius:10px;padding:9px 9px 9px 12px}
/* 折行而不是横向滚：这是个要看全、要抄下来的地址，让用户先发现它能滚、
   再滑到头，是把本该由排版解决的事推给了他。 */
.joinurl code{flex:1 1 auto;min-width:0;white-space:pre-wrap;overflow-wrap:anywhere;
  font-family:ui-monospace,Menlo,monospace;font-size:12px;line-height:1.65;padding-top:3px}
.joinurl button{flex:0 0 auto}
</style></head><body>{{LANGSW}}<div class="app"><div class="join">
  <h1>{{HEADING}}</h1>
  <p class="lede">{{LEDE}}</p>
  {{NOTICE}}
  <div class="cols">
    <div class="card qrbox" id="qrcard">
      <div id="qrwrap">{{QR}}</div>
      <div class="codetext">{{CODE}}</div>
      <div class="dim" style="font-size:12.5px">{{HOST}}</div>
      <div class="dim" style="font-size:12px;margin-top:10px">{{VALIDFOR}}</div>
      <!-- 就在这台 iPhone 上打开这一页时，摄像头扫不了自己这块屏幕。
           而二维码里本来就是一条 knockbox:// 链接，直接点它唤起 app 即可，
           连截图再从相册选那一步都省了。非 iOS 或没装 app 的浏览器点了没反应，
           所以只把它作为次要入口摆在二维码下面，不抢扫码的位置。 -->
      <a class="btn outline sm" style="margin-top:14px" href="{{LINK}}">{{QRICON}}{{OPENHERE}}</a>
      <div class="dim" style="font-size:11.5px;margin-top:6px">{{OPENHINT}}</div>
    </div>
    <div class="card"><div class="card-b" style="padding:20px">
      <div style="font-weight:600;margin-bottom:10px">{{AFTERSCAN}}</div>
      <ol class="steps">
        <li><b>1</b><div>{{STEP1}}</div></li>
        <li><b>2</b><div>{{STEP2}}</div></li>
        <li><b>3</b><div>{{STEP3}}</div></li>
      </ol>
      <div class="note" style="margin-top:16px">{{ALERT}}<div>{{NOACCOUNT}}</div></div>
      <div style="display:flex;gap:8px;margin-top:16px">{{DOCS}}</div>
    </div></div>
  </div>
</div></div>
<script>
var TXT = {
  copy: {{JS_COPY}}, copied: {{JS_COPIED}},
  paired: {{JS_PAIRED}}, pairedHint: {{JS_PAIREDHINT}},
  yourUrl: {{JS_YOURURL}}, yourUrlSub: {{JS_YOURURLSUB}},
  openSend: {{JS_OPENSEND}}, keepSecret: {{JS_KEEPSECRET}}, addDevice: {{JS_ADDDEVICE}}
};
function cpUrl(b, u){
  navigator.clipboard.writeText(u).then(function(){
    b.textContent=TXT.copied; setTimeout(function(){b.textContent=TXT.copy},1500);
  });
}
// 轮询到配对成功就换成「已接入」，不用用户自己刷新去猜有没有成功。
//
// 间隔【先快后慢】：扫码就发生在打开这一页之后的头十几秒，那段时间用户正盯着屏幕，
// 每秒问一次；之后逐步拉长到 6 秒。固定 10 秒省下的那点请求，
// 代价是扫完之后干等十秒——而那十秒里他会以为没成功。
(function () {
  var url = {{STATUSURL}}, tries = 0, wait = 800;
  function poll() {
    if (++tries > 200) return;                    // 码早就过期了，别再问
    fetch(url).then(function (r) { return r.json() }).then(function (j) {
      if (!j.paired) {
        wait = Math.min(wait * 1.18, 6000);
        return setTimeout(poll, wait);
      }
      var extra = '';
      if (j.send_url) {
        // ⚠️ 必须把【真实地址】显示出来，不能只写「保存好这个地址」。
        // 用户正站在 /join 这一页上，「这个地址」会被读成当前页的地址——
        // 而要保存的是下面这条完全不同的 URL。显示出来，歧义就没有了。
        extra =
          '<div style="margin-top:18px;text-align:left">' +
          '<div style="font-weight:600;margin-bottom:4px">' + TXT.yourUrl + '</div>' +
          '<div class="dim" style="font-size:12.5px;margin-bottom:8px">' + TXT.yourUrlSub + '</div>' +
          '<div class="joinurl"><code>' + j.send_url + '</code>' +
          '<button class="btn outline sm" onclick="cpUrl(this,\'' + j.send_url + '\')">' + TXT.copy + '</button></div>' +
          '<a class="btn" style="width:100%;justify-content:center;margin-top:10px" href="' + j.send_url + '">' + TXT.openSend + '</a>' +
          '<div class="dim" style="font-size:12px;margin-top:10px">' + TXT.keepSecret + '</div>' +
          '</div>';
      }
      document.getElementById('qrcard').innerHTML =
        '<div class="ok-state" style="display:block">' +
        '<div class="badge ok" style="height:28px;font-size:14px"><i></i>' + TXT.paired + '</div>' +
        '<div class="dim" style="margin-top:12px;font-size:13px">' + TXT.pairedHint + '</div>' +
        extra +
        '<div class="dim" style="margin-top:14px;font-size:12px">' + TXT.addDevice + '</div></div>';
    }).catch(function () { setTimeout(poll, Math.min(wait * 2, 6000)); });
  }
  poll();
})();
</script></body></html>`
