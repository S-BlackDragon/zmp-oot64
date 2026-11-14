# 3. Jugar juntos

[English](../en/03-play.md) | **Español**

La forma más rápida de entrar en una partida es un **código de invitación** del anfitrión: consulta
[Pulsa Alojar partida y pásale el código a tu amigo](00-host-and-send-the-code.md). Esta página explica todas las
formas de unirse y cómo funciona el cooperativo una vez dentro.

## La ventana de ZMP

La ventana **"Sala ZMP"** se abre sola la primera vez. Si la cerraste: pulsa **Esc**, ve a **Network > ZMP** y pulsa
**"Mostrar u ocultar ventana de sala"**.

Sus pestañas y mensajes siguen el idioma de Windows (español o inglés); se cambia en **Perfil > Idioma**. El juego
escribe sus textos sin tildes (su tipo de letra no las tiene). Algunas cosas solo están en español: los avisos del
grupo en pantalla y los ajustes de la parte de abajo de **Avanzado**.

- **Unirse**: entrar en una partida (con un código, desde tu red local o por dirección).
- **Alojar**: alojar una partida en este PC ([guía](00-host-and-send-the-code.md)).
- **Perfil**: tu **nombre** (se ve sobre tu Link; distinto del de todos los demás) y el idioma. Puedes cambiar el
  nombre en plena partida: pulsa **Cambiar nombre** y todos ven el nuevo al instante.
- **Avanzado**: el puerto por defecto, una contraseña de servidor, los enlaces `zmp://`, el autoguardado, reanudar,
  el cuadro de mensajes.

## Unirse a una partida

Quédate en la pantalla de selección de partida (no cargues ninguna), pon tu nombre en **Perfil** y usa una de las
tres secciones de **Unirse**:

1. **Unirse con codigo**: pega el código de invitación (`ZMP-...`) o un enlace `zmp://` y pulsa **Unirse con este
   codigo**. Si el anfitrión eligió poner en el código solo una pista de la contraseña, el juego muestra "El anfitrion
   dejo una pista de la contrasena: ...": escribe la contraseña en la casilla **Contrasena** (en Por direccion) y
   vuelve a pulsar el botón.
2. **En tu red local**: las partidas alojadas en tu red de casa aparecen aquí solas (sala, anfitrión, jugadores y si
   tiene contraseña). Si la tiene, escríbela antes en la casilla **Contrasena** de abajo y luego pulsa **Entrar** en
   esa fila.
3. **Por direccion**: para servidores cuya dirección conoces.
   - **Servidor**: lo que te dio el anfitrión. Vale: `host`, `host:puerto`, una IP, `[IPv6]:puerto`, `ws://...` o
     `wss://...`. Sin puerto, el juego usa el 47100 (o el que pongas en Avanzado).
   - **Sala**: el nombre de la sala, el mismo para todos.
   - **Contrasena (opcional)**: la contraseña de la sala, si tiene.
   - **Recientes**: los servidores y salas que usaste antes; pulsa uno para rellenar los campos, `x` para olvidarlo.
   - Pulsa **Conectar**.

Mientras estás en una sala, la pestaña Unirse muestra dónde estás, los jugadores con su estado y su conexión, y un
botón **Desconectar**.

**Quién empieza la partida**: el primer jugador que carga una partida (normalmente el anfitrión) crea el grupo; esa
partida pasa a ser la de la sala y el servidor la guarda. Todos los que están conectados en la pantalla de selección
de partida, o se conectan después, aparecen en el mundo en pocos segundos, en la zona donde está la mayoría.

## Cómo funciona el cooperativo

- **Cada jugador tiene un Link de verdad**, con su propio mando y su propia cámara. Cada Link tiene un color de túnica
  ligeramente distinto, y el nombre de cada uno se muestra sobre su Link.
- **Todos los que están en la misma zona ven exactamente lo mismo**: los mismos enemigos, los mismos golpes, el mismo
  mundo, fotograma a fotograma. Los enemigos van a por el Link que esté más cerca.
- **Es tuyo**: la salud, la magia, los objetos equipados (espada, escudo, túnica, botas), los botones C, la munición y
  lo que llevas en tus botellas.
- **Se comparte entre todos**: los objetos encontrados, las rupias (una cartera común), las llaves, los contenedores de
  corazón y todo lo que cambia el mundo (cofres, puertas, interruptores, la hora del día). Te llega al instante aunque
  estés en otra zona.
- **Ve adonde quieras**: salir de una zona solo te mueve a ti. Nadie espera a nadie. Si ya hay amigos en la zona a la
  que entras, desde ese momento ves exactamente lo que ven ellos.
- **Menú de pausa**: START abre el tuyo y no detiene el juego; los demás siguen jugando (cuidado cerca de enemigos).
- **Caer y reanimar**: sin un hada en una botella, un Link sin salud queda **caído** en lugar de provocar el fin de
  partida, y mira la cámara de un amigo (C-izquierda / C-derecha para cambiar). Para reanimar a un amigo, ponte junto
  a su Link y **mantén A durante 3 segundos**. Solo si **todos** los de la zona están caídos aparece el fin de partida;
  quien creó el grupo elige continuar para todos.
- **Hablar, leer carteles y la ocarina** son por jugador: cada uno tiene su propio cuadro de texto.
- **Cofres y objetos**: la escena del cofre y el mensaje "You got..." (has conseguido...) son solo tuyos; los demás
  reciben un aviso breve.
- **Las escenas de jefe y de historia** las ve todo el mundo a la vez. Quien creó el grupo puede saltarse las escenas
  de historia con START.
- **Las puertas de jefe** esperan a todos los jugadores de la mazmorra para que entren juntos.
- **Fuego amigo**: las explosiones (bombas, bombchus, flores bomba) hieren a todos los Link cercanos, como en el
  original. Las espadas, las flechas, la honda y el gancho no hieren a otros Link.
- **Las canciones de teletransporte y los portales azules** llevan solo al jugador que los usa; los amigos cercanos
  reciben un aviso durante 10 segundos y quien **mantenga L** durante cerca de un segundo se va con él.
- **Edad**: sacar o devolver la Espada Maestra cambia la edad de toda la sala.
- **Navi**: hay una sola Navi, la de quien creó el grupo. Los demás Link fijan objetivo con Z sin hada.
- **Desconexiones**: si alguien se desconecta, su Link se queda quieto. Al volver a conectarse a la misma sala desde
  el mismo juego, lo recupera (el juego guarda su propia clave de jugador, así que funciona aunque haya cambiado de
  nombre). Una conexión lenta no congela a los demás: ese Link repite su última entrada hasta que se pone al día.
- **Desincronización**: si alguna vez los PC no coinciden, el juego lo detecta solo y lo corrige en un momento (puede
  que veas "DESYNC: resincronizado"). No hay que hacer nada.

## Guardar

- El **servidor** guarda el progreso compartido de cada sala (objetos, rupias, cofres, llaves, lo que han hecho, la
  hora del día) en cuanto cambia. Si el servidor se reinicia, los juegos se reconectan solos en unos segundos y se
  sigue jugando. Cuando alojas desde el juego, los datos de ese servidor están en la carpeta `zmp-host` de la carpeta
  del juego.
- Solo el PC de **quien creó el grupo** escribe un archivo de partida (desde el menú de pausa o con el autoguardado,
  cada 5 minutos por defecto). También guarda el equipo de cada amigo por nombre.
- Los ajustes de esto están en la parte de abajo de **Avanzado**:

| Ajuste | Qué hace |
|---|---|
| Autoguardado cada (minutos, 0 = nunca) | cada cuántos minutos se guarda solo (0 = nunca) |
| Reanudar sesion | quien crea el grupo carga la última sesión guardada de esta sala |
| Unirse a (nombre de un jugador, opcional) | al conectar, apareces junto a ese jugador (opcional) |
| Empezar la sala con mi partida | empieza la sala de nuevo con tu partida (borra el progreso de la sala en el servidor) |

- Para volver exactamente donde estaban: aloja (o entra en) la **misma sala** que la última vez; quien crea el grupo
  marca **Reanudar sesion** antes de conectarse; los demás entran como siempre. (Si el anfitrión deja vacío el nombre
  de la sala, el juego se inventa uno nuevo cada vez: escribe uno en la pestaña Alojar para conservarlo.)
- Para **empezar la sala de nuevo** con otra partida: usa otro nombre de sala, o marca **Empezar la sala con mi
  partida** antes de conectarte.
- Los guardados rápidos de Ship of Harkinian (F5/F7) y su propio autoguardado están desactivados mientras se juega en
  grupo.

## Controles

Los controles habituales de Ship of Harkinian. Teclado por defecto: **WASD** para moverse, **X** = A, **C** = B, **Z** =
fijar objetivo (Z), **Espacio** = START, **teclas de flecha** = botones C, **E** / **R** = L / R. **Esc** abre el menú
de Ship of Harkinian, donde puedes reasignar todo.

## Cuadro de mensajes y diagnóstico

Los avisos del grupo (quién entra o sale, quién consiguió qué, guardados, errores) aparecen en un cuadro abajo a la
izquierda y se desvanecen. En la parte de abajo de **Avanzado** puedes ocultarlo ("Ocultar el cuadro de mensajes"),
cambiar cuántas líneas muestra ("Lineas visibles"), el color del texto ("Color del texto") y qué mensajes ver ("Que
mensajes ver"). Ahí también puedes activar la línea de diagnóstico de arriba ("Mostrar la linea de diagnostico";
"Nombre | tick 1234 | D=2 | HASH OK" quiere decir que tu juego y el de todos los demás coinciden).

## Por favor, no hagas esto

- Cambiar las opciones de juego ("Enhancements", "Cheats", randomizer) de forma distinta a tus amigos: la sala
  comprueba que todos tengan los mismos ajustes de juego. Dejar todo con los valores por defecto es lo más seguro.
- Usar paquetes de texturas o de modelos: no son compatibles en multijugador.

¿Algo va mal? Consulta [Solución de problemas](08-troubleshooting.md).
