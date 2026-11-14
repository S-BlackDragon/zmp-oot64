# 6. Hospedar en un VPS con Docker (siempre encendido)

Un pequeño servidor Linux alquilado (un VPS) mantiene el servidor de salas encendido todo el tiempo, así que el grupo
puede jugar sin que el PC de nadie haga de anfitrión. `zmp-server` necesita muy poco: basta el VPS más pequeño de
cualquier proveedor (nunca ejecuta el juego). Elige uno en la misma región que tu grupo.

## 1. Preparar el VPS

Necesitas un VPS Linux (Debian o Ubuntu son lo más fácil) con Docker. En un VPS Debian/Ubuntu recién creado, con la
sesión iniciada por SSH:

```bash
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER     # then log out and back in
```

Abre el **puerto TCP 47100** en el cortafuegos del proveedor (la página de "grupo de seguridad" o "firewall" del VPS)
y, si el VPS tiene su propio cortafuegos (`ufw`), también ahí: `sudo ufw allow 47100/tcp`.

## 2. Obtener los archivos del servidor

```bash
git clone -b zmp --depth 1 https://github.com/S-BlackDragon/zmp-oot64.git
cd zmp-oot64/server/deploy
mkdir -p data && sudo chown 1000:1000 data    # the container writes the saves as user 1000
docker compose up -d --build                  # builds the image from source (about a minute) and starts it
docker compose logs -f zmp-server             # Ctrl+C to stop watching; the server keeps running
```

Deberías ver `listening on [::]:47100 (tcp and websocket)`. Compruébalo desde tu propio PC: abre
`http://<dirección del VPS>:47100/healthz` en un navegador; responde `zmp-server <version> protocol 4`.

Los amigos escriben la dirección del VPS (su IP o un nombre de dominio que apunte a él), el puerto `47100` y el
nombre de la sala.

El contenedor está blindado: se ejecuta como un usuario normal, su sistema de archivos es de solo lectura salvo
`data/`, no tiene privilegios adicionales y está limitado a 512 MB de memoria y una CPU. Se reinicia solo si el VPS se
reinicia.

Comandos útiles (desde `zmp-oot64/server/deploy`):

```bash
docker compose ps                   # is it running?
docker compose logs --tail 100      # recent log
docker compose restart              # restart (players reconnect by themselves)
docker compose down                 # stop
git pull && docker compose up -d --build   # update to the latest version
```

Las partidas compartidas de las salas están en `server/deploy/data/`; haz una copia de seguridad de esa carpeta si te
importan.
Para guardar también los registros de tus jugadores en el servidor (opcional, apagado por defecto), mira
[Registros de los jugadores en el servidor](09-player-logs.md).

## 3. Sin Docker

Copia `zmp-server-<version>-linux-amd64` (o `-linux-arm64`) desde una versión publicada al VPS y ejecútalo con
`./zmp-server-<version>-linux-amd64 -listen :47100 -data ./data`. Para mantenerlo en marcha después de cerrar sesión, usa una
unidad de systemd, `tmux` o `screen`.

## 4. Modo navegador en el VPS

Para que los amigos jueguen en el navegador, pon la versión para navegador (de `ZMP-<version>-web.zip`, o compilada por
ti) en `server/deploy/web/` y descomenta las dos líneas marcadas de `server/deploy/docker-compose.yml` (el volumen
`./web:/web:ro` y el `command` con `-web /web`); después, `docker compose up -d`. Los amigos abren
`http://<dirección del VPS>:47100/`.

Esa página es `http` simple. Funciona, pero algunos navegadores avisan de ello, y una página abierta por `https` solo
puede conectarse con `wss://`. Para tener una dirección `https://` limpia, pon delante un proxy inverso con TLS, por
ejemplo [Caddy](https://caddyserver.com/) con un nombre de dominio que apunte al VPS. Añade este servicio al mismo
`docker-compose.yml`:

```yaml
  caddy:
    image: caddy:2
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - ./caddy-data:/data
```

y crea `server/deploy/Caddyfile` con tu dominio:

```
play.example.org {
    reverse_proxy zmp-server:47100
}
```

Abre los puertos 80 y 443 en el cortafuegos y después `docker compose up -d`. Caddy obtiene un certificado gratuito por
sí solo. Los jugadores de navegador abren `https://play.example.org/`; la página rellena `wss://play.example.org/` como
servidor. Los jugadores de escritorio siguen usando directamente la dirección y el puerto `47100` (el juego de
escritorio habla TCP simple, que no pasa por el proxy).

La configuración de Caddy de arriba es un ejemplo estándar que el proyecto no ha probado de extremo a extremo; la
configuración con `http` simple (`-web` en el puerto 47100) es la que cubren las pruebas del servidor.
