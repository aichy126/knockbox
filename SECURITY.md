# Security Policy

**English** · [简体中文](SECURITY.zh-CN.md)

## Reporting a vulnerability

Email **info@miramiao.com**. Please do not open a public issue for a security
problem.

Include what you did, what happened, and what you expected. A proof of concept
helps but is not required.

This is a one-person project. You should get a first reply within seven days. If
the report is valid, the fix ships in the next release and the advisory names you
unless you ask otherwise.

Only the latest release is supported. There are no backports to older tags.

## The APNs key in this repository

`internal/library/apns/embedded/AuthKey_TXYBPWPD2L.p8` is a **real Apple push key,
committed on purpose**. Read this section before you deploy.

### Why it is here

APNs device tokens are bound to a bundle id, and only a key belonging to that app's
team can push to it. Without a shared key, self-hosting Knockbox would require an
Apple developer account at $99 a year and building the iOS app yourself. The key
exists so that you can run your own server and still use the App Store build of the
app.

### What it can and cannot do

It is a **topic-specific** key, a key type Apple has supported since February 2025.
Unlike a Team-level key, it is bound to a single bundle id.

- It can push notifications to `com.miramiao.knockbox` — the App Store build of the
  Knockbox iOS app.
- It **cannot** push to any other app, including other apps under the same Apple
  developer account.
- It **cannot** push to the sandbox environment. That key is a different one and is
  not in this repository, so the public key cannot reach anyone's Xcode Debug build.
- It **cannot** read anything. APNs is send-only.

### The honest risk

Pushing to a device requires two things: this key and that device's APNs token. The
key is public, so the whole of the remaining protection is the token.

An APNs token is an opaque value Apple issues to the app on a given device. It is not
secret in a cryptographic sense, but it is not published anywhere either: it exists on
the device and in the `device.apns_token` column of the server it paired with, **in
plaintext**. That column is what an attacker would go after. A dump of your database
is therefore enough to push arbitrary notifications to every device paired with your
instance, and no key rotation on our side would prevent it — treat the database file
with that in mind.

What such an attacker still could not do: read your messages, reach your server, or
affect any other app. APNs is send-only and this key is bound to one bundle id.

This is a real reduction in security compared to keeping the key private. It is the
price of not requiring a developer account, and you should decide for yourself
whether you accept it.

### How to stop depending on it

Point the server at your own key and your own bundle id:

```toml
[apns]
topic = "com.example.yourbuild"

[apns.production]
key_id   = "YOURKEYID"
key_file = "/etc/knockbox/AuthKey_YOURKEYID.p8"
```

You then have to build and sign the iOS app yourself, because App Store builds of
Knockbox carry the `com.miramiao.knockbox` bundle id.

### Rotation

If the key is abused and Apple rate-limits or revokes it, it will be replaced. A new
key means **every self-hosted instance has to upgrade to keep receiving pushes**;
there is no way to update an embedded key remotely.

A rotation is announced in two places at once:

- a GitHub Security Advisory on this repository, and
- the release notes of the version that carries the new key.

To be notified, use **Watch → Custom → Releases and Security advisories** on this
repository. If you run Knockbox for other people, do this.

## What the server holds

These are design decisions, not defects. They are covered in more detail in the
README's trade-offs section; the short version matters for threat modelling.

- **No end-to-end encryption.** The server stores message bodies in plaintext,
  because it also has to generate push summaries and serve history. The security
  boundary is the machine you run it on.
- **Channel tokens are stored in plaintext** so the app can show a ready-to-paste
  `curl` line. They are write-only credentials scoped to one channel, rate-limited
  per token, and can be rotated or deleted at any time.
- **Device credentials are stored as a SHA-256 hash.** This is the token the app
  sends in `Authorization: Bearer` to read history; the plaintext is returned exactly
  once, at pairing.
- **APNs device tokens are stored in plaintext.** They have to be, because they are
  what gets sent to Apple on every push. See the section above for what that means
  alongside a public key.
- **Admin passwords are hashed with bcrypt.** Sessions are stored server-side and can
  be revoked; changing a password invalidates every existing session.
- **Attachment links are HMAC-signed and expire.** The signing key is generated
  randomly on first start and stored in the database — there is no default value.
- **Pairing codes are 40 bits**, single-use, short-lived, and rate-limited per IP.
  All three together are what makes a code that short safe to type by hand.
- **Purging really deletes.** SQLite runs with `secure_delete`, and a purge is
  followed by a WAL checkpoint and an incremental vacuum. What this cannot promise is
  that the bytes leave an SSD, since wear levelling sits below the application.

## Deployment advice

- Serve over HTTPS and set `server.external_url` to the `https://` address. Pairing
  codes, channel tokens and signed attachment URLs all travel over it.
- Do not expose the admin interface to the internet unless you need it there. It has
  full read access to every message on the instance, by design.
- Pass secrets through the environment rather than the config file, for example
  `IGO_APNS_PRODUCTION_KEY_BASE64`.
- Keep `public.enabled = false` unless you intend to let strangers register.

## Out of scope

- Reports produced only by running a scanner, without a demonstrated impact.
- The embedded APNs key itself. It is documented above and is a deliberate choice;
  a report saying "there is a private key in the repository" adds nothing.
- Missing hardening on an instance the reporter configured themselves, such as
  running the admin interface over plain HTTP on a public address.
