# 10. Pon la versión web en tu sitio (y preconfigúrala)

[English](../en/10-web-version-on-your-site.md) | **Español**

La versión web es una carpeta de archivos (`index.html`, `soh.js`, `soh.wasm`, ...), de `ZMP-<version>-web.zip` o
[compilada por ti](build.md). Cualquier servidor web la sirve tal cual. Para que arranque como juega tu comunidad,
añade **un archivo junto a esos**: `zmp-config.json`. Nunca cambias el código del juego.

- **Sin `zmp-config.json`**: el juego no trae nada puesto. El jugador escribe un servidor (o pega un código de
  invitación), una sala y un nombre.
- **Con `zmp-config.json`**: el campo del servidor ya tiene el tuyo, y tú decides si el jugador puede cambiarlo, qué
  sala usar, cómo van las contraseñas, el nombre por defecto y el idioma.

Un valor que el juego no acepta se **ignora**, se dice en la consola del navegador y en un aviso arriba de la página,
y el resto del archivo se aplica igual. Un archivo roto nunca para el juego.

Tu servidor tiene que ser accesible desde el navegador: por `https://` eso quiere decir una dirección `wss://`
([wss:// para navegadores](12-wss-for-browsers.md)). Puede ser tu propio servidor, un [VPS](06-host-on-a-vps.md) o
un [relé](11-relay.md).

## Ejemplo

```json
{
  "version": 1,
  "servers": [{ "label": "Nuestro servidor", "address": "wss://zmp.example.org/", "default": true }],
  "lockServer": true,
  "room": { "default": "bosque" },
  "password": { "policy": "ask" },
  "name": { "default": "", "lock": false },
  "ui": { "language": "auto", "title": "Ocarina cooperativo" },
  "url": { "allow": ["room", "name", "lang", "autojoin", "invite", "pw"] },
  "updates": { "check": false }
}
```

Con este archivo, quien abre la página elige su ROM y entra directamente en la sala `bosque` de tu servidor (entra
sin preguntar porque el servidor está fijado y hay sala; solo pide contraseña si la sala la tiene).

La lista completa de claves, para editores que comprueban JSON, está en
[zmp-config.schema.json](../zmp-config.schema.json).

## Claves

| Clave | Valores | Qué hace |
|---|---|---|
| `version` | `1` | versión del formato |
| `servers` | lista de `{label, address, default}` | los servidores que sugieres; el que tenga `"default": true` (o el primero) rellena el campo del servidor. `address`: `wss://host/ruta` (una página servida por https necesita `wss://`), `ws://host:puerto/` o `host:puerto` |
| `lockServer` | `true` / `false` | `true`: el jugador no puede cambiar el servidor, y el juego entra sin preguntar si además hay sala |
| `room` | un nombre (1 a 64 caracteres), `"ask"`, o `{ "default": "...", "ask": true/false, "suggestRandom": true/false }` | la sala; `"ask"` deja el campo vacío para el jugador; `suggestRandom` se inventa una (`sala-xxxx`) |
| `password.policy` | `none`, `ask`, `required`, `given` | `none`: sin campo de contraseña; `ask`: campo opcional (por defecto); `required`: no conecta sin ella; `given`: la contraseña viene de `password.value` o de `#pw=` en la dirección, y no se muestra campo |
| `password.value` | texto, hasta 64 | la contraseña de `given` (quien lea tu configuración la ve: úsala solo para salas privadas porque nadie conoce la dirección) |
| `password.fromUrlFragment` | `true` / `false` | si se lee `#pw=` de la dirección (por defecto `true`) |
| `name.default` | texto, hasta 32 | el nombre ya escrito; el jugador puede cambiarlo |
| `name.lock` | `true` / `false` | el jugador no puede cambiar el nombre |
| `ui.language` | `auto`, `es`, `en` | idioma de las pantallas de multijugador (`auto`: el del navegador) |
| `ui.title` | texto, hasta 80 | el título de la página |
| `url.allow` | lista de `server`, `room`, `name`, `lang`, `autojoin`, `invite`, `pw` | qué parámetros de la dirección pueden cambiar los ajustes (todos si no está la clave) |
| `updates.check`, `updates.source` | `true`/`false`, una dirección `https://` | reservados para un futuro aviso de versión; esta versión no busca actualizaciones |
| `assets.remember`, `embed.parentOrigins` | `ask`/`never`, una lista | reservados para versiones futuras (se aceptan, aún no se usan) |

## Parámetros de la dirección

Mandan sobre el archivo (si `url.allow` los deja):

- `?server=wss://zmp.example.org/` — el servidor (se rechaza si `lockServer` es `true`).
- `?room=bosque` — la sala; una sala en la dirección entra directamente, como `?autojoin=1`.
- `?name=Saria`, `?lang=es`, `?autojoin=1` (o `0` para no entrar nunca solo).
- `#invite=ZMP-...` — un código de invitación: el juego entra con él. `#pw=...` — la contraseña de la sala.

Lo que va detrás de `#` nunca llega a ningún servidor, y la página lo borra de la barra al leerlo.

## Lo que el juego nunca hace

El código del juego no nombra ningún servidor ni sitio. Todo lo que tu página muestra sale de tu `zmp-config.json`;
una copia de la carpeta web sin él no trae nada puesto. La página lee `zmp-config.json` de su propio sitio y solo se
conecta al servidor que resulte; no sube nada.
