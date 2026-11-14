# ZMP guides (English)

[Versión en español](../es/README.md)

Start here if you just want to play:

1. [Get the game](01-get-the-game.md): download a release or build it yourself.
2. [Your ROM](02-your-rom.md): the game data comes from your own copy of Ocarina of Time.
3. [Play together](03-play.md): connect to a room, what is shared and what is yours, controls.

Someone in the group has to run the small room server (one per group, on any PC):

4. [Host on your own PC](04-host-on-your-pc.md) (port forwarding on your router).
5. [Host with Tailscale or ZeroTier](05-host-with-a-virtual-lan.md) (no router changes).
6. [Host on a VPS with Docker](06-host-on-a-vps.md) (always on, optional HTTPS for the browser).

And:

7. [Browser mode](07-browser-mode.md): play in Chrome, Edge or Firefox, no install.
8. [Troubleshooting](08-troubleshooting.md): every error message and what to do.
9. [Player logs on the server](09-player-logs.md): optional, off unless the server turns it on; what is sent and how to
   opt out.
10. [Building from source](build.md): Windows, browser and server builds, packaging.
11. [Making a release](release.md) (for maintainers).

## How ZMP works, in one paragraph

ZMP is **lockstep** multiplayer: every player's PC runs the whole game, and all of them receive exactly the same
inputs in the same order, so every PC computes exactly the same world, frame by frame. The only thing in the middle is
a small **relay server**, `zmp-server`: it does not run the game, it only puts every player's buttons in order, passes
them on, and keeps each room's shared save. It is a single small program (a few MB, very little CPU and memory) that
one of the players can run on their own PC. There is no central ZMP server, no account and nothing to sign up for:
each group of friends uses its own server.
