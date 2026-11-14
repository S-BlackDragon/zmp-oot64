# Pulsa Alojar partida y pásale el código a tu amigo

[English](../en/00-host-and-send-the-code.md) | **Español**

Esta es la forma fácil de jugar con amigos, y la primera que conviene probar. Uno del grupo (el **anfitrión**)
pulsa un botón en el juego, recibe un **código de invitación** corto y se lo pasa a los demás. Ellos lo pegan y ya
están dentro. Sin cuentas y sin programas aparte que configurar: el juego trae todo lo necesario.

Antes de empezar, todos necesitan:

- **la misma versión de ZMP** (el mismo `ZMP-<version>-windows.zip`; [Obtener el juego](01-get-the-game.md));
- **la misma versión de la ROM** ([Tu ROM](02-your-rom.md));
- el anfitrión necesita un **PC con Windows**. Los amigos pueden estar en la misma casa o en cualquier parte de
  internet.

## La ventana de ZMP

Todo pasa en la ventana **"Sala ZMP"**. Se abre sola la primera vez que arrancas el juego. Si la cerraste: pulsa
**Esc**, ve a **Network > ZMP** y pulsa **"Mostrar u ocultar ventana de sala"**.

Tiene cuatro pestañas: **Unirse**, **Alojar**, **Perfil** y **Avanzado**. Su idioma sigue al de Windows (español o
inglés); puedes cambiarlo en **Perfil > Idioma**. El juego escribe sus textos sin tildes (su tipo de letra no las
tiene); aquí van tal cual se ven.

## Lo que hace el anfitrión

1. Abre `soh.exe` y quédate en la pantalla de selección de partida.
2. Pestaña **Perfil**: escribe tu nombre y pulsa **Cambiar nombre**. Cada jugador necesita un nombre distinto; se ve
   sobre su Link.
3. Pestaña **Alojar**, sección **Alojar partida**:
   - **Quien puede entrar**: deja **"Mi red local y quien tenga el codigo"**. (La otra opción, "Solo este PC
     (pruebas)", es para pruebas: no entra nadie más.)
   - **Sala** y **Contrasena de la sala**: déjalas vacías y el juego se inventa un nombre de sala y una contraseña.
     Escribe un nombre de sala propio si quieres volver otro día a la misma partida (consulta
     [Guardar](03-play.md#guardar)).
   - **Puerto**: deja `47100`.
   - **"Seguir alojando al cerrar el juego"**: déjalo sin marcar (así, al cerrar el juego también se deja de alojar).
4. Pulsa **Alojar y jugar**.
5. La primera vez, el **Firewall de Windows** puede preguntar si dejas que `zmp-server` (y `soh.exe`) usen la red:
   pulsa **Permitir acceso**. Si tu red de casa está marcada como "Pública" en Windows, marca también las redes
   públicas.
6. A los pocos segundos la pestaña muestra:
   - "Alojando la sala ... en el puerto 47100" y la **contraseña de la sala**;
   - **Codigo de invitacion (pasaselo a tus amigos)**: un código como `ZMP-0410-416B-01RG-...`, con los botones
     **Copiar codigo** y **Copiar enlace**;
   - **Desde internet**: si los amigos de fuera de tu casa pueden llegar (más abajo);
   - **Jugadores**: cuántos han entrado.
7. Pulsa **Copiar codigo** y pásaselo a tus amigos (chat, correo, mensaje). El código lleva tu dirección, la sala y su
   contraseña: **quien lo tenga puede entrar**, así que pásaselo solo a la gente con la que juegas.
8. Carga tu partida (o empieza una nueva). Esa partida pasa a ser la del grupo. Hasta entonces, arriba de la pantalla
   pone "Esperando a que alguien cargue una partida para crear el grupo".

Para terminar: pulsa **Dejar de alojar**, o cierra el juego. En los dos casos el juego para su servidor y quita la
regla que había pedido al router.

## Lo que hace el amigo

1. Abre `soh.exe` (la misma versión que el anfitrión) y quédate en la pantalla de selección de partida. **No cargues
   ninguna partida.**
2. Pestaña **Perfil**: escribe tu nombre y pulsa **Cambiar nombre**.
3. Pestaña **Unirse**, sección **Unirse con codigo**: pega el código en la casilla y pulsa **Unirse con este codigo**.
4. La pestaña dice "Codigo valido: sala ... en ...", luego "Conectando a ...", y después "Conectado a ... (sala ...,
   N jugadores)". Cuando la partida del anfitrión está en marcha apareces en el mundo junto al grupo (la pantalla
   pone "Entrando en la partida del grupo...").

Si algo falla, una línea en rojo dice por qué (contraseña incorrecta, otra versión...). Cada mensaje se explica en
[Solución de problemas](08-troubleshooting.md).

### En la misma casa

Los amigos de la misma red de casa también ven la partida en **Unirse > En tu red local**, con la sala, el nombre del
PC del anfitrión, los jugadores y si tiene contraseña. Para entrar desde esa lista, escribe la contraseña de la sala
(el anfitrión la ve en la pestaña Alojar) en la casilla **Contrasena** de **Por direccion**, y pulsa **Entrar** en esa
fila. Pegar el código también funciona, y no necesita nada más.

### Enlaces en vez de códigos

**Copiar enlace** da un enlace `zmp://join/...`. Solo abre el juego en los PC donde se ha permitido: en **Avanzado**,
pulsa una vez **"Abrir los enlaces zmp:// con este juego"** (se configura solo para tu usuario de Windows). Desde
entonces, al pulsar un enlace `zmp://` se abre el juego y entra en la sala; si el juego ya está abierto, el enlace va
a ese juego abierto. Algunas aplicaciones de chat no convierten los enlaces `zmp://` en pulsables: entonces copia el
código y listo.

## Qué quiere decir la línea "Desde internet"

El juego pide a tu router que abra el puerto (con UPnP, NAT-PMP o PCP, que tienen casi todos los routers de casa) y
luego dice con honestidad qué ha pasado:

| La línea dice | Qué significa | Qué hacer |
|---|---|---|
| "Preguntando al router…" | Aún espera la respuesta del router. | Espera unos segundos. |
| "Abierto por UPnP (o PCP, NAT-PMP): tu dirección pública es ..., puerto 47100." | El router abrió el puerto y tu dirección es una dirección de internet de verdad. | Pasa el código. Si un amigo aun así no entra, suele ser el Firewall de Windows: permite `zmp-server` en redes privadas y públicas. |
| "Se llega desde internet: ya ha entrado un jugador desde fuera de tu red." | Comprobado: alguien ha entrado por internet. | Nada. A jugar. |
| "Posible CGNAT: el router abrió el puerto por ..., pero su dirección exterior (...) no es pública." | Tu operadora (o un segundo router) comparte tu conexión, así que desde internet probablemente no se llega. | Los de tu casa sí pueden entrar. Para los demás, usa un plan B (abajo). |
| "Router sin UPnP: no respondió a UPnP, PCP ni NAT-PMP." | El router no ayudó (puede que tenga UPnP apagado en su configuración). | Enciende UPnP en la configuración del router y vuelve a alojar, o usa un plan B. |
| "El router respondió pero no abrió el puerto (...)." | El router se negó, a menudo porque otro equipo ya usa ese puerto. | Prueba otro puerto (por ejemplo `47101`) y vuelve a alojar, o usa un plan B. |
| "Solo este PC puede entrar (escucha en 127.0.0.1)." | Elegiste "Solo este PC". | Elige "Mi red local y quien tenga el codigo". |

El juego solo sabe lo que le dice tu router. "Abierto" es muy buena señal; la línea pasa a "Se llega desde internet"
en cuanto un amigo entra de verdad desde fuera.

Debajo de la línea, "Desde un navegador de tu red: http://192.168.1.20:47100/" muestra la dirección de tu PC en tu red
de casa (útil en algunas guías del plan B).

## Si los amigos de internet no llegan: plan B

Elige uno; todos funcionan con el mismo juego:

- [Abrir el puerto a mano en el router](09-open-the-port-by-hand.md) (cuando el router no tiene UPnP).
- [Tailscale o ZeroTier](05-host-with-a-virtual-lan.md): una red privada por internet; funciona con CGNAT y no hay
  que tocar el router.
- [Un relé](11-relay.md): un `zmp-server` que alguien tiene en internet; la sala se crea allí.
- [Un VPS con Docker](06-host-on-a-vps.md): un servidor pequeño de alquiler, siempre encendido.
- [Un PC que solo aloja](04-host-on-your-pc.md): el servidor solo, con doble clic y su propia página.

## Privacidad

- ZMP no sube nada y no tiene cuentas. El juego solo se conecta al servidor que escribes o al que lleva el código.
- Para abrir el puerto, el juego habla solo con **tu propio router**; tu dirección pública la da el router, no ningún
  servicio de fuera.
- Al alojar, la sala se anuncia **solo en tu red local** (así funciona "En tu red local").
- Tus registros se quedan en tu PC: la carpeta `logs` del juego y, cuando alojas, `zmp-host\zmp-server.log`. No se
  envía nada a ninguna parte salvo que lo envíes tú.
