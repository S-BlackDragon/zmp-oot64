# 1. Get the game

**English** | [Español](../es/01-get-the-game.md)

You need **Windows 10 or 11, 64-bit** for the desktop game, or a recent desktop browser (Chrome, Edge, Firefox) for
the [browser mode](07-browser-mode.md). A controller is recommended; the keyboard works too.

**Everyone in a room must use exactly the same build**: the same `soh.exe` (desktop) or the same `soh.wasm`
(browser). The game checks this when you join and refuses a different build. Desktop and browser players cannot
share a room (they are different builds), but one server can hold desktop rooms and browser rooms at the same time.

## Option A: download a release (easiest)

1. Open the repository's [latest release](https://github.com/S-BlackDragon/zmp-oot64/releases/latest) on GitHub and download:
   - `ZMP-<version>-windows.zip`: the desktop game. It already includes the room server (`zmp-server.exe`, used by
     the **Host and play** button) and `zmp-invite.exe` (makes invite codes by hand); that is all a group needs;
   - `ZMP-<version>-web.zip`: the browser version, only if you want the [browser mode](07-browser-mode.md);
   - `zmp-server-<version>-...` and `zmp-invite-<version>-...` (Windows, Linux x64, Linux ARM64): the server and the
     code tool on their own, only for a [PC that only hosts](04-host-on-your-pc.md), a [VPS](06-host-on-a-vps.md)
     or a [relay](11-relay.md);
   - the `.sha256.txt` files: checksums, to check a download if you want to.
2. Unzip `ZMP-<version>-windows.zip` into a new, normal folder (for example `C:\Games\ZMP`). Do not run it from
   inside the zip, and avoid `Program Files` (the game writes its files next to `soh.exe`).
3. Open `soh.exe`. If Windows shows "Windows protected your PC", click **More info**, then **Run anyway** (the
   program is not signed).
4. The first time it asks for your ROM: see [Your ROM](02-your-rom.md).
5. Then [press Host game and send the code to your friend](00-host-and-send-the-code.md).

The release contains no part of the original game: no ROM, no graphics, no sounds, no text from it.

## Option B: build it yourself

Everything is in this repository: the game (at the root: our fork of Ship of Harkinian with ZMP inside), the server
(`server/`) and the tools. Follow [Building from source](build.md). If you build it yourself, the friends you play
with need **your** build (send them your zip), because a build you make is not byte-identical to the release.

## Updating

When a new release comes out, everyone in the group updates together (the game refuses mismatched builds). To keep
your settings and saves, copy the new files over the old folder; or start in a new folder and copy `Save\`,
`shipofharkinian.json` and, if you host from the game, `zmp-host\` across. The shared progress of a room lives on the
server (for the **Host and play** button, in that `zmp-host` folder), so it is not lost when you update the game, as
long as the new version can still open it (the release notes will say if it cannot).
