# Knockbox

**English** · [简体中文](README.zh-CN.md)

[![CI](https://github.com/aichy126/knockbox/actions/workflows/ci.yml/badge.svg)](https://github.com/aichy126/knockbox/actions/workflows/ci.yml)
[![Licence: MIT](https://img.shields.io/badge/licence-MIT-blue.svg)](LICENSE)

Self-hosted push notification server for iOS. Send a message with one `curl`, get it on your phone.

Knockbox is a single Go binary with a single SQLite file. It stores your messages,
pushes them straight to Apple's servers, and serves the history to the
[iOS app](https://apps.apple.com/app/id6811317906). There is no relay in between
and no account to sign up for — the server is yours.

### Quick start

```bash
docker run -d --name knockbox -p 8080:8080 -v knockbox-data:/app/data \
  -e IGO_SERVER_EXTERNAL_URL=http://localhost:8080 \
  ghcr.io/aichy126/knockbox
docker logs knockbox
```

The first start creates the database and an admin account, and prints that account's
username and password in the log — once, and nowhere else. Open
<http://localhost:8080/login>, sign in with it, and the pairing page shows a QR code
to scan with the iOS app. You can change that password on the
settings page of the same admin interface.

**Set `IGO_SERVER_EXTERNAL_URL` to the address your phone will actually reach**, for
example `https://push.example.com`. It goes into the pairing links and the attachment
URLs, so `localhost` works only while you are testing on the same machine.

Images are published to both `ghcr.io/aichy126/knockbox` and `aichy66/knockbox` on
Docker Hub, for `linux/amd64` and `linux/arm64`. Tagged releases also publish prebuilt
binaries, and `make build` produces one from source (needs Go 1.26+).

Then send something:

```bash
curl -d "build failed" https://push.example.com/api/v1/send/<channel-token>
```

### Sending

Four content types are accepted, and every scalar can also come from the query
string, so `curl` works however you write it:

```bash
# plain body
curl -d "disk at 92%" .../api/v1/send/<token>
# GET, for things that cannot POST
curl ".../api/v1/send/<token>?title=CI&text=build+failed"
# multipart, with an image
curl -F "title=Disk alert" -F "text=/data at 92%" -F "image=@chart.png" .../api/v1/send/<token>
# JSON, for everything else
curl -H 'Content-Type: application/json' -d '{
  "type": "markdown",
  "title": "Build #1832 failed",
  "body": "**main** · 42s\n\n```go\nfunc Execute() {}\n```",
  "idem_key": "drone-build-1832"
}' .../api/v1/send/<token>
```

`POST /api/v1/send/batch` takes a list of tokens and reports per-token results, so
one upstream call can reach many people without a broadcast credential.

### Asking for an answer

A message can ask for an answer: buttons to tap, checkboxes to pick several, a slider for a
number, or a field to type in. The person opens the notification, answers in the app, and the
server POSTs their answer to a URL you provide.

```bash
curl -H 'Content-Type: application/json' -d '{
  "title": "Curtains open in 30 seconds",
  "reply": {
    "type": "choice",
    "options": ["Open", "Keep closed"],
    "timeout": 30,
    "webhook": "https://home.example.com/api/webhook/curtain"
  }
}' .../api/v1/send/<token>
```

Once they answer, your endpoint receives:

```json
{"uid":"01J…","channel":"…","title":"Curtains open in 30 seconds","reply":"Keep closed","replied_at":1726…}
```

Signed with `X-Knockbox-Signature: sha256=<HMAC-SHA256(body, channel token)>` so you can verify
where it came from. The other types are `multi` (checkboxes, several picked then submitted),
`number` (`min`, `max`, optional `step` and `unit`) and `text` (a field they type in); `reply`
comes back as a string, an array of strings for `multi`, a number for `number`.
`timeout` is optional — leave
it out and the message stays answerable indefinitely; set it only when something happens by
itself once the time is up. **`webhook` is required**: an answer with nowhere to go is worse
than an error at send time, because someone will have tapped that button for nothing.

The flat form works too, so a one-line `curl` can ask as well:

```bash
curl ".../api/v1/send/<token>?title=Deploy?&choices=Deploy,Hold&reply_webhook=https://…"
```

A message is answered once, first answer wins; other devices see that same answer through sync.
The answer does **not** come back on the send connection — that one ends when the message is
stored, and the person may look at their phone minutes later.

A public instance refuses callbacks to private addresses by default — that is an SSRF surface.
A self-hosted one allows them, because the server sits inside your own network and those are
exactly the addresses it needs to reach. See `[reply]` in `config.toml`.

### Handing it to an AI agent

The same send credential also works as an [MCP](https://modelcontextprotocol.io) endpoint
over streamable HTTP, exposing a single tool that sends one notification.

```bash
claude mcp add --transport http knockbox https://your.server/mcp/<token>
```

Clients that take a config file want this:

```json
{
  "mcpServers": {
    "knockbox": {
      "type": "http",
      "url": "https://your.server/mcp/<token>"
    }
  }
}
```

An assistant that configures itself only needs the sentence "add an MCP server, Streamable HTTP,
at this address" plus the address. All three forms sit on `/docs` and on a channel's send page
with the address already filled in.

It is the same path as `/api/v1/send` — same permission, same rate limit, same quota — so an
agent that can run commands gains nothing by switching. What it buys is three things: clients
without a shell can connect; a body with newlines and quotes never goes through shell quoting;
and the tool sits in the agent's tool list, so nothing has to be pasted in first.

### How notifications behave

A channel is an inbox line, like a group chat bot. Its **sound, interruption level
and mute flag live on the server** because they are written into the APNs payload
at push time — the app's notification extension can be skipped by the system, and
when it is, the payload's own values are what the phone uses. Everything else about
a channel (name, icon, colour) is an opaque blob the server stores and never reads.

Message bodies are **not** in the payload. The payload carries a summary and an id;
the body stays in the database and the app fetches it. That is why a body can be far
longer than a notification allows (up to `limit.body_max_kb`, 1 MB by default), and
why a push can always be sent — the payload degrades until it fits.

### Design trade-offs you should know about

These are choices, not omissions. Read them before you deploy.

**No end-to-end encryption.** There is no relay, so messages only ever touch your
own server. That server also has to store the history and generate push summaries,
both of which need the plaintext. E2E here would mainly protect you from your own
server administrator, who is you.

**The server keeps everything in plaintext, forever by default.** Full archive is the
point — new devices pull the whole history. Set `retention.days` if you want an
instance-wide limit; a public instance can additionally cap each member with
`public.retention_days`, and the stricter of the two applies. The security boundary
is that machine; back it up and lock it down accordingly.

**A public APNs key is embedded in this repository.** APNs device tokens are bound to
a bundle id, so only the key belonging to that app's team can push to the App Store
build of the iOS app. To let you self-host without a $99/year Apple developer account,
this repo ships a key that is **topic-specific** — it can only push to
`com.miramiao.knockbox` and nothing else.

The honest consequence: anyone who also obtains a device's APNs token could push
arbitrary content to that device. Those tokens sit on the device and in your own
database, in plaintext, so a database leak is the realistic path — and with the key
already public, nothing else stands in the way. It is a real downgrade. Two ways out:

- Point `apns.production.key_file` at your own key and change `apns.topic` — then
  build and sign the iOS app yourself.
- If the shared key is ever abused and Apple rate-limits it, it will be rotated and
  **every self-hosted instance will need to upgrade to keep working.** A rotation is
  announced as a GitHub Security Advisory here and in the release notes of the version
  that carries the new key, so use **Watch → Custom → Releases and Security advisories**
  if you run this for anyone but yourself.

What the key can and cannot reach, and how to report a problem, are in
[SECURITY.md](SECURITY.md).

**The admin interface is in Chinese.** The pages a stranger reaches — the pairing
page, the sending guide and their error pages — are bilingual and default to English.
`/login` and `/admin/*` are not: they are read by whoever runs the server, and the
person maintaining them reviews in Chinese. Nothing is locked behind them — every
action there is also available from the CLI and the HTTP API, both of which are
English.

**`/admin/api/*` is not a public contract.** `/api/v1` is: sending, and the endpoints
the iOS app uses. Everything under `/admin/api/` exists to serve the admin interface
that ships in this repository, shares its session cookie, and will change shape
whenever that interface does. Do not build against it — if you want to script this
server, `/api/v1` and the CLI are the supported ways, and both are documented above.

**Channel tokens are stored in plaintext.** The app shows you a ready-to-paste `curl`
line, which hashing would make impossible. They are write-only credentials scoped to
one channel, `last_used_at`/`ip` are recorded, and a channel can be rotated or deleted
at any time. Every token is also rate-limited (`limit.send_qps` / `send_burst`,
20/s with a burst of 60 by default), so a leaked one cannot be used to flood the
database — that limit applies to self-hosted instances too, where there is no quota.

### Deleting is real deleting

Clearing a channel physically removes the rows. SQLite's `DELETE` leaves content in
the file until it is overwritten, so the server also runs with `secure_delete`, does a
`wal_checkpoint(TRUNCATE)` and an `incremental_vacuum` afterwards. Attachments are
reference-counted, and the blob is removed when the last message referencing it is gone.

What this does **not** promise: an `os.Remove` cannot guarantee the bytes are gone from
an SSD, because wear levelling is below the application layer.

### CLI

Everything is a subcommand with `-h`; no operation requires touching the database.

```
knockbox serve                 start the server
knockbox user add <name>       create an admin account (prompts for the password)
knockbox user passwd <name>    reset a password
knockbox user list             list accounts
knockbox pair                  issue a pairing code and print it as a QR code
```

Passwords are read interactively by default, and also accept a pipe
(`echo 's3cret' | knockbox user passwd admin`). Passing one as a flag is
possible but leaves it in `ps`.

### Configuration

Copy `config.toml.example` and edit. Every key can be overridden by an environment
variable (`server.external_url` → `IGO_SERVER_EXTERNAL_URL`), which is the intended
way to pass secrets such as `IGO_APNS_PRODUCTION_KEY_BASE64`.

### Licence

MIT. See [LICENSE](LICENSE).
