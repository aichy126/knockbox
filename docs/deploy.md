# Running your own server

**English** · [简体中文](deploy.zh-CN.md)

Knockbox is one Go binary and one SQLite file. **No database server, no Redis, no
message queue** — rate limiting is counted in memory and the push retry queue is a
table in that same SQLite file. Anything that can run a container can run this.

For sending messages once it is up, see [api.md](api.md).

- [Pick a way to install it](#pick-a-way-to-install-it)
- [Docker Compose](#docker-compose)
- [Docker, one command](#docker-one-command)
- [A prebuilt binary](#a-prebuilt-binary)
- [From source](#from-source)
- [The images](#the-images)
- [First start](#first-start)
- [Putting it on the internet](#putting-it-on-the-internet)
- [Upgrading and rolling back](#upgrading-and-rolling-back)
- [Backups](#backups)
- [When something is wrong](#when-something-is-wrong)
- [Configuration](#configuration)
- [Running it for other people](#running-it-for-other-people)
- [Using your own APNs key](#using-your-own-apns-key)

## Pick a way to install it

| | Good for | Upgrading is |
|---|---|---|
| [Docker Compose](#docker-compose) | Most people. One file you keep next to the data. | `docker compose pull && docker compose up -d` |
| [Docker, one command](#docker-one-command) | Trying it out in a minute. | Stop, remove, `docker run` again |
| [A prebuilt binary](#a-prebuilt-binary) | No container runtime, or a machine you already manage with systemd. | Replace the binary, restart |
| [From source](#from-source) | You want to change something. Needs Go 1.26+. | `git pull && make build` |

Everything below assumes `push.example.com` is where your phone will reach the server.
Replace it with your own address.

## Docker Compose

Take [compose.yaml](../compose.yaml) from this repository, change
`IGO_SERVER_EXTERNAL_URL` to your address, and:

```bash
docker compose up -d
docker compose logs
```

The whole file is short enough to read:

```yaml
services:
  knockbox:
    image: ghcr.io/aichy126/knockbox:latest
    container_name: knockbox
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      IGO_SERVER_EXTERNAL_URL: "https://push.example.com"
      IGO_SERVER_NAME: "My Push"
    volumes:
      - knockbox-data:/app/data
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/health"]
      interval: 30s
      timeout: 5s
      start_period: 10s
      retries: 3

volumes:
  knockbox-data:
```

Two lines deserve a second look:

- **`127.0.0.1:8080:8080`** publishes the port on loopback only, because the container
  speaks plain HTTP and TLS belongs in a reverse proxy in front of it. If you are only
  trying it out on this machine, `"8080:8080"` is fine.
- **`knockbox-data:/app/data`** is a named volume. To keep the data somewhere you can
  see it, use a path instead: `- ./data:/app/data`.

Day to day:

```bash
docker compose logs -f           # follow the log
docker compose restart           # restart
docker compose pull && docker compose up -d   # upgrade
docker compose down              # stop (the volume stays)
```

## Docker, one command

```bash
docker run -d --name knockbox -p 8080:8080 -v knockbox-data:/app/data \
  -e IGO_SERVER_EXTERNAL_URL=http://localhost:8080 \
  ghcr.io/aichy126/knockbox
docker logs knockbox
```

Good for a first look. For anything lasting, use Compose — the settings end up in a
file you still have next month.

## A prebuilt binary

Every tagged release carries binaries for Linux, macOS, Windows and FreeBSD on
amd64, arm64, 386 and arm. Download the one for your machine from the
[releases page](https://github.com/aichy126/knockbox/releases), then:

```bash
tar xzf knockbox_*_linux_amd64.tar.gz
cp config.toml.example config.toml   # edit server.external_url
./knockbox serve -c config.toml
```

The archive contains the binary, `config.toml.example`, both READMEs and both
SECURITY files. There is nothing else to install: no runtime, no shared libraries.

A systemd unit, if you want one:

```ini
[Unit]
Description=Knockbox
After=network-online.target

[Service]
ExecStart=/opt/knockbox/knockbox serve -c /opt/knockbox/config.toml
WorkingDirectory=/opt/knockbox
User=knockbox
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

## From source

```bash
git clone https://github.com/aichy126/knockbox.git
cd knockbox
make build          # produces ./knockbox
```

Go 1.26 or newer, and Node 20+ for the admin interface (`make build` runs it for you).
The Go build is `CGO_ENABLED=0` and must stay that way — the SQLite driver is pure Go,
which is what makes a static binary and a two-stage container image possible.

**The deliverable does not change**: still one binary and one SQLite file. The admin
interface is built into `admin/dist` and embedded with `go:embed`, so a release binary
or the Docker image carries it already — `npm` is a cost for contributors, not for you.
Skipping it still compiles; the admin interface then says it has not been built, and
everything else works.

## The images

| Registry | Address |
|---|---|
| GitHub Container Registry | [`ghcr.io/aichy126/knockbox`](https://github.com/aichy126/knockbox/pkgs/container/knockbox) |
| Docker Hub | [`aichy66/knockbox`](https://hub.docker.com/r/aichy66/knockbox) |

Both carry `linux/amd64` and `linux/arm64`, and both are the **same image** — the
release workflow builds once and copies the manifest, so the digests match. Use
whichever your network reaches faster.

Tags are `latest` and the exact version, `v1.0.3`. There are no floating `v1` or
`1.0` tags: an upgrade should be something you decided to do.

```bash
docker pull ghcr.io/aichy126/knockbox:v1.0.3   # pin a version
docker pull aichy66/knockbox                   # or latest, from Docker Hub
```

## First start

The first start creates the database and an admin account, and prints that account's
username and password in the log — **once, and nowhere else**:

```bash
docker compose logs | grep -A2 -i password
```

Then:

1. Open `https://push.example.com/login` and sign in with it.
2. Go to the pairing page. It shows a QR code.
3. Install [the iOS app](https://apps.apple.com/app/id6811317906) and scan it.
4. Change the password on the settings page of the same admin interface.

Lost the password before you wrote it down? Reset it from the command line — no
database surgery required:

```bash
docker compose exec -it knockbox /app/knockbox user passwd admin
```

### `IGO_SERVER_EXTERNAL_URL` is the one to get right

It goes into pairing links and attachment URLs, so it has to be the address the phone
will actually reach — not the address the container sees. Two symptoms of getting it
wrong, both of which look like something else:

- The app pairs, but every image in the timeline stays blank.
- The QR code scans, and the app then cannot reach the server at all.

It is also readable before pairing: `GET /api/v1/server` answers with the server's
name and version, which is how the app tells "wrong address" apart from "wrong code".

## Putting it on the internet

The server speaks plain HTTP and does not terminate TLS. Put a reverse proxy in front
of it, and keep the container on loopback.

**Caddy** — the short one, certificates included:

```caddyfile
push.example.com {
	reverse_proxy 127.0.0.1:8080
	request_body {
		max_size 25MB
	}
}
```

**nginx**:

```nginx
server {
    listen 443 ssl http2;
    server_name push.example.com;

    ssl_certificate     /etc/letsencrypt/live/push.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/push.example.com/privkey.pem;

    # Must be at least storage.max_upload_mb, or attachments fail with 413
    # while everything else keeps working.
    client_max_body_size 25m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

`X-Forwarded-For` is worth setting: the pairing and registration limits are per IP, and
without it every request appears to come from the proxy.

Then set `IGO_SERVER_EXTERNAL_URL=https://push.example.com` and restart.

## Upgrading and rolling back

```bash
docker compose pull && docker compose up -d
```

Schema migrations run at startup, in order, and are recorded — an upgrade is just a
newer image. Read the release notes first anyway; that is where a change that needs
your attention is described.

Rolling back is pinning the old tag:

```yaml
image: ghcr.io/aichy126/knockbox:v1.0.2
```

Migrations are not reversed automatically. Roll back one version without worrying;
across several, restore a backup from before the upgrade.

## Backups

Everything lives under `/app/data`:

```
data/
  knockbox.db        the database — messages, channels, devices, accounts
  knockbox.db-wal    write-ahead log
  files/             attachments, content-addressed
  logs/              rotated log files
  backup/            automatic snapshots (see below)
```

The server takes a snapshot of the database every `retention.backup_every_hours`
(24 by default) and keeps `retention.backup_keep` of them (7). Snapshots are made with
SQLite's `VACUUM INTO`, so each one is a consistent single file even while the server
is writing — it is not a file copy.

Attachments are **not** in the snapshot. To back up everything, stop the container and
copy the whole directory, or copy `data/files/` alongside the newest snapshot.

Restoring:

```bash
docker compose down
# put the snapshot back as data/knockbox.db, and remove any -wal / -shm beside it
docker compose up -d
```

Copying `knockbox.db` while the server runs is the one thing to avoid: without the
`-wal` file beside it you get a database that is missing the most recent writes.

## When something is wrong

**Is it alive?**

```bash
curl -fsS https://push.example.com/health          # includes database connectivity
curl -fsS https://push.example.com/api/v1/server   # name, version, features
```

**The app shows nothing / cannot pair.** Check `IGO_SERVER_EXTERNAL_URL` first — see
above. Pairing codes are single-use and expire after `server.pair_ttl` (10 minutes);
issue a new one rather than retrying the old.

**Sending answers `"code":1`.** The reason is in `msg`. Note that a refused send still
returns HTTP 200 — see [Reading the response](api.md#reading-the-response).

**Sending answers 401.** The channel token is wrong, or the channel was deleted. A
missing and a disabled token give the same sentence, deliberately.

**Sending works but the phone stays quiet.** In order:

1. Is the channel muted? Mute lives on the server; the message is stored either way.
2. Are notifications allowed for the app in iOS Settings, and is the device still
   listed in the app?
3. `data/logs/knockbox.log` records every push attempt and what APNs answered. A
   device that answered `410` has been unregistered by Apple — it reappears when the
   app is opened again.

**Attachments fail while messages work.** `client_max_body_size` in the proxy is below
`storage.max_upload_mb`.

Turn up the detail with `IGO_LOCAL_LOGGER_LEVEL=DEBUG` when you need it.

## Configuration

Copy `config.toml.example` and edit it, or set environment variables — every key has
one. The rule is `IGO_` plus the path in upper case with dots replaced by underscores:

```
server.external_url         ->  IGO_SERVER_EXTERNAL_URL
apns.production.key_base64  ->  IGO_APNS_PRODUCTION_KEY_BASE64
limit.send_qps              ->  IGO_LIMIT_SEND_QPS
```

Environment variables are the intended way to pass secrets: a value in
`config.toml` is a value in a file someone can read.

The settings under `[public]` are different. Once they have been saved from the admin
interface, **the stored value wins** and changing the environment variable has no
further effect — the config file is only the default for the first start.

<details>
<summary><b>Every setting</b></summary>

### `[server]`

| Key | Default | What it does |
|---|---|---|
| `name` | `"My Push"` | The name the app shows for this server. |
| `external_url` | `"http://localhost:8080"` | The address your phone reaches. Pairing links and attachment URLs are built from it. |
| `pair_ttl` | `600` | Seconds a pairing code stays valid. Single use. |

### `[local]` — the HTTP server

| Key | Default | What it does |
|---|---|---|
| `address` | `":8080"` | Listen address. Omit the host to listen on every interface. |
| `debug` | `false` | Verbose startup route dump and request logs. Noise, not safety. |

### `[local.logger]`

| Key | Default | What it does |
|---|---|---|
| `dir` | `"./data/logs"` | Where log files go. Created on startup. |
| `name` | `"knockbox.log"` | File name. |
| `access` | `true` | One line per HTTP request. |
| `level` | `"INFO"` | `DEBUG`, `INFO`, `WARN` or `ERROR`. |
| `max_size` | `50` | Rotate after this many MB. |
| `max_backups` | `5` | How many rotated files to keep. |
| `max_age` | `14` | Days to keep them. |

### `[sqlite.knockbox]`

| Key | Default | What it does |
|---|---|---|
| `data_source` | `./data/knockbox.db?…` | Path plus pragmas. **Keep the whole pragma chain** — WAL, `busy_timeout`, `secure_delete` and `auto_vacuum` are all load-bearing, and the server asserts the important ones at startup. |
| `max_open` / `max_idle` | `4` / `4` | Connection pool. |
| `is_debug` | `false` | Log every SQL statement. |

### `[bootstrap]` — the first account

| Key | Default | What it does |
|---|---|---|
| `username` | `"admin"` | Created when the database has no accounts. |
| `password` | `""` | Empty means one is generated and printed to the log once. |

### `[apns]`

| Key | Default | What it does |
|---|---|---|
| `enabled` | `true` | Turn pushing off entirely (messages are still stored). |
| `topic` | `"com.miramiao.knockbox"` | The bundle id pushes are addressed to. Change it only with your own key and your own build of the app. |
| `team_id` | `"258X46W652"` | Team the key belongs to. |
| `concurrency` | `8` | Parallel pushes. |
| `timeout_ms` | `8000` | Per-request timeout. |
| `max_attempts` | `5` | Retries, backing off 5s / 30s / 2m / 10m / 30m. |
| `default_expiration` | `86400` | Seconds APNs should keep trying. **Do not set 0** — that means "deliver now or drop". |

`[apns.production]` and `[apns.sandbox]` each take `key_id`, `key_file` and
`key_base64`. Leave production empty to use the topic-specific key embedded in this
repository; see [Using your own APNs key](#using-your-own-apns-key). Sandbox is only
needed if you build the app yourself in Debug.

### `[storage]` — attachments

| Key | Default | What it does |
|---|---|---|
| `dir` | `"./data/files"` | Where blobs go. Content-addressed. |
| `max_upload_mb` | `20` | Largest accepted upload. Your proxy must allow at least this much. |
| `url_ttl` | `604800` | Seconds a signed download URL stays valid. Notifications can be delivered late, so short values produce blank images. |
| `image_max_px` | `1600` | Images are re-encoded to fit this. |
| `thumb_max_px` | `600` | Thumbnail size. The notification extension has a 24 MB memory budget. |
| `jpeg_quality` | `82` | Re-encode quality. |

### `[retention]` — keeping and backing up

| Key | Default | What it does |
|---|---|---|
| `days` | `0` | Delete messages older than this, instance-wide. 0 keeps everything. |
| `backup_every_hours` | `24` | Snapshot interval. 0 turns snapshots off. |
| `backup_keep` | `7` | How many snapshots to keep. |
| `backup_dir` | `"./data/backup"` | Where they go. |

### `[reply]` — answer callbacks

| Key | Default | What it does |
|---|---|---|
| `webhook_timeout_ms` | `5000` | Per-attempt timeout. |
| `webhook_max_attempts` | `3` | Retries, backing off 5s / 30s / 2m. |
| `public_allow_private` | `false` | Whether callbacks may target private addresses. Meaningful on a public instance only; a self-hosted one allows them, because those are exactly the addresses it needs to reach. |

### `[limit]` — anti-abuse, everywhere

| Key | Default | What it does |
|---|---|---|
| `send_qps` | `20` | Requests per second per channel token. 0 means no limit. |
| `send_burst` | `60` | How many may arrive at once. |
| `pair_per_min` | `5` | Pairing attempts per IP per minute. A pairing code is 8 characters — short enough to read out loud, and therefore only 40 bits — so single use, a 10-minute life and this limit are together what make guessing it impractical. |
| `body_max_kb` | `1024` | Largest message body. Oversized bodies are rejected, not truncated. |

### `[public]` — see [Running it for other people](#running-it-for-other-people)

| Key | Default | What it does |
|---|---|---|
| `enabled` | `false` | Turns the home page into a sign-up page. |
| `register_per_hour` | `3` | New identities per IP per hour. |
| `max_channels` | `20` | Channels per member. |
| `max_messages_per_day` | `500` | Messages per member over a rolling 24 hours. |
| `retention_days` | `30` | Per-member retention. The stricter of this and `retention.days` applies. |
| `site_name` | `"Knockbox"` | Name shown on the public pages. |
| `docs_url` | `"/docs"` | Where "how do I use this" points. |

</details>

## Running it for other people

The same binary has a second shape. With `public.enabled = true`, the home page
becomes a sign-up page: a stranger opens it, gets an identity and a first channel by
scanning a code, and never creates an account or a password. That is what
<https://knockbox.miramiao.com/> is.

The settings under `[public]` exist because that shape needs limits a private instance
does not: how many identities an IP can create, how many channels each member gets,
how many messages a day, and how long their messages are kept.

With it off — the default — the home page redirects to `/login` and the sign-up
endpoints return 404. Nobody who finds the address can open an account on your server.

## Using your own APNs key

This repository ships an APNs key so you can self-host without an Apple developer
account. It is **topic-specific**: it can only push to `com.miramiao.knockbox`, the
App Store build of the iOS app, and nothing else.

It is public, and the honest consequence is in
[SECURITY.md](../SECURITY.md): anyone who also obtains a device's APNs token could push
arbitrary content to that device. To remove that exposure, use your own key:

1. Create an APNs Auth Key in your Apple developer account.
2. Point `apns.production.key_file` at it (or pass `IGO_APNS_PRODUCTION_KEY_BASE64`),
   and set `apns.production.key_id` and `apns.team_id`.
3. Set `apns.topic` to your own bundle id.
4. Build and sign the iOS app yourself with that bundle id.

Step 4 is not optional: APNs device tokens are bound to a bundle id, so your key
cannot push to an app signed by someone else.
