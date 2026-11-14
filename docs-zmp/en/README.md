# ZMP guides (English)

**English** | [Español](../es/README.md)

## Start here

**[Press Host game and send the code to your friend](00-host-and-send-the-code.md).** One of you presses **Host and
play** in the game and sends the invite code; the others paste it and play. That is all most groups need.

Before that, once:

1. [Get the game](01-get-the-game.md): download a release or build it yourself.
2. [Your ROM](02-your-rom.md): the game data comes from your own copy of Ocarina of Time.
3. [Play together](03-play.md): every way to join, what is shared and what is yours, saving, controls.

## Plan B: when friends on the internet cannot reach the host

- [A PC that only hosts](04-host-on-your-pc.md): `zmp-server.exe` with a double click, with its own page, settings
  file and options.
- [Open the port by hand on your router](09-open-the-port-by-hand.md) (when the router has no UPnP).
- [Tailscale or ZeroTier](05-host-with-a-virtual-lan.md): a private network over the internet, no router changes,
  works behind CGNAT.
- [A relay](11-relay.md): a `zmp-server` on the internet where the room lives; how to use one and how to run one.
- [A VPS with Docker](06-host-on-a-vps.md): always on.
- [wss:// for browsers](12-wss-for-browsers.md): secure connections for pages served over `https`.

## And

- [Browser mode](07-browser-mode.md): play in Chrome, Edge or Firefox, no install.
- [Put the web version on your site](10-web-version-on-your-site.md): `zmp-config.json` to preset your server, room
  and passwords ([schema](../zmp-config.schema.json)).
- [Troubleshooting](08-troubleshooting.md): every message and what to do.
- [Building from source](build.md): Windows, browser and server builds, packaging.
- [Making a release](release.md) (for maintainers).

## How ZMP works, in one paragraph

ZMP is **lockstep** multiplayer: every player's PC runs the whole game, and all of them receive exactly the same
inputs in the same order, so every PC computes exactly the same world, frame by frame. The only thing in the middle is
a small **relay server**, `zmp-server`: it does not run the game, it only puts every player's buttons in order, passes
them on, and keeps each room's shared save. The game starts it for you when you press **Host and play**; it can also
run on its own on any PC, a VPS or a relay. There is no central ZMP server, no account and nothing to sign up for:
each group of friends uses its own server.
