# 3. Play together

**English** | [Español](../es/03-play.md)

Before you start, the host of the group gives everyone three things:

- the **server address** (an IP like `203.0.113.10`, a name like `my-pc.tailnet-name.ts.net`, or a domain);
- the **port**, normally `47100`;
- the **room** name (any word you agree on; everyone types the same).

ZMP's own windows and messages inside the game are in **Spanish** for now (the rest of Ship of Harkinian is in
English). The labels you need are translated below.

## Connect (desktop)

When the game starts you see the **"Sala ZMP"** (ZMP room) window. If you don't (or you closed it): press **Esc** to
open the menu, go to **Network > ZMP** and press **"Mostrar u ocultar ventana de sala"** (show or hide the room
window). In the room window:

| Field (Spanish) | Meaning | What to type |
|---|---|---|
| Servidor (host) | server | the address the host gave you |
| Puerto | port | `47100` unless told otherwise |
| Sala | room | the room name, the same for everyone |
| Nombre | name | your name; different from everyone else's (it is shown over your Link) |
| Unirse a (opcional) | join (optional) | the name of a player you want to appear next to |

Then press **"Conectar"** (connect).

- **The first player (the room's creator)** loads their save file (or starts a new one), and once in the game presses
  **Conectar**. The first time, that save becomes the room's game: the server keeps it and updates it as you play.
- **Everyone else** presses **Conectar** from the file select screen (no need to load a save). In a few seconds you
  appear in the world, in the area where most players are (or next to the player you named in "Unirse a").

## How the co-op works

- **Each player has a real Link**, with their own controller and their own camera. Each Link has a slightly
  different tunic colour, and each name is shown over its Link.
- **Everyone in the same area sees exactly the same thing**: the same enemies, the same hits, the same world, frame
  by frame. Enemies go for whichever Link is nearest.
- **Yours**: health, magic, equipped items (sword, shield, tunic, boots), C buttons, ammunition and what is in your
  bottles.
- **Shared by everyone**: items found, rupees (one shared wallet), keys, heart containers and everything that changes
  the world (chests, doors, switches, the time of day). It reaches you at once even if you are in another area.
- **Go wherever you want**: leaving an area only moves you. Nobody waits for anybody. If friends are already in the
  area you enter, from then on you see exactly what they see.
- **Pause menu**: START opens yours and does not stop the game; the others keep playing (careful near enemies).
- **Falling and reviving**: with no fairy in a bottle, a Link with no health is **downed** instead of game over, and
  watches a friend's camera (C-left / C-right to switch). To revive a friend, stand next to their Link and **hold A
  for 3 seconds**. Only if **everyone** in the area is downed does the game over appear; the room's creator chooses
  continue for the group.
- **Talking, reading signs and the ocarina** are per player: each one has their own text box.
- **Chests and items**: the chest scene and the "You got..." message are only yours; the others get a short notice.
- **Boss and story scenes** are watched by everyone together. The room's creator can skip story scenes with START.
- **Boss doors** wait for every player in the dungeon so you go in together.
- **Friendly fire**: explosions (bombs, bombchus, bomb flowers) hurt every Link nearby, as in the original. Swords,
  arrows, the slingshot and the hookshot do not hurt other Links.
- **Warp songs and blue warps** take only the player who uses them; nearby friends get a notice for 10 seconds and
  whoever **holds L** for about a second goes along.
- **Age**: drawing or returning the Master Sword changes the age of the whole room.
- **Navi**: there is one Navi, the room creator's. The other Links target with Z without a fairy.
- **Disconnects**: if someone disconnects, their Link stays still. Reconnecting with the **same name** and the same
  room gets them back. A slow connection does not freeze the others: that Link repeats its last input until it catches
  up.
- **Desync**: if the PCs ever disagree, the game notices by itself and corrects it in a moment (you may see
  "DESYNC: resincronizado"). Nothing to do.

## Saving

- The **server** keeps each room's shared progress (items, rupees, chests, keys, what you have done, the time of day)
  as soon as it changes. If the server restarts, the windows reconnect by themselves within seconds and you carry on.
- Only the **room creator's PC** writes a save file (from the pause menu or the autosave, every 5 minutes by default;
  "Autoguardado cada (minutos)" in the room window). It also stores every friend's equipment by name.
- To come back exactly where you were: the room creator ticks **"Reanudar sesion"** (resume session) before
  connecting, in the same room; the others join as usual with the same names.
- To **start the room over** with another save: use another room name, or the creator ticks **"Empezar la sala con mi
  partida"** (start the room with my save) before connecting; that erases the server's progress for that room.
- Ship of Harkinian's quick saves (F5/F7) and its own autosave are off while you play in a group.

## Controls

The usual Ship of Harkinian controls. Keyboard by default: **WASD** move, **X** = A, **C** = B, **Z** = Z-target,
**Space** = START, **arrow keys** = C buttons, **E** / **R** = L / R. **Esc** opens the Ship of Harkinian menu, where
you can remap everything.

## Messages box and diagnostics

Group notices (who joins or leaves, who got what, saves, errors) appear in a box at the bottom left and fade out. In
the room window, section "Cuadro de mensajes", you can move it, resize it, change the font and colour, or hide it.
There you can also turn on the diagnostic line ("Name | tick 1234 | D=2 | HASH OK": your game and everyone else's
agree).

## Please don't

- Change gameplay options ("Enhancements", "Cheats", randomizer) differently from your friends: the room checks that
  everyone has the same gameplay settings. Leaving everything at the defaults is the safe choice.
- Use texture or model packs: they are not supported in multiplayer.

Something wrong? See [Troubleshooting](08-troubleshooting.md).
