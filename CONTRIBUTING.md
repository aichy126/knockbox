# Contributing

**English** · [简体中文](CONTRIBUTING.zh-CN.md)

Thanks for taking a look. This is a small project maintained by one person; the
notes below exist so a patch does not get stuck on something avoidable.

Found a security problem? Do not open an issue — see [SECURITY.md](SECURITY.md).

## What belongs here

Knockbox delivers messages: it stores them, pushes them, keeps the history and serves
attachments. Who should receive which message is the upstream system's job — there is
no notion of subscriptions here, and adding one is out of scope.

Two constraints shape most decisions:

- **One binary, one SQLite file.** No Redis, no message broker, no external service.
  A self-hoster should be able to run this with `docker run` and nothing else.
- **Zero outbound network except Apple.** The server talks to APNs and to nobody else.

A change that breaks either of those needs a discussion in an issue first.

## Getting set up

Go 1.26 or newer.

```bash
cp config.toml.example config.toml   # edit server.external_url at minimum
make build
make run                             # first start creates an admin and prints its password
./knockbox pair                      # prints a QR code for the iOS app
```

`config.toml` is gitignored. Every key in it is documented in
`config.toml.example`; if you add a key, document it there too and make sure the code
actually reads it.

## Before you open a pull request

```bash
make lint    # go vet, go mod tidy -diff, gofmt, golangci-lint
make test    # go test ./... -race -cover
```

CI runs the same checks, plus a `CGO_ENABLED=0` build and a Docker smoke test that
boots the image and waits for `/health`. The pure-Go SQLite driver is the reason the
CGO-free build has to keep working: the release binaries are static and the image is
built `FROM alpine`.

## Tests

A bug fix should come with a test that fails without the fix. This is not a formality
— several of the bugs found in this codebase were invisible from the outside
(a reference count that never decremented, a queue row that stayed pending forever,
a body silently truncated on the way to the database). Those only show up if a test
asserts the invariant directly.

`make cover` prints the total and opens the HTML report. The packages carrying logic
(`internal/service`, `internal/dao`, `internal/library/*`) are the ones worth raising;
the HTML-building code in `internal/api/web` is not.

## Conventions

**Comments are written in Chinese.** That is deliberate — the maintainer reviews in
Chinese. Pull requests and issues are welcome in either English or Chinese. Please do
not translate existing comments as part of an unrelated change.

**Comments explain why, not what.** No development history, no dates, no personal
names, no comparisons with other projects. `// 引用计数必须和插入同一个事务` is useful;
"used to be outside the transaction, fixed on the 11th" is not — the reader wants the
constraint that holds now, and git already has the rest.

**Schema changes go in a new migration.** DDL lives only in
`internal/migrate/sql/NNNN_name.sql`; the structs in `internal/models` map columns and
nothing else. Never edit a migration that has already shipped — add the next number.

**The SQLite pragma chain has one definition.** `migrate.DSN` is it, and
`migrate.bootstrap()` asserts the critical pragmas at startup. Do not hand-copy the
string into a test.

**Errors get wrapped, never swallowed.** The user sees what happened and what they can
do about it, in one sentence, with no type names or stack traces. The original error
goes to the log verbatim. Both halves are required: a raw
`typeMismatch(Swift.Int64, …)` in a toast is useless, and so is "operation failed".

**User-visible text is product copy.** Before writing a string that reaches a screen,
ask whether it needs to exist at all; if it does, say what the thing is, what the user
should do, and what will happen. No jokes, no self-deprecation, no implementation
details. The tone is restrained and factual.

**Every config key must be read by the code.** A key that exists only in the example
file is worse than no key: someone will change it and wonder why nothing happens.

**Dependencies.** The build must stay CGO-free. Adding a module needs a reason in the
PR description; a thirty-line helper is usually better than a new import.

**The pusher runs as a single instance.** `claim` takes rows off `push_log` without a
lease, so two processes on the same database would push everything twice. If you need
horizontal scale, that needs a lease column first, and that is a design change worth
an issue.

## Adding a language

The pages a stranger reaches — the pairing page, the sending guide and their error
pages — are translated. Adding a language is **one file**:

1. Copy `internal/api/web/locales/en.json` to `<code>.json`, where `<code>` is the
   language subtag (`ja`, `de`, `pt-br`). The file name *is* the language; nothing
   registers it anywhere else.
2. Translate the values. Leave `lang_name` as that language's own name for itself
   (`日本語`, not `Japanese`) — it is what the language switch shows. Set `html_lang`
   only if it differs from the file name (`zh` → `zh-CN`).
3. Keep the `%s` and `%d` placeholders, and keep their order.

Keys you do not translate fall back to English, so a half-finished translation is
still worth sending. A key that is **not** in `en.json` fails the tests — it is always
either a typo or a key that was renamed without the translations following.

The admin interface (`/login`, `/admin/*`) is not part of this — see the trade-offs
section of the README for why.

## Commits

Conventional-commit prefixes, as used in the existing history: `feat:`, `fix:`,
`docs:`, `perf:`, `chore:`, `style:`. The release changelog drops `docs:`, `test:` and
`chore:`, so put anything a user should read under one of the others.

Say what changed and why it was wrong before. A commit body that explains the failure
mode is worth more than one that lists the files touched.

## Do not commit

`config.toml`, `data/`, `logs/`, and any `.p8` file. The one exception is the public
key already tracked at `internal/library/apns/embedded/` — see
[SECURITY.md](SECURITY.md) for why it is there.
