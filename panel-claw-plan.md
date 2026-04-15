# panel-claw — Plan maestro

> Indicator para Wingpanel que permite hacer cualquier pregunta desde el panel de elementary OS.
> Ejemplo de flujo: el usuario escribe _"encuentra el PDF con nombre contrato.pdf y dame un resumen"_ →
> el gateway ejecuta las tools necesarias (glob, pdf_text, read_file) → el indicador muestra
> una tarjeta con el path del archivo y el resumen del contenido.

---

## Arquitectura de alto nivel

```
┌─────────────────────────────────────────┐
│          panel-claw (Vala / GTK4)        │
│                                          │
│  [Wingpanel] → [ChatPopover]             │
│      icon          ├─ Gtk.Entry          │
│   + status dot     ├─ ResultList         │
│                    │    └─ FileCard      │
│                    │    └─ TextCard      │
│                    └─ Spinner            │
└──────────────┬──────────────────────────┘
               │ HTTP POST (libsoup-3.0)
               ▼
┌─────────────────────────────────────────┐
│       claw gateway  :4389  (Go)          │
│                                          │
│  /v1/chat/completions                    │
│  tools: glob, read_file,                 │
│         grep_search, list_dir,           │
│         pdf_text  ← nuevo               │
│                                          │
│  workspaceRoot = $HOME                   │
└─────────────────────────────────────────┘
```

---

## Fases de trabajo

---

### Fase 0 — Verificación pre-requisitos en VM
**Objetivo:** confirmar que las dependencias necesarias están disponibles antes de escribir código.

- [ ] **0.1** — Verificar `wingpanel-9` pkg-config en la VM:
  ```bash
  pkg-config --modversion wingpanel-9
  ```
- [ ] **0.2** — Verificar `libsoup-3.0` (cliente HTTP async):
  ```bash
  pkg-config --modversion libsoup-3.0
  ```
- [ ] **0.3** — Verificar `poppler-utils` (`pdftotext` para extraer texto de PDFs):
  ```bash
  which pdftotext
  ```
  Si no está: `sudo apt install poppler-utils`
- [ ] **0.4** — Verificar `json-glib-1.0` (parseo JSON en Vala):
  ```bash
  pkg-config --modversion json-glib-1.0
  ```

---

### Fase 1 — Scaffold del proyecto `panel-claw`
**Objetivo:** crear la estructura mínima del indicador que compile y cargue en Wingpanel (aunque haga nada).

Archivos a crear:

```
panel-claw/
├── meson.build
├── data/
│   └── icons/
│       └── io.elementary.panel.claw.svg   ← ícono del indicador
└── src/
    └── Indicator.vala
```

- [ ] **1.1** — Crear `panel-claw/meson.build` con:
  - dependencias: `wingpanel-9`, `granite-7 >= 7.7.0`, `gtk4`, `libadwaita-1`, `libsoup-3.0`, `json-glib-1.0`
  - `shared_module` instalado en `wingpanel_indicatorsdir`
  - `install_dir = wingpanel_dep.get_variable('indicatorsdir')`

- [ ] **1.2** — Crear `src/Indicator.vala`:
  - clase `ClawIndicator : Wingpanel.Indicator`
  - `code_name = "claw"`
  - `get_display_widget()` → `Gtk.Image` con ícono del sistema (placeholder)
  - `get_widget()` → `Gtk.Box` vacío (popover placeholder)
  - función pública `get_indicator()` como entry point

- [ ] **1.3** — Integrar `panel-claw/` en `vm/package-vm-bundle.sh` para que se empaquete con el resto

- [ ] **1.4** — Compilar en la VM y verificar que Wingpanel carga el indicador sin crash

---

### Fase 2 — Popover con chat input
**Objetivo:** el indicador abre un popover al hacer clic; el popover tiene un campo de texto donde escribir.

Archivos a crear:

```
src/
├── Indicator.vala             (actualizar get_widget)
└── Widgets/
    └── ChatPopover.vala
```

- [ ] **2.1** — Crear `ChatPopover.vala` con:
  - `Gtk.Entry` con placeholder `"Pregunta algo…"`
  - Botón de enviar (ícono `send-symbolic`)
  - `Gtk.Spinner` (oculto en reposo, visible mientras procesa)
  - `Gtk.ScrolledWindow` vacío (área de resultados, se poblará en Fase 5)
  - Al presionar Enter o el botón → emitir señal `query_submitted (string query)`
  - Tamaño del popover: ~380 × 480 px

- [ ] **2.2** — Conectar la señal `query_submitted` en `Indicator.vala`
  - Por ahora: imprimir la query en stdout para verificar que funciona

- [ ] **2.3** — Compilar y testear: abrir el panel, click en el indicador, escribir texto, presionar Enter → ver output en journal

---

### Fase 3 — Gateway client en Vala
**Objetivo:** `GatewayClient` hace POST asíncrono al gateway y devuelve la respuesta del LLM.

Archivos a crear:

```
src/
└── Services/
    └── GatewayClient.vala
```

- [ ] **3.1** — Crear `GatewayClient.vala` con:
  ```vala
  public class GatewayClient : Object {
      public async string chat (string query) throws Error;
  }
  ```
  - Usar `Soup.Session` (libsoup-3.0) para HTTP async
  - Endpoint: `http://127.0.0.1:4389/v1/chat/completions`
  - Payload JSON:
    ```json
    {
      "model": "gpt-4o",
      "messages": [{"role": "user", "content": "<query>"}],
      "stream": false
    }
    ```
  - Parsear respuesta con `Json.Parser` (json-glib)
  - Extraer `choices[0].message.content`
  - Propagar el error si el gateway no responde (con mensaje amigable: `"Gateway offline"`)

- [ ] **3.2** — Conectar `GatewayClient` a `ChatPopover`:
  - `query_submitted` → llama `gateway.chat(query)` → output en resultList

- [ ] **3.3** — Prueba end-to-end: preguntar algo simple como `"¿cuánto es 2 + 2?"` → respuesta aparece en la consola / popover

---

### Fase 4 — Gateway: workspaceRoot = $HOME
**Objetivo:** permitir que las tools del gateway (glob, read_file, grep_search) operen sobre el directorio home del usuario, no sobre el config dir.

Cambios en Go (`internal/`):

- [ ] **4.1** — En `internal/app/app.go` (o donde se construye el `Registry`), cambiar el `workspaceRoot` que se pasa a las tools:
  - Actual: probablemente usa `paths.ConfigDir` o similar
  - Nuevo: si no hay workspace configurado → usar `os.Getenv("HOME")`
  - Opcional: permitir override en config YAML: `workspace_root: /home/usuario`

- [ ] **4.2** — Verificar que `glob`, `read_file`, `grep_search`, `list_dir` funcionan correctamente con `workspaceRoot = /home/oscarcode91`
  - Test rápido: `curl -X POST .../v1/chat/completions` con query `"lista los archivos PDF en mi home"`

---

### Fase 5 — Nueva tool: `pdf_text`
**Objetivo:** el LLM puede leer el contenido de un PDF para resumirlo (sin esta tool, `read_file` sólo devuelve bytes binarios ilegibles).

Archivo a crear: `internal/tools/pdf_text.go`

- [ ] **5.1** — Implementar `pdf_text` tool siguiendo el patrón de `exec.go`:
  ```go
  // Ejecuta: pdftotext <path> -
  // Devuelve el texto extraído (hasta N caracteres para evitar tokens excesivos)
  ```
  - `Name()` → `"pdf_text"`
  - `Description()` → `"Extract text content from a PDF file. Use this to read and summarize PDF documents."`
  - Parámetro: `path` (requerido)
  - Usa `exec.Command("pdftotext", filePath, "-")` → captura stdout
  - Trunca a 8000 chars para no exceder ventana del LLM
  - Si `pdftotext` no está instalado → error claro: `"poppler-utils not installed"`

- [ ] **5.2** — Registrar `pdf_text` en el registry (en `app.go` o donde se registran las otras tools)

- [ ] **5.3** — Compilar gateway, deployer en VM, testear:
  ```bash
  curl -X POST http://127.0.0.1:4389/v1/chat/completions \
    -d '{"model":"gpt-4o","messages":[{"role":"user","content":"find a PDF in my home and summarize it"}]}'
  ```
  → verificar que el LLM invoca `glob` + `pdf_text` y devuelve el resumen

---

### Fase 6 — Result widgets: TextCard y FileCard
**Objetivo:** el popover muestra resultados ricos, no solo texto plano. Cuando la respuesta incluye paths, se renderizan como tarjetas clickeables.

Archivos a crear:

```
src/Widgets/
├── ChatPopover.vala      (actualizar para poblar ResultList)
├── TextCard.vala         ← bloque de texto con Markdown básico
└── FileCard.vala         ← path + tipo de archivo + snippet
```

- [ ] **6.1** — Crear `TextCard.vala`:
  - `Gtk.Label` con `wrap = true`, `selectable = true`
  - CSS class: `.claw-text-card`
  - Acepta respuesta en texto plano (sin Markdown parsing complejo)

- [ ] **6.2** — Crear `FileCard.vala`:
  - Ícono del tipo de archivo (`Gtk.Image.from_gicon`)
  - Label con el nombre del archivo (negrita)
  - Label con el path completo (monospace, pequeño, truncado)
  - Botón "Abrir" → `AppInfo.launch_default_for_uri("file:///ruta/...")`
  - CSS class: `.claw-file-card`

- [ ] **6.3** — Lógica de parsing en `ChatPopover.vala`:
  - Después de recibir la respuesta del LLM, parsear el texto buscando paths:
    - Regex: `(\/[^\s]+\.(pdf|txt|md|png|jpg|docx))`
  - Para cada path encontrado → crear un `FileCard`
  - El resto del texto → crear un `TextCard`
  - Agregar ambos al `ScrolledWindow` resultado

- [ ] **6.4** — CSS en un archivo `data/styles/panel-claw.css`:
  ```css
  .claw-file-card {
      border-radius: 8px;
      padding: 8px 12px;
      background-color: alpha(@base_color, 0.8);
      margin-bottom: 4px;
  }
  .claw-text-card {
      padding: 8px 12px;
      font-size: 0.9em;
  }
  ```

---

### Fase 7 — Historial de conversación (context window)
**Objetivo:** preguntas de seguimiento funcionan. El usuario puede preguntar _"y cuál es más reciente?"_ después de una búsqueda de PDFs.

- [ ] **7.1** — En `GatewayClient.vala`, mantener un array `messages` con el historial de la sesión activa:
  ```vala
  private Json.Array messages;
  ```
- [ ] **7.2** — Cada `chat()` call agrega el mensaje del usuario al array y envía todo el historial
- [ ] **7.3** — La respuesta del asistente también se agrega al historial
- [ ] **7.4** — Botón "Nueva conversación" (ícono `edit-clear-symbolic`) → limpia el historial

---

### Fase 8 — Status badge en el ícono del panel
**Objetivo:** el ícono del indicador muestra visualmente si el gateway está activo o no.

- [ ] **8.1** — En `Indicator.vala`, agregar un timer periódico (cada 10s) que hace `GET /healthz`
  - Usar `GLib.Timeout.add_seconds`
- [ ] **8.2** — Display widget usa `Gtk.Overlay`:
  - Base: ícono del agente
  - Overlay: punto pequeño (8×8 px) verde (`@lime_1`) si OK, rojo (`@strawberry_1`) si offline
- [ ] **8.3** — Actualizar `opened()` para hacer health check inmediato al abrir el popover

---

### Fase 9 — Deploy completo y test end-to-end
**Objetivo:** flujo completo funcionando en la VM con Wingpanel real.

- [ ] **9.1** — Actualizar `vm/package-vm-bundle.sh` para incluir `panel-claw/`
- [ ] **9.2** — En la VM:
  ```bash
  cd panel-claw && meson build && sudo ninja -C build install
  wingpanel &  # o reiniciar la sesión
  ```
- [ ] **9.3** — Test del flujo completo:
  1. Click en el ícono de claw en el panel
  2. Escribir: `"encuentra el archivo contrato.pdf y dame un resumen"`
  3. Verificar: spinner → respuesta con `FileCard` (path del PDF) + `TextCard` (resumen)
  4. Click en "Abrir" → se abre el PDF en el visor nativo
- [ ] **9.4** — Test de gateway offline:
  - Detener el gateway → el ícono muestra el punto rojo
  - Intentar enviar query → mensaje de error amigable `"Gateway offline"`

---

## Convenciones de código Vala

Todo el código Vala de `panel-claw` debe seguir el estilo y los patrones de
`references/initial-setup/src/Views/AIConnectView.vala` como referencia canónica:

- Estructura de clase: campos privados declarados al inicio, luego `construct {}` + `build_ui()` separados
- Widgets como campos privados de la clase (nunca `var` locales si se referencian después)
- CSS classes aplicadas con `add_css_class()` (patron de `nothing-installer.css`)
- Señales declaradas con `public signal void nombre (tipo param);`
- Strings con Unicode: usar `\u201c` etc., nunca comillas tipográficas literales
- Transiciones async de UI: usar `Idle.add(() => { ...; return false; })`, nunca llamar a métodos de estado desde dentro de tick callbacks
- Easing y animaciones: seguir los helpers `ease_in_out()` / `ease_out()` de `AIConnectView`
- **Antes de crear cualquier widget nuevo**, revisar `AIConnectView.vala` para ver si ya existe el patrón

---

## Resumen de archivos nuevos

| Archivo | Descripción |
|---------|-------------|
| `panel-claw/meson.build` | Build system del indicador |
| `panel-claw/src/Indicator.vala` | Entry point, display widget + popover |
| `panel-claw/src/Widgets/ChatPopover.vala` | UI del popover completo |
| `panel-claw/src/Widgets/TextCard.vala` | Widget para respuestas de texto |
| `panel-claw/src/Widgets/FileCard.vala` | Widget para archivos encontrados |
| `panel-claw/src/Services/GatewayClient.vala` | Cliente HTTP async al gateway |
| `panel-claw/data/styles/panel-claw.css` | CSS del indicador |
| `panel-claw/data/icons/io.elementary.panel.claw.svg` | Ícono del panel |
| `internal/tools/pdf_text.go` | Tool para extraer texto de PDFs |

## Resumen de cambios en archivos existentes

| Archivo | Cambio |
|---------|--------|
| `internal/app/app.go` | `workspaceRoot = os.Getenv("HOME")` como default |
| `vm/package-vm-bundle.sh` | Incluir `panel-claw/` en el bundle |

---

## Orden de ejecución recomendado

```
Fase 0 → Fase 1 → Fase 2 → Fase 3 → Fase 4 → Fase 5 → Fase 3 (test tools) → Fase 6 → Fase 7 → Fase 8 → Fase 9
```

Las fases 4 y 5 (gateway) se pueden trabajar en paralelo con las fases 2 y 3 (UI), ya que son independientes.
