# 9. Registros de los jugadores en el servidor (opcional, apagado salvo que el servidor lo active)

Un servidor de salas puede guardar los registros (logs) de sus jugadores, para que cuando algo falle en un PC (una
desincronización, un cierre inesperado, un jugador que no pudo entrar) quien lleva el servidor pueda leer después lo
que pasó en cada PC, sin pedir a todos que busquen y envíen su carpeta `logs`.

**Viene apagado.** Un `zmp-server` solo recoge registros si su dueño lo arranca con una carpeta de registros (más
abajo). El juego envía entonces sus registros **solo a ese servidor**, y solo después de que el servidor haya dicho, al
conectarte, que los recoge. Un servidor arrancado sin carpeta de registros nunca recibe nada: el juego ni lo intenta.

## Para jugadores

### Cómo lo sabes

Al conectarte, el juego escribe una de estas líneas en `logs\zmp.log`:

- `logship: this server does not collect logs: nothing is sent to it` (este servidor no recoge registros: no se le
  envía nada)
- `logship: the server collects logs: this run's logs go to it every 180 s ...` (el servidor los recoge: se le envían
  cada 180 s)
- `logship: the server collects logs, but the setting gZmp.Logs.Upload is off: nothing is sent` (los recoge, pero tienes
  el envío desactivado: no se envía nada)

### Qué se envía

- Escritorio: `logs\zmp.log` (las líneas de ZMP) y `logs\Ship of Harkinian.log` (el registro del juego), solo la parte
  escrita por la ejecución actual del juego; el archivo `logs\desync-<tick>.txt` de una desincronización, si tu juego
  lo escribió; y, tras un cierre inesperado, la parte de los registros de la ejecución anterior que aún no se había
  enviado.
- Navegador: el registro de ZMP de esa pestaña.
- Cada envío lleva solo lo que el servidor aún no tiene (se añade al final), como mucho 2 MB cada vez.
- Nunca se envía: tu ROM ni nada sacado de ella, tus partidas guardadas, capturas, ajustes ni ningún otro archivo.

### Cuándo

Al conectarte (lo que quedara pendiente), cada 3 minutos mientras estás conectado, al terminar una sesión, en una
desincronización y tras un cierre inesperado (desde el gestor de errores, y otra vez al arrancar la próxima vez para lo
que no saliera). El envío va en su propio hilo (en el navegador, una petición en segundo plano): el juego nunca lo
espera, y un envío fallido solo se anota en tu registro y se reintenta más tarde.

### Adónde

A la misma dirección y puerto a los que te conectaste, como un `POST` HTTP a `/zmp/logs` en ese puerto, con un token
que el servidor dio a esa conexión. Por `http` normal (el juego de escritorio, o una página del navegador conectada con
`ws://`) los registros viajan sin cifrar, igual que el tráfico del propio juego; con `wss://` van por HTTPS.

### No enviarlos

Desmarca **"Enviar mis registros al servidor si los recoge (para que puedan ver los fallos)"** en la ventana de sala,
debajo de los campos de conexión. Tiene efecto al momento y se recuerda (`gZmp.Logs.Upload` = 0 en
`shipofharkinian.json`). La versión de navegador tiene la misma casilla en la ventana de sala del juego.

### Privacidad

Los registros contienen tu nombre de jugador, el nombre de la sala, la dirección del servidor, datos de tu PC como la
tarjeta gráfica, y rutas de archivos que pueden incluir tu nombre de usuario de Windows. El servidor además anota la
dirección IP desde la que te conectaste en el registro de eventos de la sala. Solo puede leerlos quien lleva el servidor
(y quien pueda leer el disco de ese PC o VPS). Deja la casilla marcada solo en servidores de alguien de confianza.

## Para quien lleva el servidor

### Activarlo

Dale al servidor una carpeta para los registros:

```powershell
zmp-server.exe -listen :47100 -data C:\Games\ZMP-server\data -log-dir C:\Games\ZMP-server\logs
```

o pon la variable de entorno `ZMP_LOG_DIR` con esa carpeta. El registro del servidor dice entonces
`zmp logs: ON, store <carpeta> (...)`. Sin `-log-dir` (y sin `ZMP_LOG_DIR`) la recogida está apagada, el servidor no la
anuncia y un envío a `/zmp/logs` recibe un `404`.

Con Docker (`server/deploy/docker-compose.yml`), quita el comentario del volumen marcado `./logs:/logs` y de la línea
`ZMP_LOG_DIR`, crea la carpeta (`mkdir -p logs && sudo chown 1000:1000 logs`) y ejecuta `docker compose up -d`.

Para apagarlo, arranca el servidor otra vez sin la opción. Para borrarlo todo, borra la carpeta.

**Avisa a tus jugadores** de que tu servidor guarda sus registros, guárdalos solo mientras los necesites y no los
publiques (ver Privacidad arriba).

### Límites

| Opción (variable de entorno) | Por defecto | Qué |
|---|---|---|
| `-log-max-total` (`ZMP_LOG_MAX_TOTAL`) | `2G` | tamaño de toda la carpeta; al pasarlo se borran los archivos más antiguos hasta quedar al 90 % |
| `-log-max-upload` (`ZMP_LOG_MAX_UPLOAD`) | `8M` | el envío más grande |
| `-log-max-per-day` (`ZMP_LOG_MAX_PER_DAY`) | `128M` | lo que puede enviar un nombre de jugador en un día UTC |

Todos los jugadores juntos pueden enviar como mucho 1 GiB por día UTC. Los tamaños admiten un número de bytes o `512M`,
`2G`.

### Qué hay en la carpeta

```
sessions/<sala>/<sesión>/server-events.log               entradas, salidas y desincronizaciones de esa sesión de la sala
sessions/<sala>/<sesión>/p<id>-<nombre>/<run>-<tipo>.log  el registro de un jugador en una ejecución del juego
server/zmp-server-<aaaammdd>.log                          una copia del registro del propio servidor, un archivo por día
```

El servidor pone nombre a cada archivo él mismo, con lo que sabe de la conexión (sala, sesión, id y nombre del
jugador), nunca con lo que envía el cliente, y limpia cada nombre. Un token vale para el jugador de una conexión y
caduca a las 72 horas.
