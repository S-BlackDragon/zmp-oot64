# 7. Modo navegador

[English](../en/07-browser-mode.md) | **Español**

ZMP también funciona en un navegador web: el mismo juego y el mismo multijugador, compilados a WebAssembly. No hay
nada que instalar, y funciona en cualquier sistema de escritorio con un **Chrome, Edge o Firefox** reciente. Los
móviles y las tabletas no están soportados.

Lo que conviene saber:

- **Los jugadores de navegador juegan con jugadores de navegador.** Una compilación para navegador y el `soh.exe` de
  escritorio son compilaciones distintas, así que no pueden compartir sala ("Esta sala es de otro motor: escritorio con
  escritorio, navegador con navegador"). Un mismo servidor puede tener los dos tipos de sala a la vez.
- **Un navegador no puede alojar.** Los jugadores de navegador necesitan un servidor: uno que sirva la página (abajo),
  un [servidor con doble clic](04-host-on-your-pc.md), un [VPS](06-host-on-a-vps.md) o un [relé](11-relay.md).
- Todos en una sala de navegador deben cargar **la misma compilación web** (el mismo `soh.wasm`). La forma más fácil de
  asegurarlo es que todos abran la página desde el mismo sitio.
- Cada pestaña usa cerca de **1 GB de memoria**. Una pestaña por jugador.
- La página pide **tu ROM cada vez** que la abres. Se lee de tu disco y se convierte en la memoria de la pestaña (cerca
  de un minuto). No se sube nada y no se guarda nada de la ROM en el navegador. Todos deben usar la misma versión de la
  ROM, igual que en escritorio.
- El progreso de la sala vive en el servidor, igual que en escritorio. El navegador solo guarda los ajustes del juego.

## La forma fácil: el servidor sirve la página

1. Quien aloja descarga `ZMP-<version>-web.zip` de la versión publicada (o [lo compila](build.md)) y lo descomprime en
   una carpeta, por ejemplo `C:\Juegos\ZMP-web` (contiene `index.html`, `soh.js`, `soh.wasm`...).
2. Arranca el servidor con esa carpeta, desde una terminal en la carpeta del servidor:

   ```
   zmp-server.exe -web C:\Juegos\ZMP-web
   ```

   El registro dice `serving the browser game from C:\Juegos\ZMP-web on the same port`. (Docker: consulta
   [VPS](06-host-on-a-vps.md), sección 6.) Los amigos de internet necesitan que se llegue al puerto, como con
   cualquier servidor: [ábrelo a mano](09-open-the-port-by-hand.md) o usa
   [Tailscale/ZeroTier](05-host-with-a-virtual-lan.md).
3. Todos abren `http://<dirección del servidor>:47100/` en el navegador: el anfitrión puede usar
   `http://127.0.0.1:47100/`.
4. La página muestra **Join a game** (unirse a una partida): la dirección del servidor ya está puesta (es el propio
   servidor de la página). Escribe la **sala** (Room) y **tu nombre** (Your name).
5. Pulsa **Choose ROM file** (elegir archivo de ROM) o suelta tu ROM en la página. En cerca de un minuto el juego
   arranca y entra solo en la sala.

Para una sala con contraseña: abre la ventana de ZMP del juego (**Esc > Network > ZMP**), pestaña **Unirse**, escríbela
en **Contrasena** de **Por direccion** y pulsa **Conectar**. O ponla en la dirección de la página: `.../#pw=<contraseña>`.

## Entrar con un código de invitación

Los códigos de invitación también funcionan en el navegador:

- abre la página con el código detrás de `#invite=`, por ejemplo
  `http://<dirección de la página>/#invite=ZMP-0410-416B-...`: el juego entra con él en cuanto la ROM está lista;
- o, con el juego ya en marcha, pega el código en la ventana de ZMP: **Unirse > Unirse con codigo**.

Lo que va detrás de `#` en una dirección nunca llega a ningún servidor, y la página lo borra de la barra al leerlo.

## Otras formas de servir la página

La compilación para navegador es un sitio web estático normal: cualquier servidor web puede servir la carpeta, por
ejemplo `python -m http.server 8080` dentro de ella (luego abre `http://127.0.0.1:8080/`). En ese caso el campo del
servidor está vacío: escribe tú la dirección del servidor de salas (`host:puerto`, por ejemplo `192.168.1.20:47100`).

La página tiene que abrirse a través de un servidor web (`http://` o `https://`), no con doble clic en `index.html`
(`file://` no funciona).

Para dejar puestos tu servidor, la sala o la política de contraseñas a todos los que abran tu página, añade un
`zmp-config.json` junto a ella: [Pon la versión web en tu sitio](10-web-version-on-your-site.md).

### http, https y wss

- Una página abierta por **http** puede conectarse a cualquier dirección de servidor (`host:puerto`).
- Una página abierta por **https** solo puede usar WebSockets seguros: el servidor tiene que estar disponible en una
  dirección `wss://`. Escribe la dirección completa, como `wss://zmp.example.org/`. La página te avisa si lo que
  escribiste va a quedar bloqueado. Cómo montarlo: [wss:// para navegadores](12-wss-for-browsers.md).

### Formatos de dirección del servidor que acepta la página

`host` (puerto 47100), `host:puerto`, `[IPv6]:puerto`, o una dirección completa `ws://...` / `wss://...`. Déjalo vacío
para jugar solo; aún puedes conectarte después desde la ventana de ZMP del juego (Esc, Network, ZMP), igual que en
escritorio.

### Enlace con todo rellenado

La página también lee su dirección: `http://<dirección de la página>/?server=<servidor>&room=<sala>` rellena el
servidor y la sala y entra en cuanto la ROM está lista (`?host=<servidor>&port=47100` también funciona). Mejor no
pongas el nombre, para que cada amigo escriba el suyo.

## Compilaciones de navegador antiguas

Una compilación de navegador de ZMP 0.6.x (protocolo 4) puede seguir entrando en salas **sin** contraseña de un
servidor 0.7.0, con otros jugadores de esa misma compilación. Las salas con contraseña necesitan la 0.7.0.
