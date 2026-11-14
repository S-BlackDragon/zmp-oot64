# 10. Put the web version on your site (and preconfigure it)

**English** | [Español](../es/10-web-version-on-your-site.md)

The web version is a folder of files (`index.html`, `soh.js`, `soh.wasm`, ...), from `ZMP-<version>-web.zip` or
[built yourself](build.md). Any web server can serve it as it is. To make it start the way your community plays, add
**one file next to those files**: `zmp-config.json`. You never change the game's code.

- **No `zmp-config.json`**: the game presets nothing. The player types a server (or pastes an invite code), a room
  and a name.
- **With `zmp-config.json`**: the server field already holds your server, and you decide whether the player may
  change it, which room to use, how passwords work, the default name and the language.

A value the game does not accept is **ignored**, reported in the browser console and in a notice at the top of the
page, and the rest of the file still applies. A broken file never stops the game.

Your server must be reachable from the browser: over `https://` that means a `wss://` address
([wss:// for browsers](12-wss-for-browsers.md)). It can be your own server, a [VPS](06-host-on-a-vps.md) or a
[relay](11-relay.md).

## Example

```json
{
  "version": 1,
  "servers": [{ "label": "Our server", "address": "wss://zmp.example.org/", "default": true }],
  "lockServer": true,
  "room": { "default": "forest" },
  "password": { "policy": "ask" },
  "name": { "default": "", "lock": false },
  "ui": { "language": "auto", "title": "Ocarina co-op" },
  "url": { "allow": ["room", "name", "lang", "autojoin", "invite", "pw"] },
  "updates": { "check": false }
}
```

With this file a visitor opens the page, chooses their ROM and lands straight in room `forest` on your server (it
joins without asking, because the server is fixed and a room is given; it asks only for a password if the room has
one).

The full list of keys, for editors that check JSON, is in [zmp-config.schema.json](../zmp-config.schema.json).

## Keys

| Key | Values | What it does |
|---|---|---|
| `version` | `1` | format version |
| `servers` | list of `{label, address, default}` | the server(s) you suggest; the one with `"default": true` (or the first) fills the server field. `address`: `wss://host/path` (pages served over https need `wss://`), `ws://host:port/`, or `host:port` |
| `lockServer` | `true` / `false` | `true`: the player cannot change the server, and the game joins without asking when a room is known too |
| `room` | a name (1 to 64 characters), `"ask"`, or `{ "default": "...", "ask": true/false, "suggestRandom": true/false }` | the room to use; `"ask"` leaves the field empty for the player; `suggestRandom` makes one up (`sala-xxxx`) |
| `password.policy` | `none`, `ask`, `required`, `given` | `none`: no password field; `ask`: optional field (default); `required`: the game does not connect without one; `given`: the password comes from `password.value` or from `#pw=` in the address, and no field is shown |
| `password.value` | text, up to 64 | the password for `given` (anyone who can read your config can read it: use it only for rooms that are private because nobody knows the address) |
| `password.fromUrlFragment` | `true` / `false` | whether `#pw=` in the address is read (default `true`) |
| `name.default` | text, up to 32 | the name already filled in; the player can still change it |
| `name.lock` | `true` / `false` | the player cannot change the name |
| `ui.language` | `auto`, `es`, `en` | language of the game's multiplayer screens (`auto`: the browser's) |
| `ui.title` | text, up to 80 | the page title |
| `url.allow` | list of `server`, `room`, `name`, `lang`, `autojoin`, `invite`, `pw` | which address parameters may change the settings (all of them when the key is absent) |
| `updates.check`, `updates.source` | `true`/`false`, an `https://` address | reserved for a future update notice; this version does not check for updates |
| `assets.remember`, `embed.parentOrigins` | `ask`/`never`, a list | reserved for later versions (accepted, not used yet) |

## Address parameters

They override the file (when `url.allow` lets them):

- `?server=wss://zmp.example.org/` — the server (refused when `lockServer` is `true`).
- `?room=forest` — the room; a room in the address joins straight away, like `?autojoin=1`.
- `?name=Saria`, `?lang=es`, `?autojoin=1` (or `0` to never join by itself).
- `#invite=ZMP-...` — an invite code: the game joins with it. `#pw=...` — the room password.

What follows `#` never reaches any server, and the page wipes it from the address bar once read.

## What the game never does

The game's code does not name any server or site. Everything your page shows comes from your `zmp-config.json`; a
copy of the web folder without it presets nothing. The page reads `zmp-config.json` from its own site and connects
only to the server it ends up with; it uploads nothing.
