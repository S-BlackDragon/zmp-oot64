# 2. Your ROM

ZMP, like Ship of Harkinian, ships **no part of the original game**. The graphics, sounds, music and text are built
on your PC from your own legally obtained copy of *The Legend of Zelda: Ocarina of Time* (a ROM dump). Nobody can send
you the ROM or the files made from it, and you must not share yours.

## The rule for multiplayer: everyone uses the same ROM version

Every player in a room must have the **same version** of the ROM (for example, all of you the European GameCube one,
or all of you the N64 NTSC 1.0). If yours is different, the game refuses to join and tells you, by name, which
version you have and which one the room uses. Agree on one version before you start.

Most of ZMP's automated testing used one version (the PAL GameCube debug ROM). Every other version that Ship of
Harkinian accepts loads the same way, but has had less testing with ZMP.

## Supported versions

The same list as Ship of Harkinian: N64 NTSC 1.0, 1.1 and 1.2; N64 PAL 1.0 and 1.1; GameCube NTSC-U, NTSC-J,
NTSC-J Collector's Edition and PAL; Master Quest NTSC-U, NTSC-J and PAL; and the PAL GameCube and Master Quest debug
ROMs. Accepted files: `.z64`, `.n64` or `.v64`, uncompressed (32, 54 or 64 MB). Not a `.zip`, `.7z` or `.rar`:
extract the ROM first. You can check your dump with the compatibility checker at <https://ship.equipment/>.

## Desktop game: once

1. Open `soh.exe`. The first time it says **"No O2R files found. Generate one now?"**: click **Yes** and choose your
   ROM.
2. It takes one or two minutes. It writes `oot.o2r` (or `oot-mq.o2r` for Master Quest) next to `soh.exe`; that file is
   the game data made from your ROM. When it asks **"Extract another?"**, click **No**.
3. That's it: next time the game starts directly. Your ROM file itself is not copied or changed.

Do not send your `oot.o2r` to friends: each player makes their own from their own ROM.

## Browser mode: each visit

The page asks for the ROM every time you open it (button **Choose ROM file**, or drop the file on the page). It is
read from your disk and converted inside the browser tab's memory in about a minute; nothing is uploaded and nothing
is stored in the browser. See [Browser mode](07-browser-mode.md).
