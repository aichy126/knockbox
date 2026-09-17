package web

// Style 后台样式。**直接从设计稿 docs/design/knockbox-admin/_style.css 搬过来**，
// 改动只有三处：画板固定尺寸改自适应、字体换成系统字体栈（中文要能正常显示，
// 也少一次外部请求）、以及补上实装才需要的那些类（hover、分页、空状态等）。
//
// 服务端直出 HTML，没有前端构建步骤——这是个自建工具，不该为了一个管理页
// 引入 node_modules。
const Style = `*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",Roboto,sans-serif;-webkit-font-smoothing:antialiased}
a{color:var(--primary,oklch(0.58 0.19 262));text-decoration:none}
a:hover{color:color-mix(in oklch,var(--primary,oklch(0.58 0.19 262)) 78%,var(--fg,#000))}
/* 令牌定义在 :root：登录页和配对页不走 Shell、根节点上没有 .app，
   令牌挂在 .app 里的话那两页拿到的全是未定义变量——表现为输入框和边框整个消失。 */
:root{--bg:oklch(1 0 0);--fg:oklch(0.141 0.005 285.823);--card:oklch(1 0 0);--muted:oklch(0.967 0.001 286.375);--muted-fg:oklch(0.552 0.016 285.938);--border:oklch(0.92 0.004 286.32);--primary:oklch(0.58 0.19 262);--primary-fg:oklch(0.985 0 0);--accent-soft:oklch(0.95 0.03 262);--success:oklch(0.63 0.17 148);--success-soft:oklch(0.97 0.03 150);--warning:oklch(0.68 0.16 60);--warning-soft:oklch(0.97 0.03 80);--danger:oklch(0.58 0.22 27);--danger-soft:oklch(0.96 0.03 20);background:var(--bg);color:var(--fg);font-size:14px;line-height:1.45}
.app{display:flex;min-height:100vh;background:var(--bg);color:var(--fg)}
:root[data-theme="dark"]{--bg:oklch(0.141 0.005 285.823);--fg:oklch(0.985 0 0);--card:oklch(0.21 0.006 285.885);--muted:oklch(0.274 0.006 286.033);--muted-fg:oklch(0.705 0.015 286.067);--border:oklch(1 0 0 / 12%);--primary:oklch(0.66 0.17 262);--primary-fg:oklch(0.13 0.02 262);--accent-soft:oklch(0.28 0.08 262);--success:oklch(0.8 0.18 150);--success-soft:oklch(0.25 0.05 150);--warning:oklch(0.85 0.16 85);--warning-soft:oklch(0.3 0.06 70);--danger:oklch(0.72 0.18 25);--danger-soft:oklch(0.3 0.08 20)}
.sidebar{width:240px;flex-shrink:0;display:flex;flex-direction:column;border-right:1px solid var(--border);background:var(--card)}
.brand{height:56px;display:flex;align-items:center;gap:10px;padding:0 20px;border-bottom:1px solid var(--border)}
.brand-mark{width:28px;height:28px;border-radius:10px;background:var(--primary);color:var(--primary-fg);display:flex;align-items:center;justify-content:center;flex-shrink:0}
.brand-name{font-size:15px;font-weight:600;letter-spacing:-0.01em}
.nav{display:flex;flex-direction:column;gap:2px;padding:12px}
.nav .item{display:flex;align-items:center;gap:10px;height:36px;padding:0 12px;border-radius:10px;font-size:13.5px;font-weight:500;color:var(--muted-fg)}
.nav .item.on{background:var(--accent-soft);color:var(--primary);font-weight:600}
.side-foot{margin-top:auto;display:flex;align-items:center;gap:10px;border-top:1px solid var(--border);padding:14px 20px;font-size:12px;color:var(--muted-fg)}
.dot{width:8px;height:8px;border-radius:999px;flex-shrink:0}
.main{flex:1;min-width:0;display:flex;flex-direction:column}
.topbar{height:56px;flex-shrink:0;display:flex;align-items:center;gap:12px;border-bottom:1px solid var(--border);background:var(--card);padding:0 24px}
.crumb{display:flex;align-items:center;gap:6px;font-size:13.5px;color:var(--muted-fg)}
.crumb .cur{font-weight:600;color:var(--fg)}
.spacer{flex:1}
.content{flex:1;min-height:0;padding:24px 28px;display:flex;flex-direction:column;gap:20px}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:6px;height:32px;padding:0 10px;border-radius:10px;border:1px solid transparent;font-size:14px;font-weight:500;background:var(--primary);color:var(--primary-fg);white-space:nowrap}
.btn.outline{background:var(--bg);border-color:var(--border);color:var(--fg)}
.btn.ghost{background:transparent;color:var(--muted-fg)}
.btn.sm{height:28px;padding:0 10px;font-size:12.8px;border-radius:8px}
.btn.icon{width:34px;height:34px;padding:0}
.card{background:var(--card);border-radius:14px;box-shadow:0 0 0 1px color-mix(in oklch,var(--fg) 10%,transparent);display:flex;flex-direction:column;min-width:0}
.card-h{display:flex;align-items:center;gap:10px;padding:16px 16px 10px}
.card-h .t{font-weight:600}
.card-b{padding:0 16px 16px}
.badge{display:inline-flex;align-items:center;gap:6px;height:22px;padding:0 8px;border-radius:8px;font-size:12px;font-weight:600;white-space:nowrap}
.badge i{width:6px;height:6px;border-radius:999px;background:currentColor;display:block;flex-shrink:0}
.badge.ok{background:var(--success-soft);color:var(--success)}
.badge.info{background:var(--accent-soft);color:var(--primary)}
.badge.err{background:var(--danger-soft);color:var(--danger)}
.badge.warn{background:var(--warning-soft);color:var(--warning)}
.badge.muted{background:var(--muted);color:var(--muted-fg)}
table{width:100%;border-collapse:collapse;font-size:14px}
th{height:40px;padding:0 8px;text-align:left;font-weight:500;white-space:nowrap}
td{padding:8px;vertical-align:middle;white-space:nowrap}
thead tr,tbody tr{border-bottom:1px solid var(--border)}
tbody tr:last-child{border-bottom:0}
.mono{font-family:"JetBrains Mono",Menlo,Consolas,monospace;font-size:12.5px}
.dim{color:var(--muted-fg)}
.num{font-variant-numeric:tabular-nums}
.ph{display:flex;align-items:flex-start;gap:12px}
.ph h1{margin:0;font-size:20px;font-weight:600;letter-spacing:-0.01em}
.ph .sub{margin-top:4px;font-size:13px;color:var(--muted-fg)}
.stat{padding:16px 20px}
.stat .l{font-size:12.5px;font-weight:600;color:var(--muted-fg)}
.stat .v{margin-top:6px;font-size:28px;font-weight:600;letter-spacing:-0.02em;line-height:1.1}
.stat .b{margin-top:8px}
/* .note 是 flex（为了让图标与文字顶对齐），因此文字必须整段包在一个元素里。
   直接塞行内 <code>/<span> 会让它成为独立的 flex item，一句话被切成几块各自换行。 */
.note{display:flex;gap:8px;align-items:flex-start;padding:10px 12px;border-radius:10px;background:var(--muted);font-size:12.5px;color:var(--muted-fg);line-height:1.55}
.note>*{min-width:0}
.note p{margin:0}
.note b{color:var(--fg);font-weight:600}
.field{display:flex;flex-direction:column;gap:6px}
.field label{font-size:12.5px;font-weight:600;color:var(--muted-fg)}
.input{height:36px;border-radius:10px;border:1px solid var(--border);background:var(--bg);padding:0 12px;display:flex;align-items:center;font-size:14px;color:var(--fg)}
.input.ph-text{color:var(--muted-fg)}
.seg{display:inline-flex;gap:2px;padding:3px;border-radius:10px;background:var(--muted)}
.seg span{display:inline-flex;align-items:center;height:26px;padding:0 10px;border-radius:8px;font-size:12.8px;font-weight:500;color:var(--muted-fg)}
.seg span.on{background:var(--card);color:var(--fg);font-weight:600;box-shadow:0 1px 2px rgba(0,0,0,.06)}
sc-for{display:contents}
.tabline{display:flex;gap:18px;border-bottom:1px solid var(--border);padding:0 16px}
.tabline span{padding:10px 0;font-size:13.5px;font-weight:500;color:var(--muted-fg);border-bottom:2px solid transparent;margin-bottom:-1px}
.tabline span.on{color:var(--fg);font-weight:600;border-bottom-color:var(--primary)}
.app{position:relative}
.app.tall{height:1040px}
.toggle{width:34px;height:20px;border-radius:999px;background:var(--muted);position:relative;display:inline-block;flex-shrink:0}
.toggle::after{content:"";position:absolute;top:2px;left:2px;width:16px;height:16px;border-radius:999px;background:var(--card);box-shadow:0 1px 2px rgba(0,0,0,.15)}
.toggle.on{background:var(--primary)}
.toggle.on::after{left:16px}
.chan{display:inline-flex;align-items:center;gap:8px;font-weight:500}
.chan i{width:22px;height:22px;border-radius:7px;display:inline-flex;align-items:center;justify-content:center;flex-shrink:0}
.kv{display:grid;grid-template-columns:112px 1fr;gap:9px 14px;font-size:13.5px;align-items:center}
.kv .k{color:var(--muted-fg);font-weight:500}
.kv .v{display:flex;align-items:center;gap:8px;min-width:0}
.code{font-family:"JetBrains Mono",Menlo,Consolas,monospace;font-size:12.5px;line-height:1.65;background:var(--muted);border-radius:10px;padding:12px 14px;white-space:pre;color:var(--fg);overflow:hidden}
.code .c{color:var(--muted-fg)}
.btn.danger{background:var(--danger);color:#fff}
.btn.outline.danger{background:var(--bg);border-color:color-mix(in oklch,var(--danger) 40%,transparent);color:var(--danger)}
.scrim{position:absolute;inset:0;background:oklch(0 0 0 / 42%);display:flex;align-items:center;justify-content:center}
.modal{width:480px;background:var(--card);border-radius:16px;box-shadow:0 24px 64px rgba(0,0,0,.28),0 0 0 1px color-mix(in oklch,var(--fg) 10%,transparent);padding:20px 22px;display:flex;flex-direction:column;gap:14px}
.modal h2{margin:0;font-size:16px;font-weight:600}
.steps{display:flex;flex-direction:column;gap:12px}
.step{display:flex;gap:12px;align-items:flex-start;font-size:13.5px;line-height:1.5}
.step b{width:22px;height:22px;border-radius:999px;background:var(--accent-soft);color:var(--primary);display:inline-flex;align-items:center;justify-content:center;font-size:12px;font-weight:600;flex-shrink:0}
tr.tomb td{color:var(--muted-fg)}
.md h3{margin:0 0 10px;font-size:16px;font-weight:600}
.md p{margin:0 0 10px;font-size:14px;line-height:1.65}
.md ul{margin:0 0 10px;padding-left:20px;font-size:14px;line-height:1.65}
.md .code{margin:10px 0}
.qr{width:280px;height:280px;border-radius:14px;background:oklch(1 0 0);color:oklch(0.141 0.005 285.823);display:flex;align-items:center;justify-content:center;box-shadow:0 0 0 1px color-mix(in oklch,var(--fg) 10%,transparent)}
.bigcode{font-family:"JetBrains Mono",Menlo,Consolas,monospace;font-size:30px;font-weight:500;letter-spacing:.14em;line-height:1}
.pager{display:flex;align-items:center;gap:8px;padding:10px 16px;border-top:1px solid var(--border);font-size:12.5px;color:var(--muted-fg)}
/* ── 这些是稿子里没有、实装才需要的 ────────────────────────── */
html{color-scheme:light dark}
.app{margin:0 auto}
@media (prefers-color-scheme:dark){:root:not([data-theme="light"]){--bg:oklch(0.141 0.005 285.823);--fg:oklch(0.985 0 0);--card:oklch(0.21 0.006 285.885);--muted:oklch(0.274 0.006 286.033);--muted-fg:oklch(0.705 0.015 286.067);--border:oklch(1 0 0 / 12%);--primary:oklch(0.66 0.17 262);--primary-fg:oklch(0.13 0.02 262);--accent-soft:oklch(0.28 0.08 262);--success:oklch(0.8 0.18 150);--success-soft:oklch(0.25 0.05 150);--warning:oklch(0.85 0.16 85);--warning-soft:oklch(0.3 0.06 70);--danger:oklch(0.72 0.18 25);--danger-soft:oklch(0.3 0.08 20)}}
a.item,a.btn{text-decoration:none}
a.item:hover{background:var(--muted);color:var(--fg)}
tbody tr:hover{background:var(--muted)}
.empty{padding:48px 16px;text-align:center;color:var(--muted-fg);font-size:13.5px}
.tools{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.tools input,.tools select{height:32px;border-radius:10px;border:1px solid var(--border);background:var(--bg);color:var(--fg);padding:0 10px;font-size:13.5px;font-family:inherit}
.pager{display:flex;gap:8px;align-items:center;padding:12px 16px;border-top:1px solid var(--border)}
form.inline{display:inline}
button.btn{cursor:pointer;font-family:inherit}
.wrap-any{word-break:break-all}
pre.body{margin:0;background:var(--bg);color:var(--fg);white-space:pre-wrap;word-break:break-word;font-family:ui-monospace,"JetBrains Mono",Menlo,monospace;font-size:12.5px;line-height:1.6;background:var(--muted);border-radius:10px;padding:12px}
@media (max-width:900px){.sidebar{display:none}.content{padding:16px}}

/* ── 后台：整屏布局 + 内部滚动（像聊天窗，不是整页滚） ────────── */
.app.fixed{height:100vh;overflow:hidden}
.content.scroll{overflow-y:auto}
.content.flat{overflow:hidden;padding-bottom:0}
.stream{flex:1;min-height:0;overflow-y:auto;display:flex;flex-direction:column;gap:10px;padding-bottom:20px}
.ph{display:flex;align-items:flex-start;gap:12px;flex-shrink:0}
.ph h1{font-size:24px;margin:0 0 2px;letter-spacing:-0.02em}

/* ── 弹出式详情（<details> 当按钮用，不写 JS） ──────────────── */
.pop{position:relative}
.pop>summary{list-style:none;cursor:pointer}
.pop>summary::-webkit-details-marker{display:none}
.pop[open]>summary{background:var(--muted)}
.pop-body{position:absolute;top:calc(100% + 6px);right:0;z-index:20;width:min(460px,80vw);
  background:var(--card);border:1px solid var(--border);border-radius:12px;padding:14px 16px;
  box-shadow:0 16px 40px rgb(0 0 0 / 28%)}

/* ── 可搜索的选择器（datalist，不引任何组件库） ──────────────── */
/* 成员可能有几百个，下拉框会变成一条滚不完的列表。
   <input list=...> 既能打字过滤又能点开看全部，浏览器原生支持，零 JS。 */
.picker{position:relative;display:inline-flex;align-items:center;gap:6px}
.picker input{min-width:190px}
.picker .hint{font-size:12px;color:var(--muted-fg)}

/* ── 设置页 ──────────────────────────────────────────────── */
/* 表单不该拉到 1200px 宽：标签在最左、控件在最右，中间一大片空白，
   眼睛要横扫整个屏幕才能把一行读完。 */
.narrow{max-width:720px;width:100%}
.set-row{display:flex;align-items:center;gap:16px;padding:13px 16px;border-bottom:1px solid var(--border)}
.set-row:last-child{border-bottom:0}
.set-row .lab{flex:1;min-width:0}
.set-row .lab b{font-weight:500;font-size:14px}
.set-row .lab span{display:block;font-size:12px;color:var(--muted-fg);margin-top:2px}
.set-row .ctl{flex-shrink:0;display:flex;align-items:center;gap:8px}
.set-row input[type=number]{width:96px;text-align:right}
.set-row input[type=text]{width:190px}
/* 改过的项打一个点就够了，不必每行挂一个徽标——6 个徽标是噪音，
   而「哪几项被改过」这个信息本身很少被需要。 */
.dot-set{width:6px;height:6px;border-radius:3px;background:var(--primary);flex-shrink:0}
/* 纯 CSS 开关：系统复选框在深色下又小又难按 */
.sw{position:relative;display:inline-block;width:44px;height:26px;flex-shrink:0}
.sw input{opacity:0;width:0;height:0;position:absolute}
.sw i{position:absolute;inset:0;background:var(--muted);border-radius:999px;transition:.18s;cursor:pointer}
.sw i::after{content:"";position:absolute;left:3px;top:3px;width:20px;height:20px;border-radius:999px;
  background:#fff;transition:.18s;box-shadow:0 1px 3px rgb(0 0 0 / 30%)}
.sw input:checked+i{background:var(--primary)}
.sw input:checked+i::after{transform:translateX(18px)}
.sw input:focus-visible+i{outline:2px solid var(--primary);outline-offset:2px}
.card-foot{padding:12px 16px;border-top:1px solid var(--border);display:flex;align-items:center;gap:10px}
.kv-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:1px;background:var(--border)}
.kv-grid>div{background:var(--card);padding:14px 16px}
.kv-grid .k{font-size:12px;color:var(--muted-fg)}
.kv-grid .v{font-size:20px;font-weight:600;margin-top:3px;letter-spacing:-0.01em}

/* ── 页内 tab ────────────────────────────────────────────── */
.tabs{display:flex;gap:2px;padding:3px;border-radius:12px;background:var(--muted);
  align-self:flex-start;flex-shrink:0}
.tabs a{padding:7px 14px;border-radius:9px;font-size:13.5px;font-weight:500;
  color:var(--muted-fg);text-decoration:none;white-space:nowrap}
.tabs a.on{background:var(--card);color:var(--fg);font-weight:600;
  box-shadow:0 1px 2px rgb(0 0 0 / 18%)}

/* ── markdown ────────────────────────────────────────────── */
.md{font-size:14px;line-height:1.7}
.md>:first-child{margin-top:0}
.md>:last-child{margin-bottom:0}
.md p{margin:0 0 8px}
.md h1,.md h2,.md h3{font-size:16px;font-weight:600;margin:14px 0 6px;letter-spacing:-0.01em}
.md ul,.md ol{margin:0 0 8px;padding-left:22px}
.md li{margin:2px 0}
.md code{font-family:ui-monospace,Menlo,monospace;font-size:12.5px;background:var(--muted);
  padding:1px 5px;border-radius:5px}
.md pre{background:var(--muted);border-radius:10px;padding:12px 14px;overflow-x:auto;margin:0 0 8px}
.md pre code{background:transparent;padding:0;font-size:12.5px;line-height:1.6}
.md table{border-collapse:collapse;margin:0 0 8px;font-size:13px;width:auto;min-width:50%}
.md th,.md td{border:1px solid var(--border);padding:6px 10px;text-align:left}
.md th{background:var(--muted);font-weight:600}
.md blockquote{margin:0 0 8px;padding-left:12px;border-left:3px solid var(--border);color:var(--muted-fg)}
.md img{max-width:100%;border-radius:8px}
.md a{word-break:break-all}

/* ── 登录页与配对页（服务端直出的两张独立页，不走 Shell） ────────── */
.wrap{max-width:1040px;margin:0 auto;padding:44px 24px;display:flex;flex-direction:column;gap:18px}
.wrap.login{max-width:360px;padding-top:12vh;gap:0}
/* 品牌区不是导航条，别继承侧栏 .brand 那套高度和下边框 */
.wrap.login .brand,.wrap .brand{border:0;height:auto;padding:0;margin-bottom:20px;gap:12px}
/* 卡片要有内边距。没有的话标签贴着边、和输入框的底色分成两条带，一眼就丑 */
.wrap .card{padding:20px}
.wrap.login .card{gap:0}
.sub{color:var(--muted-fg);font-size:13.5px;line-height:1.6}
.brand-sub{color:var(--muted-fg);font-size:12px}
.field{display:flex;flex-direction:column;gap:7px;margin-bottom:16px}
.field label{font-size:12.5px;font-weight:600;color:var(--muted-fg)}
.field input{height:38px;border-radius:10px;border:1px solid var(--border);background:var(--bg);
  color:var(--fg);padding:0 12px;font-size:15px;font-family:inherit;width:100%}
.field input:focus{outline:2px solid var(--primary);outline-offset:-1px}
.btn.primary{width:100%;height:42px;font-size:15px;margin-top:2px}
.err{background:var(--danger-soft);color:var(--danger);border-radius:10px;
  padding:10px 12px;font-size:13.5px;margin-bottom:14px}
.cols{display:grid;grid-template-columns:320px 1fr;gap:20px;align-items:start}
@media (max-width:820px){.cols{grid-template-columns:1fr}}
.qr{text-align:center;padding:22px}
.qr svg{width:248px;height:248px;border-radius:12px;display:block;margin:0 auto}
.host{font-family:ui-monospace,Menlo,monospace;font-size:12.5px;color:var(--muted-fg)}
.steps{display:flex;flex-direction:column;gap:12px;margin:0;padding:0;list-style:none}
.steps li{display:flex;gap:10px;align-items:flex-start;font-size:14px;line-height:1.6}
/* 必须用直接子元素选择器。写成 .steps li b 的话，步骤文字里的任何一个 <b>
   都会被当成序号徽标，变成 22px 的圆形 flex item 把句子挤断。 */
.steps li>b:first-child{flex-shrink:0;width:22px;height:22px;border-radius:999px;background:var(--accent-soft);
  color:var(--primary);font-size:12px;display:flex;align-items:center;justify-content:center;margin-top:1px}
.steps li div b{font-weight:600}
.row{display:flex;align-items:center;gap:10px}
`

// Mark 品牌标：一个盒子 + 两道敲击的声波。
const Mark = `<svg viewBox="0 0 64 64" fill="none" width="20" height="20" aria-hidden="true">` +
	`<rect x="9" y="24" width="36" height="30" rx="7" stroke="currentColor" stroke-width="6"/>` +
	`<path d="M46 14a10 10 0 0 1 8 8" stroke="currentColor" stroke-width="5" stroke-linecap="round"/>` +
	`<path d="M50 5a19 19 0 0 1 12 12" stroke="currentColor" stroke-width="5" stroke-linecap="round"/></svg>`
