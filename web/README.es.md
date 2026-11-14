# web/

[English](README.md) | **Español**

La versión para navegador de ZMP es el mismo juego que el de escritorio, compilado a WebAssembly con Emscripten desde
este repositorio (el código del juego está en la raíz, como en Ship of Harkinian). Esta carpeta solo tiene el script que convierte esa compilación en una carpeta que puedes servir:

```
python web/make_web.py --build <web build>/soh --out dist/web
```

- La página en sí (formulario de conexión, selector local de ROM, pantalla de carga) es
  [`soh/web/shell.html`](../soh/web/shell.html); la comprobación y conversión de la ROM es
  [`rom-setup.js`](../soh/web/rom-setup.js); lo que la página le entrega al juego es
  [`zmp-web.js`](../soh/web/zmp-web.js).
- Cada jugador elige la ROM desde su propio ordenador; se convierte en la memoria de la pestaña y nunca se sube ni
  se guarda. La página no tiene servidor por defecto: el jugador lo escribe, o es la dirección de la propia página cuando
  `zmp-server -web` la sirve.
- Cómo compilar: [docs-zmp/es/build.md](../docs-zmp/es/build.md#juego-para-navegador). Cómo jugar y hospedar:
  [docs-zmp/es/07-browser-mode.md](../docs-zmp/es/07-browser-mode.md).
