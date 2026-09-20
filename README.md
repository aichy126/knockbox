<p align="center">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/icon-256.png" width="104" alt="">
</p>

<h1 align="center">Knockbox</h1>

<p align="center">
  Self-hosted push notifications for iOS.<br>
  Send a message with one <code>curl</code>, get it on your phone — and get an answer back.
</p>

<p align="center">
  <b>English</b> · <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <a href="https://github.com/aichy126/knockbox/actions/workflows/ci.yml"><img src="https://github.com/aichy126/knockbox/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/aichy126/knockbox/releases"><img src="https://img.shields.io/github/v/release/aichy126/knockbox?color=4F63EE" alt="Latest release"></a>
  <a href="https://github.com/aichy126/knockbox/pkgs/container/knockbox"><img src="https://img.shields.io/badge/ghcr.io-aichy126%2Fknockbox-4F63EE?logo=github" alt="GitHub Container Registry"></a>
  <a href="https://hub.docker.com/r/aichy66/knockbox"><img src="https://img.shields.io/docker/pulls/aichy66/knockbox?logo=docker&color=4F63EE" alt="Docker Hub"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/licence-MIT-blue.svg" alt="Licence: MIT"></a>
</p>

<p align="center">
  <a href="https://apps.apple.com/app/id6811317906"><img src="https://aichy126.github.io/miramiao/knockbox/images/badge-appstore-en.svg" height="44" alt="Download on the App Store"></a>
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://aichy126.github.io/miramiao/knockbox/images/arch-en-dark.svg">
    <img src="https://aichy126.github.io/miramiao/knockbox/images/arch-en-light.svg" width="880" alt="Your script sends one curl to your own server, which pushes through Apple's APNs to your iPhone. There is no relay in between.">
  </picture>
</p>

Knockbox is one Go binary and one SQLite file. It stores your messages, pushes them
straight to Apple's servers, and serves the history to the iOS app. There is no relay
in between and no account to sign up for — the server is yours.

<p align="center">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/shot-en-channels.png" width="252" alt="A channel list, one line per thing being watched">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/shot-en-markdown.png" width="252" alt="A message rendered with a table, code and an image">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/shot-en-selfhost.png" width="252" alt="The server and the push key both belong to you">
</p>

## Try it in a minute

There is a public instance at <https://knockbox.miramiao.com/>. Open it on your iPhone
with the app installed and you have your own inbox: no sign-up, no password. It runs
the same image as below, with quotas and a 30-day retention.

To keep your messages on your own hardware, run the server yourself.

## Run your own

```bash
docker run -d --name knockbox -p 8080:8080 -v knockbox-data:/app/data \
  -e IGO_SERVER_EXTERNAL_URL=http://localhost:8080 \
  ghcr.io/aichy126/knockbox
docker logs knockbox
```

The first start creates the database and an admin account, and prints that account's
username and password in the log — once, and nowhere else. Open
<http://localhost:8080/login>, sign in, and the pairing page shows a QR code to scan
with the iOS app.

**Set `IGO_SERVER_EXTERNAL_URL` to the address your phone will actually reach**, for
example `https://push.example.com`. It goes into pairing links and attachment URLs, so
`localhost` works only while you are testing on the same machine.

Then send something:

```bash
curl -d "build failed" https://push.example.com/api/v1/send/<channel-token>
```

There is a [`compose.yaml`](compose.yaml) for anything longer-lived, and prebuilt
binaries for Linux, macOS, Windows and FreeBSD on every tagged release.
**No database server, no Redis** — the whole thing is a binary and a `.db` file.
→ [Running your own server](docs/deploy.md)

## What it does

| | |
|---|---|
| **Send however you like** | Plain body, query string, multipart or JSON — every field also works from the query string, so a one-line `curl` is enough. [→](docs/api.md#sending) |
| **Bodies without a length limit** | The body is not in the push payload, so it can be far longer than a notification allows. Markdown, tables, code, images. [→](docs/api.md#message-fields) |
| **Ask for an answer** | Buttons to tap, checkboxes, a slider, or a field to type in. The answer is POSTed to a URL you provide, signed. [→](docs/api.md#asking-for-an-answer) |
| **One call, many people** | `send/batch` takes a list of tokens and reports per-token results — no broadcast credential needed. [→](docs/api.md#sending-to-many-channels-at-once) |
| **An endpoint for agents** | The same credential is an MCP endpoint over streamable HTTP, exposing one tool. [→](docs/api.md#handing-it-to-an-ai-agent) |
| **A channel per thing** | Sound, interruption level and mute live on the server, so a script never has to decide what counts as urgent. [→](docs/api.md#before-you-start) |
| **Every device, the whole history** | A new device pairs by scanning a code and pulls everything. |

## It comes with its own pages

The server serves a sending guide at `/docs` — with your address already filled in and
a "send this one" button on every example. The public instance's copy is at
<https://knockbox.miramiao.com/docs>.

<p align="center">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/web-docs-en.png" width="760" alt="The built-in sending guide, showing a send address and ready-to-run curl examples">
</p>

Behind `/login` there is an admin interface: an overview, message search, members and
their devices, pairing, and the settings that take effect without a restart.

<p align="center">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/web-admin-en.png" width="760" alt="The admin overview: counters for messages, push success rate, reachable devices and channels, with a list of recent messages">
</p>

Every page, this interface included, is English and Chinese, switchable from the top
right; the admin interface remembers the choice. Adding a language is adding one JSON
file to `internal/api/web/locales/` — the server and the admin interface read the same
one, so there is no second copy to keep in step.

## Documentation

| | |
|---|---|
| [**Running your own server**](docs/deploy.md) | Compose, Docker, binaries, source · reverse proxy · upgrades · backups · every setting |
| [**Sending and integrating**](docs/api.md) | Fields · attachments · answers and callbacks · batch · MCP · limits · CLI |
| [**Security**](SECURITY.md) | What the embedded APNs key can and cannot reach, and how to report a problem |
| [**Contributing**](CONTRIBUTING.md) | How to build, test and propose a change |

## Trade-offs you should know about

These are choices, not omissions. Read them before you deploy.

- **No end-to-end encryption.** There is no relay, so messages only ever touch your own
  server — which also has to store them and generate push summaries, both of which need
  the plaintext. E2E here would mainly protect you from your own administrator.
- **Everything is kept in plaintext, forever by default.** Full archive is the point:
  new devices pull the whole history. Set `retention.days` for a limit. The security
  boundary is that machine. [→](docs/deploy.md#backups)
- **A public APNs key is embedded in this repository**, so that you can self-host
  without an Apple developer account. It is topic-specific — it can only push to this
  app — but it is public, and the honest consequence is that anyone who also obtains a
  device's APNs token could push to that device. Use your own key to remove that
  exposure. [→](docs/deploy.md#using-your-own-apns-key) · [SECURITY.md](SECURITY.md)
- **Channel tokens are stored in plaintext**, because the app shows you a
  ready-to-paste `curl` line and hashing would make that impossible. They are
  write-only, scoped to one channel, rate-limited, and can be rotated at any time.
- **`/admin/api/*` is not a public contract.** `/api/v1` is. Everything under
  `/admin/api/` serves the admin interface that ships here and will change whenever it
  does. [→](docs/api.md#what-is-a-public-contract)

**Deleting is real deleting.** Clearing a channel physically removes the rows: the
server runs with `secure_delete`, then checkpoints the WAL and vacuums. Attachments are
reference-counted and the blob goes when the last message referencing it does. What
this does not promise is that the bytes are gone from an SSD — wear levelling is below
the application layer.

## Licence

MIT. See [LICENSE](LICENSE).
