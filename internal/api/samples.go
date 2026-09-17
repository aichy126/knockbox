package api

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
)

// sampleText 一条示例里所有会被人读到的字，每种语言一份。
//
// **连 Curl 和 Input 也要分语言**：示例命令里带着 `-d "洗衣机洗完了"`，
// 而按下「发这条」真推到手机上的也是那句话。只翻标题和说明的话，
// 英文用户读完一段英文，抄下来的却是一条中文命令，按一下手机响的还是中文。
type sampleText struct {
	Title string
	Desc  string
	// Curl 展示用的命令。{{URL}} 会被换成这个频道的真实地址。
	Curl string
	// Input 真发时用的内容。NeedsImage 的那条在发送时才现做图。
	Input service.SendInput
}

// sample 一个可以「看代码」也可以「真发一条」的示例。
//
// **展示的命令和真发出去的那条必须来自同一份定义**，否则页面上写的和按钮做的
// 迟早会对不上——而这一页存在的全部意义就是让人照着抄。
type sample struct {
	ID string
	// Lang 代码块左上角那个标签
	Lang       string
	NeedsImage bool
	T          map[web.Lang]sampleText
}

// text 取这一语的文案，没有就回落英文——多一种语言时漏译一条，
// 应该是「这一条还是英文」，不是整页空白。
func (s sample) text(l web.Lang) sampleText {
	if t, ok := s.T[l]; ok {
		return t
	}
	return s.T[web.LangEN]
}

var samples = []sample{
	{
		ID:   "text",
		Lang: "bash",
		T: map[web.Lang]sampleText{
			web.LangEN: {
				Title: "The simplest one",
				Desc:  "The whole body is the message; nothing to configure. It arrives as a plain text notification.",
				Curl:  `curl -d "The laundry is done" {{URL}}`,
				Input: service.SendInput{Type: models.TypeText, Body: "The laundry is done. Second spin has finished."},
			},
			web.LangZH: {
				Title: "最简单的一条",
				Desc:  "整个 body 就是正文，什么都不用配。手机上是一条纯文本通知。",
				Curl:  `curl -d "洗衣机洗完了" {{URL}}`,
				Input: service.SendInput{Type: models.TypeText, Body: "洗衣机洗完了，第 2 次甩干结束。"},
			},
		},
	},
	{
		ID:   "title",
		Lang: "bash",
		T: map[web.Lang]sampleText{
			web.LangEN: {
				Title: "With a title",
				Desc:  "The title is the first line of the notification, the body sits under it. Plain GET works too, for places that can only build a URL.",
				Curl:  `curl "{{URL}}?title=Parcel arrived&text=Locker A12 downstairs"`,
				Input: service.SendInput{
					Type: models.TypeText, Title: "Parcel arrived", Body: "Locker A12, downstairs.",
				},
			},
			web.LangZH: {
				Title: "带标题",
				Desc:  "标题是通知的第一行，正文在下面。纯 GET 也收，方便那些只会拼 URL 的地方。",
				Curl:  `curl "{{URL}}?title=快递到了&text=放在楼下丰巢 A 区 12 号柜"`,
				Input: service.SendInput{
					Type: models.TypeText, Title: "快递到了", Body: "放在楼下丰巢 A 区 12 号柜。",
				},
			},
		},
	},
	{
		ID:   "markdown",
		Lang: "bash",
		T: map[web.Lang]sampleText{
			web.LangEN: {
				Title: "markdown",
				Desc:  "Tables, lists, quotes and code blocks (with line numbers and highlighting) all render. The body is not part of the push itself — the app fetches it separately — so it can be long.",
				Curl: `curl -H 'Content-Type: application/json' -d '{
  "type": "markdown",
  "title": "This week's spending",
  "body": "**$86** this week.\n\n| Category | Amount |\n|---|---|\n| Food | $43 |\n| Transport | $18 |\n\n> $10 more than last week."
}' {{URL}}`,
				Input: service.SendInput{
					Type:  models.TypeMarkdown,
					Title: "This week's spending",
					Body: "**$86** this week.\n\n" +
						"| Category | Amount | Share |\n|---|---|---|\n" +
						"| Food | $43 | 50% |\n| Transport | $18 | 21% |\n" +
						"| Household | $14 | 16% |\n| Other | $11 | 13% |\n\n" +
						"- $10 more than last week\n- Six takeout orders, two more than last week\n\n" +
						"> A little script at home sends this every Sunday night.",
				},
			},
			web.LangZH: {
				Title: "markdown",
				Desc:  "表格、列表、引用、代码块（带行号和高亮）都渲染。正文不进推送本身，是 app 打开时单独取的，因此可以写得很长。",
				Curl: `curl -H 'Content-Type: application/json' -d '{
  "type": "markdown",
  "title": "本周花销",
  "body": "这周一共花了 **¥612**。\n\n| 分类 | 金额 |\n|---|---|\n| 吃饭 | ¥305 |\n| 交通 | ¥128 |\n\n> 比上周多了 ¥74。"
}' {{URL}}`,
				Input: service.SendInput{
					Type:  models.TypeMarkdown,
					Title: "本周花销",
					Body: "这周一共花了 **¥612**。\n\n" +
						"| 分类 | 金额 | 占比 |\n|---|---|---|\n" +
						"| 吃饭 | ¥305 | 50% |\n| 交通 | ¥128 | 21% |\n" +
						"| 日用 | ¥97 | 16% |\n| 其它 | ¥82 | 13% |\n\n" +
						"- 比上周多了 ¥74\n- 外卖 6 单，比上周多 2 单\n\n" +
						"> 这条是家里的小脚本每周日晚上自动发的。",
				},
			},
		},
	},
	{
		ID:   "list",
		Lang: "bash",
		T: map[web.Lang]sampleText{
			web.LangEN: {
				Title: "Lists and checklists",
				Desc:  "Bulleted, numbered and checkable lists all work — good for “what is still left”.",
				Curl: `curl -H 'Content-Type: application/json' -d '{
  "type": "markdown",
  "title": "Before leaving",
  "body": "- [x] Air conditioning off\n- [x] Balcony door locked\n- [ ] Take the bins out"
}' {{URL}}`,
				Input: service.SendInput{
					Type:  models.TypeMarkdown,
					Title: "Before leaving",
					Body: "- [x] Air conditioning off\n- [x] Balcony door locked\n- [ ] Take the bins out\n\n" +
						"1. Gas off at 07:40\n2. Doors and windows locked at 07:42\n3. Cat bowl filled",
				},
			},
			web.LangZH: {
				Title: "列表 / 清单",
				Desc:  "无序、有序、可勾选的任务清单都支持，适合发「还差哪几件事」。",
				Curl: `curl -H 'Content-Type: application/json' -d '{
  "type": "markdown",
  "title": "出门前检查",
  "body": "- [x] 关空调\n- [x] 锁阳台门\n- [ ] 倒垃圾"
}' {{URL}}`,
				Input: service.SendInput{
					Type:  models.TypeMarkdown,
					Title: "出门前检查",
					Body: "- [x] 关空调\n- [x] 锁阳台门\n- [ ] 倒垃圾\n\n" +
						"1. 燃气 07:40 已关\n2. 门窗 07:42 已锁\n3. 猫粮加满了",
				},
			},
		},
	},
	{
		ID:   "card",
		Lang: "bash",
		T: map[web.Lang]sampleText{
			web.LangEN: {
				Title: "Card",
				Desc:  "An ordered set of key-value pairs — tighter than a table, good for “a few facts about one thing”. style can be ok / warn / error / muted and only colours the value on the right.",
				Curl: `curl -H 'Content-Type: application/json' -d '{
  "type": "card",
  "title": "This month's phone bill is out",
  "items": [
    {"k": "Amount", "v": "$8.30"},
    {"k": "Status", "v": "Paid automatically", "style": "ok"}
  ]
}' {{URL}}`,
				Input: service.SendInput{
					Type:    models.TypeCard,
					Title:   "This month's phone bill is out",
					Summary: "$8.30 · paid automatically · $1.70 less than last month",
					Items: []service.CardItem{
						{K: "Amount", V: "$8.30"},
						{K: "Plan", V: "30 GB"},
						{K: "Payment", V: "Automatic"},
						{K: "Status", V: "Paid", Style: "ok"},
					},
				},
			},
			web.LangZH: {
				Title: "卡片",
				Desc:  "有序的键值对，比表格更紧凑，适合「一件事的几个要点」。style 可以是 ok / warn / error / muted，只影响右边那个值的颜色。",
				Curl: `curl -H 'Content-Type: application/json' -d '{
  "type": "card",
  "title": "这个月话费出账了",
  "items": [
    {"k": "金额", "v": "¥59.00"},
    {"k": "状态", "v": "已自动扣费", "style": "ok"}
  ]
}' {{URL}}`,
				Input: service.SendInput{
					Type:    models.TypeCard,
					Title:   "这个月话费出账了",
					Summary: "¥59.00 · 已自动扣费 · 比上月少 ¥12",
					Items: []service.CardItem{
						{K: "金额", V: "¥59.00"},
						{K: "套餐", V: "畅享 30GB"},
						{K: "扣费方式", V: "自动"},
						{K: "状态", V: "已缴清", Style: "ok"},
					},
				},
			},
		},
	},
	{
		ID:         "image",
		Lang:       "bash",
		NeedsImage: true,
		T: map[web.Lang]sampleText{
			web.LangEN: {
				Title: "With an image",
				Desc:  "The app icon on the notification is replaced by the image itself, and expanding shows it full size. Images are scaled to a long edge of 1600; the original is not kept.",
				Curl:  `curl -F "title=Electricity use was high yesterday" -F "text=14.2 kWh, about 60% above normal" -F "image=@chart.png" {{URL}}`,
				Input: service.SendInput{
					Type: models.TypeImage, Title: "Electricity use was high yesterday",
					Body: "14.2 kWh, about 60% above normal. The air conditioning may have been left on.",
				},
			},
			web.LangZH: {
				Title: "带图",
				Desc:  "通知上那块 app 图标会换成图片本身，展开是大图。图会被等比缩到长边 1600，原图不保留。",
				Curl:  `curl -F "title=昨天家里用电有点高" -F "text=14.2 度，比平时高约 60%" -F "image=@chart.png" {{URL}}`,
				Input: service.SendInput{
					Type: models.TypeImage, Title: "昨天家里用电有点高",
					Body: "14.2 度，比平时高了约 60%，空调可能忘了关。",
				},
			},
		},
	},
	{
		ID:   "link",
		Lang: "bash",
		T: map[web.Lang]sampleText{
			web.LangEN: {
				Title: "Link",
				Desc:  "Long-press the notification to open it directly, without going into the app first.",
				Curl: `curl -H 'Content-Type: application/json' -d '{
  "type": "link",
  "title": "The shoes you were watching dropped",
  "link": "https://example.com/item/8812"
}' {{URL}}`,
				Input: service.SendInput{
					Type: models.TypeLink, Title: "The shoes you were watching dropped",
					Summary: "$126 → $91, four dollars below the previous low",
					Link:    "https://example.com/item/8812",
				},
			},
			web.LangZH: {
				Title: "链接",
				Desc:  "长按通知能直接打开，不用先进 app。",
				Curl: `curl -H 'Content-Type: application/json' -d '{
  "type": "link",
  "title": "你盯的那双鞋降价了",
  "link": "https://example.com/item/8812"
}' {{URL}}`,
				Input: service.SendInput{
					Type: models.TypeLink, Title: "你盯的那双鞋降价了",
					Summary: "¥899 → ¥649，比历史最低还低 ¥30",
					Link:    "https://example.com/item/8812",
				},
			},
		},
	},
	{
		ID:   "collapse",
		Lang: "bash",
		T: map[web.Lang]sampleText{
			web.LangEN: {
				Title: "A notification that updates itself",
				Desc:  "A new notification carrying the same collapse_id replaces the old one, so the notification centre only ever holds the latest — good for progress and status. Press this button a few times to see it. There is also idem_key for idempotency: a retry carrying the same value does not push twice.",
				Curl: `curl -H 'Content-Type: application/json' -d '{
  "title": "Parcel out for delivery",
  "body": "Three stops away",
  "collapse_id": "sf-1234567890"
}' {{URL}}`,
				Input: service.SendInput{
					Type: models.TypeText, Title: "Parcel out for delivery", Body: "Three stops away",
					// 刻意不带 idem_key：带了之后按第二次就被幂等掉，看起来像按钮坏了。
					// 折叠留着——反复按能看到通知互相顶掉，那正是这一条要演示的。
					CollapseID: "kb-demo-delivery",
				},
			},
			web.LangZH: {
				Title: "会自己更新的通知",
				Desc:  "带同一个 collapse_id 的新通知会顶掉旧的，通知栏里始终只有最新那条，适合进度和状态刷新。连按几次这颗按钮就能看出来。另有 idem_key 用于幂等：上游重试时带同一个值不会推第二遍。",
				Curl: `curl -H 'Content-Type: application/json' -d '{
  "title": "快递派送中",
  "body": "还有 3 站到你家",
  "collapse_id": "sf-1234567890"
}' {{URL}}`,
				Input: service.SendInput{
					Type: models.TypeText, Title: "快递派送中", Body: "还有 3 站到你家",
					CollapseID: "kb-demo-delivery",
				},
			},
		},
	},
}

func sampleByID(id string) *sample {
	for i := range samples {
		if samples[i].ID == id {
			return &samples[i]
		}
	}
	return nil
}

// viewSamples 转成只管展示的形状交给 web 层：它不该知道 service.SendInput。
func viewSamples(url string, live bool, lang web.Lang) []web.Sample {
	out := make([]web.Sample, 0, len(samples))
	for _, s := range samples {
		t := s.text(lang)
		out = append(out, web.Sample{
			ID: s.ID, Title: t.Title, Desc: t.Desc, Lang: s.Lang,
			Curl: strings.ReplaceAll(t.Curl, "{{URL}}", url),
			Live: live,
		})
	}
	return out
}

// sampleChart 「带图」那条示例现做的图。
//
// 现画而不是塞一个资源文件：这张图只在按下按钮那一刻用一次，
// 为它在仓库和镜像里多放一个 PNG 不值得，而且改尺寸还得重新导出。
func sampleChart() []byte {
	const w, h = 900, 520
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{0x14, 0x16, 0x1C, 0xFF}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	bar := color.RGBA{0x4F, 0x63, 0xEE, 0xFF}
	hot := color.RGBA{0xE5, 0x48, 0x4D, 0xFF}
	line := color.RGBA{0x2C, 0x31, 0x3D, 0xFF}

	vals := []float64{0.42, 0.38, 0.45, 0.51, 0.58, 0.74, 0.92}
	const left, right, top, bottom = 70, w - 50, 70, h - 70
	// 横向基准线
	for _, f := range []float64{0.25, 0.5, 0.75, 1} {
		y := bottom - int(float64(bottom-top)*f)
		for x := left; x < right; x++ {
			img.Set(x, y, line)
		}
	}
	slot := (right - left) / len(vals)
	bw := slot * 6 / 10
	for i, v := range vals {
		x0 := left + i*slot + (slot-bw)/2
		y0 := bottom - int(float64(bottom-top)*v)
		c := bar
		if v > 0.85 {
			c = hot
		}
		draw.Draw(img, image.Rect(x0, y0, x0+bw, bottom), &image.Uniform{c}, image.Point{}, draw.Src)
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
