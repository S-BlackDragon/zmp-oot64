# web/

**English** | [Español](README.es.md)

The browser version of ZMP is the same game as the desktop one, compiled to WebAssembly with Emscripten from
the repository (the game's code is at the root, as in Ship of Harkinian). This folder only has the script that turns that build into a folder you can serve:

```
python web/make_web.py --build <web build>/soh --out dist/web
```

- The page itself (connection form, local ROM picker, loading screen) is
  [`soh/web/shell.html`](../soh/web/shell.html); the ROM check and conversion is
  [`rom-setup.js`](../soh/web/rom-setup.js); what the page hands to the game is
  [`zmp-web.js`](../soh/web/zmp-web.js).
- The ROM is chosen by each player from their own computer, converted in the tab's memory and never uploaded or
  stored. The page has no default server: the player types it, or it is the page's own address when
  `zmp-server -web` serves the page.
- How to build: [docs-zmp/en/build.md](../docs-zmp/en/build.md#browser-game). How to play and host:
  [docs-zmp/en/07-browser-mode.md](../docs-zmp/en/07-browser-mode.md).
