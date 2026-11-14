# 3. Play together

**English** | [Español](../es/03-play.md)

The quickest way into a game is an **invite code** from the host: see
[Press Host game and send the code to your friend](00-host-and-send-the-code.md). This page explains every way to
join, and how the co-op works once you are in.

## The ZMP window

The **"Sala ZMP"** window (the ZMP room window) opens by itself the first time. If you closed it: press **Esc**, go to
**Network > ZMP** and press **"Mostrar u ocultar ventana de sala"** (show or hide the room window).

Its tabs and messages follow your Windows language (Spanish or English); change it in **Profile > Language**. Some
things are still only in Spanish: the group notices on screen and the settings at the bottom of **Advanced**. They
are translated on this page.

- **Join**: get into a game (with a code, from your local network, or by address).
- **Host**: host a game on this PC ([guide](00-host-and-send-the-code.md)).
- **Profile**: your **name** (shown over your Link; different from everyone else's) and the language. You can change
  your name while playing: press **Change name** and everyone sees the new one at once.
- **Advanced**: the default port, a server password, `zmp://` links, autosave, resuming, the messages box.

## Join a game

Stay on the file select screen (do not load a save), set your name in **Profile**, then use one of the three
sections of **Join**:

1. **Join with a code**: paste the invite code (`ZMP-...`) or a `zmp://` link and press **Join with this code**.
   If the host chose to put only a hint of the password in the code, the game shows "The host left a hint for the
   password: ...": type the password in the **Password** field (under By address) and press the button again.
2. **On your local network**: games hosted on your home network appear here by themselves (room, host, players,
   whether it has a password). If it has one, type it in the **Password** field below first, then press **Join** in
   that row.
3. **By address**: for servers you know the address of.
   - **Server**: what the host gave you. Accepted: `host`, `host:port`, an IP, `[IPv6]:port`, `ws://...` or
     `wss://...`. Without a port, the game uses 47100 (or the one set in Advanced).
   - **Room**: the room name, the same for everyone.
   - **Password (optional)**: the room's password, if it has one.
   - **Recent**: the servers and rooms you used before; click one to fill the fields, `x` to forget it.
   - Press **Connect**.

While you are in a room, the Join tab shows where you are, the players with their state and connection, and a
**Disconnect** button.

**Who starts the game**: the first player who loads a save file (usually the host) creates the group; that save
becomes the room's game and the server keeps it. Everyone who is connected on the file select screen, or connects
later, appears in the world in a few seconds, in the area where most players are.

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
  for 3 seconds**. Only if **everyone** in the area is downed does the game over appear; the player who created the
  group chooses continue for everyone.
- **Talking, reading signs and the ocarina** are per player: each one has their own text box.
- **Chests and items**: the chest scene and the "You got..." message are only yours; the others get a short notice.
- **Boss and story scenes** are watched by everyone together. The player who created the group can skip story scenes
  with START.
- **Boss doors** wait for every player in the dungeon so you go in together.
- **Friendly fire**: explosions (bombs, bombchus, bomb flowers) hurt every Link nearby, as in the original. Swords,
  arrows, the slingshot and the hookshot do not hurt other Links.
- **Warp songs and blue warps** take only the player who uses them; nearby friends get a notice for 10 seconds and
  whoever **holds L** for about a second goes along.
- **Age**: drawing or returning the Master Sword changes the age of the whole room.
- **Navi**: there is one Navi, the one of the player who created the group. The other Links target with Z without a
  fairy.
- **Disconnects**: if someone disconnects, their Link stays still. Reconnecting to the same room from the same game
  gets them back (the game keeps its own player key, so it works even after a rename). A slow connection does not
  freeze the others: that Link repeats its last input until it catches up.
- **Desync**: if the PCs ever disagree, the game notices by itself and corrects it in a moment (you may see
  "DESYNC: resincronizado"). Nothing to do.

## Saving

- The **server** keeps each room's shared progress (items, rupees, chests, keys, what you have done, the time of day)
  as soon as it changes. If the server restarts, the games reconnect by themselves within seconds and you carry on.
  When you host from the game, that server's data lives in the `zmp-host` folder of your game folder.
- Only the PC of the **player who created the group** writes a save file (from the pause menu or the autosave, every
  5 minutes by default). It also stores every friend's equipment by name.
- The settings for this are at the bottom of **Advanced** (in Spanish):

| Setting (Spanish) | Meaning |
|---|---|
| Autoguardado cada (minutos, 0 = nunca) | autosave every N minutes (0 = never) |
| Reanudar sesion | resume session: the one who creates the group loads the last session saved for this room |
| Unirse a (nombre de un jugador, opcional) | join next to this player when you connect (optional) |
| Empezar la sala con mi partida | start the room over with my save (erases the room's progress on the server) |

- To come back exactly where you were: host (or join) the **same room name** as last time; the player who creates
  the group ticks **Reanudar sesion** before connecting; the others join as usual. (If the host leaves the room name
  empty, the game makes up a new one each time: type one in the Host tab to keep it.)
- To **start the room over** with another save: use another room name, or tick **Empezar la sala con mi partida**
  before connecting.
- Ship of Harkinian's quick saves (F5/F7) and its own autosave are off while you play in a group.

## Controls

The usual Ship of Harkinian controls. Keyboard by default: **WASD** move, **X** = A, **C** = B, **Z** = Z-target,
**Space** = START, **arrow keys** = C buttons, **E** / **R** = L / R. **Esc** opens the Ship of Harkinian menu, where
you can remap everything.

## Messages box and diagnostics

Group notices (who joins or leaves, who got what, saves, errors) appear in a box at the bottom left and fade out. At
the bottom of **Advanced** you can hide it ("Ocultar el cuadro de mensajes"), change how many lines it shows ("Lineas
visibles"), its text colour ("Color del texto") and which messages to see ("Que mensajes ver"). There you can also
turn on the diagnostic line at the top ("Mostrar la linea de diagnostico"; "Name | tick 1234 | D=2 | HASH OK" means
your game and everyone else's agree).

## Please don't

- Change gameplay options ("Enhancements", "Cheats", randomizer) differently from your friends: the room checks that
  everyone has the same gameplay settings. Leaving everything at the defaults is the safe choice.
- Use texture or model packs: they are not supported in multiplayer.

Something wrong? See [Troubleshooting](08-troubleshooting.md).
