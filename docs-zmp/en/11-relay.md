# 11. Plan B: a relay

**English** | [Español](../es/11-relay.md)

A **relay** is any `zmp-server` on the internet that lets people it does not know make rooms. When nobody in the
group can be reached (CGNAT, no access to the router), everyone connects to the relay instead: the room lives there,
and every PC only makes outgoing connections, which always work.

ZMP has **no built-in relay and names none**. A relay is run by someone: a friend with a VPS, a community, you. Use
one whose owner you trust: the relay passes your game's inputs and keeps the room's shared save (it never sees your
password in clear, and it never runs the game).

## Use a relay

You need its address (for example `relay.example.org` or `203.0.113.10:47100`).

1. The host, in the game: **Join > By address**:
   - **Server**: the relay's address;
   - **Room**: a name nobody would guess (for example `forest-k3f9`);
   - **Password**: make one up. The first player who enters a room with a password sets it;
   - **Connect**, then load the save that will be the group's game.
2. The friends do the same with the same server, room and password.

Instead of sending three things, the host can make an **invite code** with `zmp-invite` (in the Windows zip, next to
`soh.exe`; open a terminal in that folder):

```
zmp-invite encode -addr relay.example.org -room forest-k3f9 -password make-one-up
```

It prints the code (`ZMP-...`) and the `zmp://` link; friends paste it in **Join > Join with a code**. Use `-port <port>`
if the relay is not on 47100, `-addr wss://relay.example.org/` for a `wss://` relay, and `-hint "..."` instead of
`-password` to put only a hint in the code.

On a relay, the password of a room that nobody uses for 7 days is forgotten; after that, the name is free again.

## Run a relay

Any `zmp-server` becomes a relay with `-relay` (or `relay = true` in its settings file, or `ZMP_RELAY=true`):

```
zmp-server -relay
```

It listens on every interface on port 47100 and applies limits meant for strangers. Each one has its own option
([table](04-host-on-your-pc.md#4-linux-a-raspberry-pi-or-without-a-page)):

| Limit | Relay default | Option |
|---|---|---|
| Rooms open at once | 100 | `-max-rooms` |
| Game connections from one address | 8 | `-max-conns-per-ip` |
| Rooms made from one address at once | 3 | `-max-rooms-per-ip` |
| Forget the password of an unused room after | 7 days | `-room-key-ttl` |
| Rooms without a password | allowed | `-require-room-password` to refuse them |
| List of rooms | never published | |

- Behind a proxy or tunnel that hides the players' addresses, everyone seems to come from the same address: turn the
  per-address limits off (`-max-conns-per-ip 0 -max-rooms-per-ip 0`).
- On a VPS, run it with Docker ([VPS guide](06-host-on-a-vps.md)) and add `ZMP_RELAY: "true"` to its `environment:`.
- For browser players behind `https`, put a TLS proxy in front ([wss:// for browsers](12-wss-for-browsers.md)).
- A site that serves the browser version can point it at its relay with `zmp-config.json`
  ([guide](10-web-version-on-your-site.md)).
