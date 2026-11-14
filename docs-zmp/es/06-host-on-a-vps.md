# 6. Plan B: un VPS con Docker (siempre encendido)

[English](../en/06-host-on-a-vps.md) | **Español**

Un pequeño servidor Linux de alquiler (un VPS) mantiene el servidor de salas encendido todo el tiempo, así que el grupo
puede jugar sin que el PC de nadie aloje, y se llega desde cualquier sitio (sin router ni problemas de CGNAT).
`zmp-server` necesita muy poco: basta con el VPS más pequeño de cualquier proveedor (nunca ejecuta el juego). Elige uno
en la misma región que tu grupo.

## 1. Preparar el VPS

Necesitas un VPS con Linux (Debian o Ubuntu son los más fáciles) y Docker. En un VPS Debian/Ubuntu recién creado, con
la sesión abierta por SSH:

```bash
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER     # luego cierra la sesión y vuelve a entrar
```

Abre el **puerto TCP 47100** en el cortafuegos del proveedor (la página "security group" o "firewall" del VPS) y, si
el VPS tiene su propio cortafuegos (`ufw`), también ahí: `sudo ufw allow 47100/tcp`.

## 2. Arrancar el servidor

```bash
git clone -b zmp --depth 1 https://github.com/S-BlackDragon/zmp-oot64.git
cd zmp-oot64/server/deploy
mkdir -p data && sudo chown 1000:1000 data    # el contenedor escribe las partidas como el usuario 1000
docker compose up -d --build                  # compila la imagen desde el código (cerca de un minuto) y la arranca
docker compose logs -f zmp-server             # Ctrl+C para dejar de mirar; el servidor sigue en marcha
```

Deberías ver `listening on [::]:47100 (tcp and websocket)`. Compruébalo desde tu PC: abre
`http://<dirección del VPS>:47100/healthz` en un navegador; responde `zmp-server <version> protocol 5`.

El contenedor está cerrado a cal y canto: se ejecuta como usuario normal, su sistema de archivos es de solo lectura
salvo `data/`, no tiene privilegios extra y está limitado a 512 MB de memoria y una CPU. Se vuelve a arrancar solo si
el VPS se reinicia.

## 3. Salas, contraseñas y el código de invitación

Es un servidor simple: vale cualquier nombre de sala, y **el primer jugador que entra en una sala con contraseña la
fija**. Así que quien organiza el grupo:

1. en el juego, **Unirse > Por direccion**: **Servidor** = la dirección del VPS (su IP o un dominio que apunte a él),
   **Sala** = un nombre difícil de adivinar, **Contrasena** = una contraseña; **Conectar**;
2. da a los amigos esas mismas tres cosas, o un **código de invitación** hecho con `zmp-invite` (va en el zip de
   Windows, y en la versión publicada para Linux):

```
zmp-invite encode -addr 203.0.113.10 -room bosque-k3f9 -password inventate-una
```

Muestra el código (`ZMP-...`) y el enlace `zmp://`. Añade `-port <puerto>` si el servidor no está en el 47100, o usa
`-hint "..."` en vez de `-password` para poner en el código solo una pista. `zmp-invite decode <código>` muestra qué
lleva un código.

Los ajustes del servidor van en la parte `environment:` del servicio `zmp-server` de
`server/deploy/docker-compose.yml` (todos están en la
[tabla de opciones](04-host-on-your-pc.md#4-linux-una-raspberry-pi-o-sin-página)), por ejemplo:

```yaml
    environment:
      ZMP_SERVER_NAME: "Nuestro servidor"
      ZMP_REQUIRE_ROOM_PASSWORD: "true"     # ninguna sala sin contraseña
```

y luego `docker compose up -d`. Para que también desconocidos puedan crear salas (un relé público), consulta
[Relé](11-relay.md).

## 4. Órdenes útiles

Desde `zmp-oot64/server/deploy`:

```bash
docker compose ps                   # ¿está en marcha?
docker compose logs --tail 100      # registro reciente
docker compose restart              # reiniciar (los jugadores se reconectan solos)
docker compose down                 # parar
git pull && docker compose up -d --build   # actualizar a la última versión
```

Las partidas compartidas de las salas y sus claves están en `server/deploy/data/`; haz copia de esa carpeta si te
importan.

## 5. Sin Docker

Copia `zmp-server-<version>-linux-amd64` (o `-linux-arm64`) de una versión publicada al VPS y ejecútalo con
`./zmp-server-<version>-linux-amd64 -listen :47100 -data ./data`. Para que siga en marcha al cerrar la sesión, usa una
unidad de systemd, `tmux` o `screen`.

## 6. Jugadores de navegador en el VPS

Para servir la versión para navegador desde el VPS, pon la compilación para navegador (de `ZMP-<version>-web.zip`, o
compilada por ti) en `server/deploy/web/` y quita el comentario de las dos líneas marcadas en
`server/deploy/docker-compose.yml` (el volumen `./web:/web:ro` y el `command` con `-web /web`); luego
`docker compose up -d`. Los amigos abren `http://<dirección del VPS>:47100/`.

Esa página es `http` normal. Funciona, pero algunos navegadores avisan, y una página abierta por `https` solo puede
conectarse con `wss://`. Para una dirección `https://` limpia, pon un proxy TLS delante: consulta
[wss:// para navegadores](12-wss-for-browsers.md).
