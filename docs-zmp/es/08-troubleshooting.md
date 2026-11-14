# 8. Solución de problemas

[English](../en/08-troubleshooting.md) | **Español**

La ventana de ZMP muestra sus mensajes en tu idioma (español o inglés, en **Perfil > Idioma**), sin tildes. Aquí van
tal cual los muestra en español; la [guía en inglés](../en/08-troubleshooting.md) tiene los textos en inglés.

## Al entrar en una sala

| Mensaje | Qué hacer |
|---|---|
| "No se pudo conectar a ..." | La dirección o el puerto están mal, o no se llega al servidor. Comprueba que el anfitrión sigue alojando. Por internet, mira la línea "Desde internet" del anfitrión ([guía principal](00-host-and-send-the-code.md#qué-quiere-decir-la-línea-desde-internet)) y elige un plan B si hace falta. |
| "Contrasena incorrecta" | Vuelve a escribir la contraseña de la sala (distingue mayúsculas y minúsculas). El anfitrión la ve en la pestaña Alojar, o en la página del servidor con doble clic. |
| "Esta sala tiene contrasena: escribela y vuelve a conectar" | Escríbela en **Contrasena** (en Por direccion) y vuelve a conectar. Un código que lleva la contraseña nunca la pide. |
| "El anfitrion dejo una pista de la contrasena: ..." | El código solo lleva una pista: escribe la contraseña en **Contrasena** y vuelve a pulsar **Unirse con este codigo**. |
| "Este servidor solo deja crear salas con contrasena: escribe una" | Esa sala aún no existe y este servidor quiere contraseña: escribe una; será la contraseña de la sala. |
| "Demasiados intentos fallidos: espera 60 segundos" | Demasiadas contraseñas incorrectas desde tu dirección. Espera y escribe la buena. |
| "Ese servidor no tiene esa sala" | El servidor solo aloja su propia sala (un juego que está alojando, o un servidor con doble clic). Usa el código, o revisa el nombre de la sala. |
| "Tu version del juego no es la de la sala (la sala usa ..., tu tienes ...): todos necesitais la misma version. Actualiza el juego." | Todos deben usar exactamente la misma versión publicada (el mismo zip). Descarga la misma versión que el anfitrión. |
| "Esta sala es de otro motor (...): escritorio con escritorio, navegador con navegador" | Los jugadores de escritorio y los de navegador no pueden compartir sala. Usa el mismo tipo que la sala. |
| "Tu ROM no es la de la sala (la sala usa ..., tu ...): hace falta la misma version de la ROM" | Todos usan la misma versión de la ROM ([Tu ROM](02-your-rom.md)). |
| "Tus archivos del juego (soh.o2r) no son los de la sala: reinstala la misma version" | Vuelve a descomprimir la misma versión en una carpeta nueva. |
| "Tus opciones de juego bloqueadas no son las de la sala: usa la misma version" | Usa la misma versión que los demás, y deja las opciones "Enhancements" / "Cheats" con sus valores por defecto (lo más rápido: cierra el juego, borra `shipofharkinian.json` de la carpeta del juego y vuelve a abrirlo). |
| "La sala juega otra semilla del randomizer (...)" | Todos necesitan la misma semilla del randomizer. |
| "Ya hay alguien llamado ... en la sala: cambia tu nombre" | Elige otro nombre en **Perfil**. |
| "La sala esta llena (6 jugadores)" | Hasta 6 jugadores por sala. |
| "Este juego y el servidor hablan versiones distintas del protocolo ..." | Actualiza el más viejo: el juego o el servidor. |
| "El nombre tiene que tener de 1 a 32 caracteres" / "La sala tiene que tener de 1 a 64 caracteres" | Corrige el nombre o la sala. |
| "Esa direccion no es valida ..." | Vale: `host`, `host:puerto`, una IP, `[IPv6]:puerto`, `ws://...`, `wss://...`. |
| "Esa direccion responde, pero no es un servidor ZMP" | La dirección apunta a otra cosa (una página web, otro programa). Revisa la dirección y el puerto. |
| "El certificado del servidor no es valido: no se conecta (wss)" | El servidor `wss://` tiene un certificado caducado, hecho a mano o de otro nombre. Lo tiene que arreglar su dueño ([guía de wss](12-wss-for-browsers.md)). |
| "Este servidor tiene contrasena propia: escribela en Avanzado" / "La contrasena del servidor no es correcta" | Escribe la contraseña del servidor en **Avanzado > Contrasena del servidor (si tiene)**. |
| "Demasiadas conexiones desde tu direccion" / "El servidor no admite mas salas" | Límites de ese servidor (normalmente un relé). Prueba más tarde o usa otro servidor. |
| "La entrada tardo demasiado: vuelve a conectar" | Vuelve a conectar. |
| "El servidor no demostro conocer la contrasena de la sala: no es la sala que buscas" | El servidor de esa dirección no es el que hizo el código. Pide al anfitrión un código nuevo. |
| "Conexion perdida con el servidor: reintentando..." | El juego se reconecta solo. Si dura, el servidor se paró o se cortó la conexión. |

## Códigos de invitación

| Mensaje | Qué hacer |
|---|---|
| "El codigo no cuadra: revisa que este copiado entero y sin erratas" | Cópialo otra vez, entero. Los guiones y los espacios dan igual. |
| "Al codigo le falta o le sobra algo" / "El codigo tiene un caracter que no vale" | Falta parte del código o se pegó algo más. Cópialo otra vez. |
| "El codigo es de una version mas nueva: actualiza el juego" | El anfitrión tiene un ZMP más nuevo. Actualiza. |

## Al alojar

| Mensaje | Qué hacer |
|---|---|
| "No se encuentra zmp-server.exe junto al juego: vuelve a descargar la app." | Vuelve a descomprimir el zip entero; `zmp-server.exe` tiene que estar junto a `soh.exe`. |
| "El servidor no arranco: ..." / "El servidor se ha cerrado (codigo ...)" | Casi siempre el puerto está ocupado (otro servidor, o una segunda copia del juego alojando). Ciérralo, o elige otro puerto (por ejemplo `47101`). El registro del servidor es `zmp-host\zmp-server.log`, en la carpeta del juego. |
| La línea "Desde internet" | Consulta [qué significa](00-host-and-send-the-code.md#qué-quiere-decir-la-línea-desde-internet). |
| Los de casa no ven la partida en "En tu red local" | El Firewall de Windows tiene que permitir `soh.exe` y `zmp-server` en redes privadas, y todos tienen que estar en la misma red (algunas redes Wi-Fi de invitados aíslan los dispositivos). Pegar el código sigue funcionando. |

## En la partida

| Aviso | Qué significa |
|---|---|
| "Esperando a que alguien cargue una partida para crear el grupo" | El anfitrión (o cualquiera) tiene que cargar una partida. |
| "Entrando en la partida del grupo..." | Espera unos segundos. |
| "Sin conexion con el grupo: esperando a ..." / "Esperando a ..." | La conexión de alguien se paró más de un segundo. Se quita sola, o ese jugador se cae. |
| "DESYNC: resincronizado" | Los PC no coincidían y se corrigió. No hay que hacer nada; si pasa a menudo, envía los registros (abajo). |
| "Solo el anfitrion guarda la partida" | Solo el PC de quien creó el grupo escribe el archivo de partida. |

## Modo navegador

| Problema | Qué hacer |
|---|---|
| La página se queda en "Downloading..." o "Preparing..." | Espera (la primera carga descarga unas decenas de MB); luego recarga. Mira la consola del navegador (F12, Console). |
| "This file is compressed..." / "...isn't a supported version..." / "...doesn't match a known good one" | Elige la ROM `.z64`/`.n64`/`.v64` sin comprimir; un volcado modificado o malo se rechaza. |
| Aviso "This page was opened over https..." | Usa una dirección `wss://`, o abre la página por `http` (por ejemplo desde `zmp-server -web`). |
| Un aviso arriba de la página sobre `zmp-config.json` | La configuración del sitio tiene un valor que el juego no acepta; ese valor se ignora y lo demás funciona. Lo puede arreglar el dueño del sitio ([guía](10-web-version-on-your-site.md)). |
| "The game stopped unexpectedly" / la pestaña se cuelga | Normalmente es la memoria: cada pestaña necesita cerca de 1 GB. Cierra otras pestañas y programas. |
| Abrir `index.html` con doble clic no muestra nada | La página tiene que servirla un servidor web (`zmp-server -web`, o `python -m http.server`). |

## Servidor

- **La consola se cierra al momento**: ejecútalo desde una terminal para leer el error. "address already in use"
  significa que otro programa (u otra copia del servidor) usa el puerto 47100: ciérralo, o usa otro puerto
  (`listen = ":47101"` en `zmp-server.toml`, o `-listen :47101`). El servidor con doble clic espera a que pulses Intro
  antes de cerrarse cuando no puede arrancar.
- **Los amigos no llegan pero en tu PC funciona**: el router o el cortafuegos. Abre
  `http://<tu dirección pública>:47100/healthz` desde un móvil con datos (no con tu Wi-Fi): debería responder
  `zmp-server ... protocol 5`. Si no responde, desde internet no se llega al puerto
  ([ábrelo a mano](09-open-the-port-by-hand.md), u otro plan B).
- **Empezar una sala de nuevo**: usa un nombre de sala nuevo, o marca "Empezar la sala con mi partida" en **Avanzado**
  antes de conectarte.

## Enviar un informe de fallo

ZMP nunca envía nada por su cuenta: los registros se quedan en tu PC hasta que los envías tú. Abre una incidencia
(issue) en GitHub con:

- qué pasó, a qué hora y qué estabas haciendo;
- una captura de pantalla;
- la carpeta `logs` de la carpeta del juego de cada jugador (`logs\zmp.log` es el registro propio de ZMP);
- si alojabas desde el juego, `zmp-host\zmp-server.log`; con otro servidor, su consola o `docker compose logs`;
- para los jugadores de navegador, el texto de la consola (F12, Console, clic derecho, "Save as...").

Los registros contienen nombres de jugadores, el nombre de la sala, direcciones de servidores y rutas de archivos que
pueden incluir tu nombre de usuario de Windows: léelos antes de publicarlos. Nunca publiques un código de invitación
que lleve la contraseña.
