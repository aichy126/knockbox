# Sending and integrating

**English** · [简体中文](api.zh-CN.md)

Everything a script, a CI job or an agent needs to push a message to a phone.
For installing and running the server, see [deploy.md](deploy.md).

- [Before you start](#before-you-start)
- [Sending](#sending)
- [Message fields](#message-fields)
- [Attachments](#attachments)
- [Asking for an answer](#asking-for-an-answer)
- [Sending to many channels at once](#sending-to-many-channels-at-once)
- [Handing it to an AI agent](#handing-it-to-an-ai-agent)
- [Rate limits and quotas](#rate-limits-and-quotas)
- [The CLI](#the-cli)
- [What is a public contract](#what-is-a-public-contract)
- [Endpoint reference](#endpoint-reference)

## Before you start

A **channel** is an inbox line, like a group chat bot. Each one has its own
**send address**, and that address is the credential — anyone holding it can send to
that channel and nothing else. Create channels in the iOS app; each channel's page
shows a ready-to-paste `curl` line.

```
https://push.example.com/api/v1/send/<channel-token>
```

`push.example.com` is whatever you set `server.external_url` to. Every example below
uses that address; replace it with yours.

A channel's **sound, interruption level and mute switch live on the server**, not in
the message. A sender is a script — it cannot know what counts as urgent to you. If
some messages should wake you at night and others should not, make two channels and
send to different addresses.

## Sending

Four content types are accepted, and every scalar field can also come from the query
string, so `curl` works however you write it.

**The whole body is the message.** Nothing to configure, arrives as plain text:

```bash
curl -d "disk at 92%" https://push.example.com/api/v1/send/<token>
```

**Query string**, for things that cannot POST a body:

```bash
curl "https://push.example.com/api/v1/send/<token>?title=CI&text=build+failed"
```

**Multipart**, when there is a file to go with it:

```bash
curl -F "title=Disk alert" -F "text=/data at 92%" -F "image=@chart.png" \
  https://push.example.com/api/v1/send/<token>
```

**JSON**, for everything else:

```bash
curl -H 'Content-Type: application/json' -d '{
  "type": "markdown",
  "title": "Build #1832 failed",
  "body": "**main** · 42s\n\n```go\nfunc Execute() {}\n```",
  "idem_key": "drone-build-1832"
}' https://push.example.com/api/v1/send/<token>
```

A successful send returns:

```json
{"code":0,"msg":"success","data":{"uid":"01J…","rev":184,"devices":2,"queued":true}}
```

`devices` is how many of your devices the push was queued for, `queued` whether it
went to the push worker at all. A muted channel stores the message and answers
`"muted":true` — muting silences the phone, it does not drop the message.

### Reading the response

**Check `code`, not the HTTP status.** Every response is wrapped the same way, and a
request that was understood but refused — an oversized attachment, a malformed `reply`
— still comes back as HTTP 200 with a non-zero `code` and the reason in `msg`.

| | HTTP | Body |
|---|---|---|
| Sent | 200 | `{"code":0,"msg":"success","data":{…}}` |
| Refused | 200 | `{"code":1,"msg":"<what was wrong>","data":null}` |
| Bad or missing token | 401 | `{"code":1,"msg":"invalid channel token"}` |
| Rate limited | 429 | `{"code":1,"msg":"…","data":{"error_code":"…"}}` |

In a shell, that means `curl -f` is not enough:

```bash
curl -sS -d "disk at 92%" https://push.example.com/api/v1/send/<token> \
  | grep -q '"code":0' || echo "send failed" >&2
```

A missing and a disabled token give the same sentence on purpose, so the error cannot
be used to probe which tokens exist.

### Where the token goes

The path is the readable form, but the token is also accepted as
`Authorization: Bearer <token>`, as a `token` header, as `?token=`, or as a `token`
form field — whichever your caller finds easiest. `POST /api/v1/send` (no token in the
path) exists for exactly that.

**The body is not in the push payload.** The payload carries a summary and an id; the
body stays in the database and the app fetches it. That is why a body can be far
longer than a notification allows — up to `limit.body_max_kb`, 1 MB by default — and
why a push can always be sent: the payload degrades until it fits.

## Message fields

Every field is optional except that a message needs *something* — a title, a body or
a file.

| Field | Type | What it does |
|---|---|---|
| `title` | string | The notification title. |
| `body` | string | The message itself. `text` is accepted as an alias. |
| `type` | string | How the app renders the body: `text` (default), `markdown`, `image`, `file`, `link`, `card`. |
| `summary` | string | Overrides the auto-generated push summary. Generated from the body when omitted. |
| `link` | string | A URL the message opens. |
| `copy` | string | A string the app offers to copy — a code, an address, a command. |
| `file` | string | The `uid` of something uploaded earlier. See [Attachments](#attachments). |
| `items` | array | Rows of a card: `[{"k":"branch","v":"main","style":"ok"}]`. `style` is one of `ok`, `warn`, `error`, `muted`. Rendered under the body, in the order given. |
| `collapse_id` | string | Messages sharing one id replace each other on the lock screen instead of stacking. |
| `idem_key` | string | Send the same key twice and the second call returns the first message with `"dedup":true` instead of creating another. Retry loops become safe. |
| `reply` | object | Asks the person to answer. See [Asking for an answer](#asking-for-an-answer). |

There is no `sound`, `level` or `priority` field, by design — see
[Before you start](#before-you-start).

The push summary is generated from the body, and generated to be *read*: code fences
are dropped whole, table pipes become separators, and rules disappear. What lands on
the lock screen is a sentence, not markup — which matters because the app's
notification extension can be skipped by the system, and then the summary is all
there is.

## Attachments

The multipart form above is the one-step version. To send the same image to several
channels, upload it once:

```bash
curl -F "file=@chart.png" -H "Authorization: Bearer <token>" \
  https://push.example.com/api/v1/upload
# {"code":0,"msg":"success","data":{"uid":"…","name":"chart.png","mime":"image/png",
#   "size":48213,"width":1600,"height":900,"url":"https://…","thumb":"https://…"}}
curl -H 'Content-Type: application/json' \
  -d '{"type":"image","title":"Disk alert","file":"<uid>"}' \
  https://push.example.com/api/v1/send/<token>
```

In the one-step multipart form the field may be called `image` or `file`; on
`/api/v1/upload` it is `file`.

Uploads are content-addressed: the same bytes uploaded twice are stored once, and the
blob is removed when the last message referencing it is gone.

Limits come from `[storage]`: `max_upload_mb` (20 by default), `image_max_px` (1600)
and `thumb_max_px` (600). Images are re-encoded to those bounds — the notification
extension has a 24 MB memory budget, and a full-resolution photo does not fit in it.

Download URLs are signed and expire after `storage.url_ttl` (7 days). They carry no
session, because the notification extension has to fetch the image before the phone
is first unlocked, when it cannot reliably read the Keychain.

## Asking for an answer

A message can ask for an answer: buttons to tap, checkboxes to pick several, a slider
for a number, or a field to type in. The person opens the notification, answers in the
app, and the server POSTs their answer to a URL you provide.

```bash
curl -H 'Content-Type: application/json' -d '{
  "title": "Curtains open in 30 seconds",
  "reply": {
    "type": "choice",
    "options": ["Open", "Keep closed"],
    "timeout": 30,
    "webhook": "https://home.example.com/api/webhook/curtain"
  }
}' https://push.example.com/api/v1/send/<token>
```

| `reply` field | Applies to | Meaning |
|---|---|---|
| `type` | all | `choice`, `multi`, `number` or `text`. Omitted, it is `choice` when `options` is present and `text` otherwise. |
| `options` | `choice`, `multi` | Up to 10, each up to 40 characters. Shown in the order given. |
| `webhook` | all | **Required.** Where the answer is POSTed. |
| `timeout` | all | Seconds until the message stops being answerable. 0 or omitted means no limit; maximum 30 days. |
| `min`, `max`, `step` | `number` | Range of the slider. `min` may be 0. |
| `unit` | `number` | Display only — `°C`, `%`, `min`. The server never interprets it. |

Leave `timeout` out unless something happens by itself when the time is up. An agent
asking a question and waiting has no default branch; a limit there only throws the
answer away.

`webhook` is required because an answer with nowhere to go is worse than an error at
send time — someone will have tapped that button for nothing.

Once they answer, your endpoint receives:

```json
{"uid":"01J…","channel":"…","title":"Curtains open in 30 seconds","reply":"Keep closed","replied_at":1726000000}
```

`reply` is a string, an array of strings for `multi`, and a number for `number`.

A message is answered once, first answer wins; other devices see that same answer
through sync. The answer does **not** come back on the send connection — that one ends
when the message is stored, and the person may look at their phone minutes later.

### Verifying the callback

Every callback carries `X-Knockbox-Signature: sha256=<HMAC-SHA256(body, channel token)>`.
Verify it before acting, and compare in constant time:

```python
import hmac, hashlib

def verify(raw_body: bytes, header: str, token: str) -> bool:
    expected = "sha256=" + hmac.new(token.encode(), raw_body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, header or "")
```

Sign the **raw** bytes, before any JSON parsing — re-serializing changes them.

Delivery retries up to `reply.webhook_max_attempts` (3) with a 5s / 30s / 2m backoff,
so your endpoint should tolerate the same answer arriving twice. `uid` is stable
across retries and makes a good deduplication key.

### The flat form

A one-line `curl` can ask as well:

```bash
curl "https://push.example.com/api/v1/send/<token>?title=Deploy%3F&choices=Deploy,Hold&reply_timeout=30&reply_webhook=https://…"
```

### Callbacks to private addresses

A public instance refuses callbacks to private addresses by default — that is an SSRF
surface. A self-hosted one allows them, because the server sits inside your own network
and those are exactly the addresses it needs to reach. The switch is
`reply.public_allow_private`.

## Sending to many channels at once

```bash
curl -H 'Content-Type: application/json' -d '{
  "tokens": ["<token-a>", "<token-b>"],
  "title": "Deploy finished"
}' https://push.example.com/api/v1/send/batch
```

Results are reported per token, so one bad token does not fail the batch — the
response carries `results`, `ok` and `failed`. One upstream call can reach many people
without anyone holding a credential that can write to every channel.

At most 200 channels per call.

## Handing it to an AI agent

The same send credential also works as an [MCP](https://modelcontextprotocol.io)
endpoint over streamable HTTP, exposing a single tool that sends one notification.

```bash
claude mcp add --transport http knockbox https://push.example.com/mcp/<token>
```

Clients that take a config file want this:

```json
{
  "mcpServers": {
    "knockbox": {
      "type": "http",
      "url": "https://push.example.com/mcp/<token>"
    }
  }
}
```

An assistant that configures itself only needs the sentence "add an MCP server,
Streamable HTTP, at this address" plus the address. All three forms sit on `/docs` and
on a channel's send page with the address already filled in.

It is the same path as `/api/v1/send` — same permission, same rate limit, same quota —
so an agent that can run commands gains nothing by switching. What it buys is three
things: clients without a shell can connect; a body with newlines and quotes never goes
through shell quoting; and the tool sits in the agent's tool list, so nothing has to be
pasted in first.

## Rate limits and quotas

These are two different things, and both apply.

**Rate limit** is anti-abuse and applies everywhere, self-hosted included: every
channel token gets `limit.send_qps` requests per second with a burst of
`limit.send_burst` (20 and 60 by default). Over it, the server answers `429`. A leaked
token cannot flood the database.

**Quota** only exists on a public instance: `public.max_channels` per member,
`public.max_messages_per_day` over a rolling 24 hours. A self-hosted instance has no
quota at all.

`limit.body_max_kb` (1 MB) rejects an oversized body rather than truncating it —
silently losing the end of a message is worse than an error the sender can see.

## The CLI

Every operation is a subcommand with `-h`; none of them requires touching the database.

```
knockbox serve                 start the server
knockbox user add <name>       create an admin account (prompts for the password)
knockbox user passwd <name>    reset a password
knockbox user list             list accounts
knockbox pair                  issue a pairing code and print it as a QR code
```

Passwords are read interactively by default, and also accept a pipe
(`echo 's3cret' | knockbox user passwd admin`). Passing one as a flag is possible but
leaves it in `ps`.

## What is a public contract

`/api/v1` is: sending, and the endpoints the iOS app uses. Those are documented here
and will not change shape without a reason.

Everything under `/admin/api/` is **not**. It exists to serve the admin interface that
ships in this repository, shares its session cookie, and will change whenever that
interface does. Do not build against it — to script this server, use `/api/v1` and the
CLI.

## Endpoint reference

Unauthenticated:

| Method | Path | What it does |
|---|---|---|
| `GET` | `/api/v1/server` | Name, version and feature list. Call it before pairing to tell "wrong address" apart from "wrong code". |
| `POST` | `/api/v1/pair` | Exchange a pairing code for a device token. Rate limited per IP. |
| `GET` | `/api/v1/file/:uid` | Download an attachment. Signature only, no session. |
| `GET` | `/health` | Liveness, including database connectivity. |

Channel token — write only, scoped to one channel:

| Method | Path | What it does |
|---|---|---|
| `POST` | `/api/v1/send` | Send, token in a header, query or form field. |
| `POST` `GET` | `/api/v1/send/:token` | Send, token in the path. |
| `POST` | `/api/v1/send/batch` | Send to several channels, per-token results. |
| `POST` | `/api/v1/upload` | Upload an attachment. |
| — | `/mcp/:token` | The MCP endpoint, streamable HTTP. |

Device token — what the iOS app uses. Listed so you know what the app can do, not as
an invitation to write a second client:

| Method | Path |
|---|---|
| `POST` | `/api/v1/device/push-token` |
| `POST` | `/api/v1/pair/issue` |
| `GET` `DELETE` | `/api/v1/devices`, `/api/v1/devices/:uuid` |
| `GET` `POST` `PATCH` `DELETE` | `/api/v1/channels`, `/api/v1/channels/:id` |
| `POST` | `/api/v1/channels/:id/rotate`, `/api/v1/channels/:id/purge` |
| `GET` | `/api/v1/sync`, `/api/v1/messages` |
| `POST` | `/api/v1/messages/read`, `/api/v1/messages/delete`, `/api/v1/messages/:uid/reply` |
