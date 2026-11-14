# 4. Plan B: un PC que solo aloja (zmp-server con doble clic)

[English](../en/04-host-on-your-pc.md) | **Español**

Lo normal es que el anfitrión pulse **Alojar y jugar** en el juego ([guía principal](00-host-and-send-the-code.md)).
Usa el servidor por separado cuando un PC deba alojar **sin jugar**: un PC de casa siempre encendido, un portátil que
no usas, un mini PC. Es el mismo `zmp-server` que usa el juego, con una pequeña página propia que muestra el código de
invitación.

El servidor nunca ejecuta el juego: solo ordena y reenvía los botones de todos y guarda las partidas compartidas de
las salas, así que casi no usa CPU, memoria ni ancho de banda.

## 1. Arrancarlo

1. Coge `zmp-server.exe` del zip de Windows (está junto a `soh.exe`), o descarga
   `zmp-server-<version>-windows-amd64.exe` de la versión publicada. Ponlo en **una carpeta solo para él**, por
   ejemplo `C:\Juegos\ZMP-servidor`: escribe sus archivos junto a sí mismo.
2. **Haz doble clic.** Se abren dos cosas:
   - una ventana de consola: es el registro del servidor. Déjala abierta mientras juegan; si la cierras, el servidor
     se para;
   - una página en tu navegador, en `http://127.0.0.1:<puerto>/`, con el código de invitación. Solo este PC puede
     abrir esa página. Si el navegador no se abre, la consola muestra la dirección de la página: ábrela a mano.
3. La primera vez, el **Firewall de Windows** pregunta si permites `zmp-server`: permítelo (en redes privadas, y
   también en públicas si tu red de casa está marcada como "Pública").

El primer arranque crea, junto al programa:

- `zmp-server.toml`: sus ajustes, con un **nombre de sala al azar** y una **contraseña al azar de 8 caracteres**;
- `data\`: las partidas compartidas de las salas y sus claves.

Después escucha en todas las tarjetas de red en el puerto **47100**, pide a tu router que abra ese puerto (UPnP,
NAT-PMP o PCP) y anuncia su sala en tu red local (así aparece en la lista **Unirse > En tu red local** del juego).

## 2. La página

| Parte | Qué hace |
|---|---|
| **Código de invitación** | El código, con **Copiar código** y **Copiar enlace**, y las direcciones que lleva dentro. Los amigos lo pegan en la pestaña Unirse del juego. |
| **Desde internet** | Si los amigos de internet pueden llegar a este PC, con consejos; los mismos mensajes que en el juego ([qué significan](00-host-and-send-the-code.md#qué-quiere-decir-la-línea-desde-internet)). |
| **Sala y contraseña** | **Sala**: cámbiale el nombre (**Cambiar**). **Contraseña**: **Ver**, **Cambiar**, **Nueva al azar**, **Quitar**. |
| **El código lleva** | **la contraseña** (los amigos entran sin escribir nada), **solo una pista** (escriben la contraseña; la pista les ayuda) o **nada (la piden al entrar)**. Pulsa **Guardar**. Elige "solo una pista" si vas a publicar el código en algún sitio público. |
| **Jugadores** | Quién está dentro, su estado y su ping. |
| **Parar** | Para el servidor (pregunta antes). |

La página tiene arriba un selector de idioma (Español / English).

Para jugar también en ese mismo PC: abre el juego y pega el código (o pulsa la sala en **En tu red local**).

**Parar**: pulsa **Parar** en la página, pulsa Ctrl+C en la consola o cierra la consola. El servidor quita la regla
que había pedido al router. Los jugadores se desconectan; el progreso de la sala se queda en `data\`.

Este servidor aloja **solo su propia sala**: quien pida otro nombre de sala recibe "Ese servidor no tiene esa sala".

## 3. El archivo de ajustes

`zmp-server.toml` es un archivo de texto; edítalo con el Bloc de notas **con el servidor parado**. La página ya te
cambia la sala, la contraseña y lo que lleva el código; lo demás está aquí. Una clave desconocida se avisa en el
registro y se ignora.

| Clave | Por defecto | Qué hace |
|---|---|---|
| `listen` | `":47100"` | Dónde escucha. `":47101"` para otro puerto; `"127.0.0.1:47100"` solo para este PC (sin router). |
| `data` | `"data"` | La carpeta de datos, relativa al archivo. |
| `server_name` | vacío | El nombre que se ve en la lista de la red local. |
| `room`, `password` | al azar | La sala y su contraseña (contraseña vacía = sin contraseña). |
| `invite_secret`, `hint` | `"password"` | Qué lleva el código: `"password"`, `"hint"` o `"none"`, y el texto de la pista. |
| `port_mapping` | `true` | Pedir al router que abra el puerto. `false` si lo abres a mano ([guía](09-open-the-port-by-hand.md)). |
| `public_address` | vacío | Un dominio, una IP pública fija o una dirección `wss://` que va **la primera** en el código. Úsala si abres el puerto a mano, tienes un dominio o pones un [proxy wss](12-wss-for-browsers.md) delante. |
| `open_browser` | `true` | Abrir la página al arrancar. |
| `ui_listen` | vacío | Dónde se sirve la página; siempre en `127.0.0.1`. |
| `lan_announce` | `true` | Anunciar la sala en tu red local (UDP, puerto 47100, nunca fuera de tu red). |
| `lan_port`, `lan_address` | `47100`, vacío | El puerto UDP del anuncio y, solo para pruebas, una dirección a la que mandarlo. |

Las claves de servidor de la sección siguiente (`require_room_password`, `server_password`, `max_rooms`, `relay`...)
también funcionan en este archivo.

## 4. Linux, una Raspberry Pi, o sin página

- En Linux (`zmp-server-<version>-linux-amd64`, o `-linux-arm64` para una Raspberry Pi 4/5), `zmp-server -app` hace lo
  mismo que el doble clic: el archivo de ajustes junto al programa, el router, el anuncio en la red local y la página
  en `127.0.0.1`. Añade `-no-browser` en una máquina sin escritorio (la consola muestra la dirección de la página);
  `-config <archivo>` lee otro archivo de ajustes.
- Con cualquier opción en la línea de órdenes (por ejemplo `zmp-server.exe -listen :47100`), el servidor funciona como
  servidor simple: sin archivo de ajustes junto a él, sin router, sin página y con cualquier nombre de sala. Es el modo
  para un VPS ([guía de Docker](06-host-on-a-vps.md)), un [relé](11-relay.md) y la
  [página del navegador](07-browser-mode.md) (`-web`).

Opciones del servidor simple. Cada una puede venir también de una variable de entorno o de un archivo indicado con
`-config <archivo>` (gana la línea de órdenes, luego la variable, luego el archivo):

| Opción | Variable | Clave del archivo | Por defecto |
|---|---|---|---|
| `-listen` | `ZMP_LISTEN` | `listen` | `:47100` |
| `-data` | `ZMP_DATA` | `data` | `./data` |
| `-name` | `ZMP_SERVER_NAME` | `server_name` | vacío |
| `-server-password-file` | `ZMP_SERVER_PASSWORD_FILE` o `ZMP_SERVER_PASSWORD` | `server_password_file` o `server_password` | sin contraseña de servidor |
| `-require-room-password` | `ZMP_REQUIRE_ROOM_PASSWORD` | `require_room_password` | `false` |
| `-max-rooms` | `ZMP_MAX_ROOMS` | `max_rooms` | 0 = sin límite |
| `-max-conns-per-ip` | `ZMP_MAX_CONNS_PER_IP` | `max_conns_per_ip` | 0 = sin límite |
| `-max-rooms-per-ip` | `ZMP_MAX_ROOMS_PER_IP` | `max_rooms_per_ip` | 0 = sin límite |
| `-list-rooms` | `ZMP_LIST_ROOMS` | `list_rooms` | `false` |
| `-room-key-ttl` | `ZMP_ROOM_KEY_TTL` | `room_key_ttl` (`30m`, `12h`, `7d`) | nunca |
| `-relay` | `ZMP_RELAY` | `relay` | `false` ([guía del relé](11-relay.md)) |
| `-web <carpeta>` | | | servir la [versión para navegador](07-browser-mode.md) en el mismo puerto |
| `-version` | | | muestra la versión y el número de protocolo |

En un servidor simple, el primer jugador que entra en una sala con contraseña fija la contraseña de esa sala; todos
los que vengan después la necesitan.

## 5. Contraseñas y seguridad

- Las contraseñas de sala nunca viajan en claro: el juego demuestra que conoce la contraseña, y el servidor solo guarda
  una clave derivada de ella (en `data\room_keys.json`).
- Una **contraseña de servidor** (`server_password` en el archivo, o `ZMP_SERVER_PASSWORD`; nunca en la línea de
  órdenes, donde otros programas podrían leerla) cierra el servidor entero: los jugadores la escriben en el juego en
  **Avanzado > Contrasena del servidor (si tiene)**.
- Tras 5 contraseñas incorrectas en un minuto desde una dirección, esa dirección espera 60 segundos ("Demasiados
  intentos fallidos").
- Los juegos antiguos (ZMP 0.6.x, protocolo 4) pueden seguir entrando en salas **sin** contraseña de este servidor, si
  tienen la misma versión que la sala. No pueden entrar en salas con contraseña ni en servidores con contraseña de
  servidor.
- Se usa un solo puerto TCP (47100 por defecto) para el juego, la versión para navegador y la comprobación
  `http://<dirección>:47100/healthz`.

## Cuántos jugadores y a qué distancia

Hasta **6 jugadores** por sala. Lockstep espera a la conexión más lenta, así que se juega mejor cuando todos están en
la misma región (el mismo país o países vecinos). El servidor ajusta el retraso de entrada a las conexiones cada pocos
segundos.
