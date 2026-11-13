# 7. Modo navegador

ZMP también funciona en un navegador web: el mismo juego y el mismo multijugador, compilados a WebAssembly. No hay
nada que instalar, y funciona en cualquier sistema operativo de escritorio con un **Chrome, Edge o Firefox** reciente.
Los teléfonos y las tabletas no son compatibles.

Cosas que debes saber:

- **Los jugadores de navegador juegan con jugadores de navegador.** Una compilación de navegador y el `soh.exe` de
  escritorio son compilaciones distintas, así que no pueden compartir sala. Un mismo servidor puede alojar ambos tipos
  de sala a la vez.
- Todos en una sala de navegador deben cargar **la misma compilación web** (el mismo `soh.wasm`). La forma más fácil de
  garantizarlo es que todos abran la página desde el mismo sitio (el servidor del anfitrión).
- Cada pestaña usa aproximadamente **1 GB de memoria**. Una pestaña por jugador.
- La página te pide **tu ROM cada vez** que la abres. Se lee desde tu disco y se convierte en la memoria de la pestaña
  (aproximadamente un minuto). No se sube nada y no se guarda nada de la ROM en el navegador. Todos deben usar la misma
  versión de la ROM, como en escritorio.
- Las partidas de la sala viven en el servidor, como en escritorio. El navegador solo conserva los ajustes del juego.

## La forma fácil: el servidor del anfitrión sirve la página

1. El anfitrión descarga `ZMP-<version>-web.zip` de la versión publicada (o [lo compila](build.md)) y lo descomprime en
   una carpeta, por ejemplo `C:\Games\ZMP-web` (contiene `index.html`, `soh.js`, `soh.wasm`...).
2. El anfitrión inicia el servidor con esa carpeta, desde una terminal en la carpeta del servidor:

   ```
   zmp-server.exe -web C:\Games\ZMP-web
   ```

   El registro dice `serving the browser game from C:\Games\ZMP-web on the same port`. (Docker: consulta
   [Hospedar en un VPS](06-host-on-a-vps.md), sección 4.)
3. Todos abren `http://<dirección del servidor>:47100/` en el navegador: el anfitrión puede usar
   `http://127.0.0.1:47100/`, y los amigos usan la dirección que escribirían en el juego de escritorio (la dirección
   pública con [redirección de puertos](04-host-on-your-pc.md), o la dirección de [Tailscale/ZeroTier](05-host-with-a-virtual-lan.md)).
4. La página muestra **Join a game** (unirse a una partida): la dirección del servidor ya viene rellenada (es el
   propio servidor de la página). Escribe la **sala** y **tu nombre**.
5. Haz clic en **Choose ROM file** (elegir archivo de ROM), o suelta tu ROM sobre la página. Después de
   aproximadamente un minuto el juego arranca y se conecta solo a la sala.

La línea superior del juego (si la línea de diagnóstico está activada) dice "Conectado a ws://... (sala <sala>, N
jugadores)".

## Otras formas de servir la página

La versión para navegador es un sitio web estático simple: cualquier servidor web puede servir la carpeta, por ejemplo
`python -m http.server 8080` dentro de ella (luego abre `http://127.0.0.1:8080/`). En ese caso el campo del servidor
está vacío: escribe tú mismo la dirección del servidor de salas (`host:puerto`, por ejemplo `192.168.0.20:47100`).

La página debe abrirse a través de un servidor web (`http://` o `https://`), no haciendo doble clic en `index.html`
(`file://` no funciona).

### http, https y wss

- Una página abierta por **http** puede conectarse a cualquier dirección de servidor (`host:puerto`).
- Una página abierta por **https** solo puede usar WebSockets seguros: el servidor debe ser accesible en una dirección
  `wss://` (por ejemplo detrás del proxy Caddy de [Hospedar en un VPS](06-host-on-a-vps.md)). Escribe la dirección
  completa, como `wss://play.example.org/`. La página te avisa si lo que escribiste va a ser bloqueado.

### Formatos de dirección de servidor que acepta la página

`host` (puerto 47100), `host:puerto`, `[IPv6]:puerto`, o una dirección completa `ws://...` / `wss://...`. Déjalo vacío
para jugar a solas; aún puedes conectarte después desde el menú del juego (Esc, Network, ZMP), como en escritorio.

### Enlace con todo rellenado

La página también lee la dirección desde su URL, así que el anfitrión puede enviar un enlace listo:
`http://<dirección de la página>/?host=<servidor>&port=47100&room=<sala>&name=<nombre>` (es mejor omitir la parte del
nombre, para que cada amigo escriba el suyo).
