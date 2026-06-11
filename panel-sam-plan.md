# panel-sam — Plan maestro

> Indicator para Wingpanel que permite hacer cualquier pregunta desde el panel de elementary OS.
> Ejemplo de flujo: el usuario escribe _"encuentra el PDF con nombre contrato.pdf y dame un resumen"_ →
> el gateway ejecuta las tools necesarias (glob, pdf_text, read_file) → el indicador muestra
> una tarjeta con el path del archivo y el resumen del contenido.

---

## Arquitectura de alto nivel

```
┌─────────────────────────────────────────┐
│       panel-sam (Vala / GTK3 + GLib)    │
│                                          │
│  [Wingpanel] → [ChatPopover]             │
│      icon          ├─ Gtk.Entry          │
│   + status dot     ├─ ResultList         │
│                    │    └─ FileCard      │
│                    │    └─ TextCard      │
│                    └─ Spinner            │
└──────────────┬──────────────────────────┘
               │ HTTP POST (libsoup-3.0, async)
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

## Problemas resueltos el 2026-04-21

### 1. El saludo de setup seguia apareciendo despues del onboarding

**Sintoma observado**

- Samantha respondia con frases como `Hola oscarcode91 — soy Semantha, configurada durante el setup del sistema.` incluso despues de terminar el setup inicial.

**Causa raiz**

- La VM tenia archivos heredados de una version anterior del workspace seed (`IDENTITY.md`, `SOUL.md`, `AGENTS.md`, `USER.md`) que seguian empujando un saludo de onboarding.
- El runtime todavia podia caer de forma implicita en la sesion `bootstrap` si ese archivo de sesion existia, aunque `BOOTSTRAP.md` ya no estuviera presente.
- El parser del prompt solo reconocia `Name:` en `IDENTITY.md`, pero el formato canonical actual usa `assistant_name:`.

**Cambios aplicados**

- En [internal/config/setup.go](/Users/oscarcode/elementary-claw/internal/config/setup.go) se agrego `RepairLegacyWorkspaceFiles(paths)` para migrar de forma idempotente los archivos heredados al formato canonical actual.
- En [internal/runtime/http_handler.go](/Users/oscarcode/elementary-claw/internal/runtime/http_handler.go) y [internal/runtime/streaming.go](/Users/oscarcode/elementary-claw/internal/runtime/streaming.go) el fallback a la sesion `bootstrap` ahora solo ocurre si `BOOTSTRAP.md` sigue existiendo de verdad.
- En [internal/prompt/prompt.go](/Users/oscarcode/elementary-claw/internal/prompt/prompt.go) `ParseIdentityName` ahora acepta tanto `Name:` como `assistant_name:`.
- En la VM tambien se elimino `~/.openclaw/workspace/BOOTSTRAP.md` y se dejo `~/.openclaw/workspace/.workspace-state.json` con `bootstrapCompleted=true` para alinear el estado con el runtime actual.

**Validacion**

- Tests locales: `go test ./internal/config ./internal/prompt ./internal/runtime`
- Verificacion en la VM: `/healthz` y `/status` reportaron `bootstrapPresent=false`.
- Prueba real contra el gateway: la consulta `Que archivos PDF tengo en mi carpeta personal?` ya no devolvio el saludo de setup.

### 2. El click en un PDF no abria el visor

**Sintoma observado**

- Samantha listaba PDFs, pero al hacer click en el resultado no se abria ninguna aplicacion.

**Causa raiz**

- El indicador estaba usando `GLib.AppInfo.launch_default_for_uri`, que en este contexto de Wingpanel no estaba abriendo el archivo de forma confiable.
- El parser de resultados solo convertia en botones paths absolutos o con `~`; cuando el modelo devolvia rutas como `Documentos/PDFs/archivo.pdf`, se mostraban como texto normal o se resolvian de forma incompleta.
- Algunos paths llegaban con puntuacion o backticks de Markdown y habia que limpiarlos antes de resolver el `file://`.
- La VM necesitaba una asociacion MIME valida para `application/pdf`.

**Cambios aplicados**

- En [panel-sam/src/Indicator.vala](/Users/oscarcode/elementary-claw/panel-sam/src/Indicator.vala) el indicador ahora usa `Gtk.show_uri_on_window(null, uri, Gdk.CURRENT_TIME)` y difiere el lanzamiento con `GLib.Idle.add`.
- El parser de resultados ahora limpia sufijos como `.`, `:`, y `` ` `` antes de crear el boton clickeable.
- El regex del indicador ahora reconoce:
  - paths absolutos (`/home/oscarcode91/...`)
  - paths con `~`
  - paths relativos al home como `Documentos/PDFs/archivo.pdf`
- Si el path es relativo, el indicador lo resuelve contra `GLib.Environment.get_home_dir()` antes de abrirlo.
- En la VM se verifico y dejo configurado el handler MIME `org.gnome.Evince.desktop` para `application/pdf`.

**Validacion**

- Build local del Vala sin errores reportados por VS Code.
- Rebuild nativo en la VM de `panel-sam` con `meson` + `ninja` + `sudo ninja -C build install`.
- Reinicio de Wingpanel en la VM para cargar la nueva `libsam.so`.
- Prueba real contra el gateway: la respuesta final ya devolvio rutas absolutas a los PDFs del home, listas para convertirse en botones clickeables.

### 3. Comando de verificacion que se uso en la VM

Para comprobar el flujo real contra el gateway activo se uso una sesion nueva y una consulta directa:

```bash
sshpass -p '12345' ssh -o StrictHostKeyChecking=no oscarcode91@192.168.64.5 \
  "printf '%s' '{\"model\":\"gpt-5.4\",\"stream\":false,\"max_completion_tokens\":300,\"session_id\":\"panel-sam-pdf-proof-2\",\"messages\":[{\"role\":\"user\",\"content\":\"Que archivos PDF tengo en mi carpeta personal?\"}]}' \
  | curl -s -X POST http://127.0.0.1:4389/v1/chat/completions \
    -H 'Content-Type: application/json' -d @-"
```

Respuesta verificada despues del fix:

```text
Tienes estos archivos PDF en tu carpeta personal:

- /home/oscarcode91/Documentos/contrato-ejemplo.pdf
- /home/oscarcode91/Documentos/PDFs/Clase1-IA-NoDevs.pdf
- /home/oscarcode91/Documentos/PDFs/contrato-servicios-tia-v3.pdf
- /home/oscarcode91/Documentos/PDFs/propuesta-guillermo-mayo.pdf
```

---

## Fases de trabajo

---

### Fase 0 — Verificación pre-requisitos en VM ✅ COMPLETADA
**Resultado verificado el 2026-04-15:**

| Dependencia | pkg-config name | Versión | Estado |
|-------------|----------------|---------|--------|
| Wingpanel | `wingpanel` | 8.0.4 | ✅ (`libwingpanel-dev` instalado) |
| libsoup | `libsoup-3.0` | 3.4.4 | ✅ |
| json-glib | `json-glib-1.0` | 1.8.0 | ✅ |
| granite | `granite-7` | 7.8.1 | ✅ |
| libadwaita | `libadwaita-1` | 1.5.0 | ✅ |
| pdftotext | `/usr/bin/pdftotext` | 24.02.0 | ✅ |
| valac | — | 0.56.17 | ✅ |

> ⚠️ **CRÍTICO:** Wingpanel 8.0.4 usa **GTK3** (no GTK4). Todo el código del indicador
> debe usar GTK3. La API del indicador está en `/usr/share/vala/vapi/wingpanel.vapi`.
> El directorio de instalación es `/usr/lib/aarch64-linux-gnu/wingpanel`.
> El nombre correcto de pkg-config es `wingpanel` (no `wingpanel-9`).

---

### Fase 1 — Scaffold del proyecto `panel-sam`
**Objetivo:** crear la estructura mínima del indicador que compile y cargue en Wingpanel (aunque haga nada).

Archivos a crear:

```
panel-sam/
├── meson.build
├── data/
│   └── icons/
│       └── io.elementary.panel.claw.svg   ← ícono del indicador
└── src/
    └── Indicator.vala
```

- [ ] **1.1** — Crear `panel-sam/meson.build` con:
  - dependencias: `wingpanel` (GTK3), `libsoup-3.0`, `json-glib-1.0`
  - **NO** incluir `gtk4` ni `libadwaita-1` — Wingpanel es GTK3
  - `shared_module` instalado en `wingpanel_indicatorsdir`
  - `wingpanel_dep = dependency('wingpanel')`
  - `install_dir = wingpanel_dep.get_variable('indicatorsdir')`

- [ ] **1.2** — Crear `src/Indicator.vala`:
  - clase `SamIndicator : Wingpanel.Indicator` (GTK3)
  - `code_name = "sam"`
  - `get_display_widget()` → `Gtk.Image` con `icon_name = "system-search-symbolic"` (GTK3)
  - `get_widget()` → `Gtk.Box` vacío (popover placeholder)
  - `opened()` / `closed()` vacíos por ahora
  - Entry point: `public Wingpanel.Indicator? get_indicator (Wingpanel.IndicatorManager.ServerType server_type)`

- [ ] **1.3** — Integrar `panel-sam/` en `vm/package-vm-bundle.sh` para que se empaquete con el resto

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

- [ ] **6.4** — CSS en un archivo `data/styles/panel-sam.css`:
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

- [ ] **9.1** — Actualizar `vm/package-vm-bundle.sh` para incluir `panel-sam/`
- [ ] **9.2** — En la VM:
  ```bash
  cd panel-sam && meson build && sudo ninja -C build install
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

Todo el código Vala de `panel-sam` debe seguir el estilo y los patrones de
`references/initial-setup/src/Views/AIConnectView.vala` como referencia canónica
**para la estructura y naming, pero usando GTK3** (no GTK4) ya que Wingpanel 8.0.4 es GTK3:

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
| `panel-sam/meson.build` | Build system del indicador |
| `panel-sam/src/Indicator.vala` | Entry point, display widget + popover |
| `panel-sam/src/Widgets/ChatPopover.vala` | UI del popover completo |
| `panel-sam/src/Widgets/TextCard.vala` | Widget para respuestas de texto |
| `panel-sam/src/Widgets/FileCard.vala` | Widget para archivos encontrados |
| `panel-sam/src/Services/GatewayClient.vala` | Cliente HTTP async al gateway |
| `panel-sam/data/styles/panel-sam.css` | CSS del indicador |
| `panel-sam/data/icons/io.elementary.panel.claw.svg` | Ícono del panel |
| `internal/tools/pdf_text.go` | Tool para extraer texto de PDFs |

## Resumen de cambios en archivos existentes

| Archivo | Cambio |
|---------|--------|
| `internal/app/app.go` | `workspaceRoot = os.Getenv("HOME")` como default |
| `vm/package-vm-bundle.sh` | Incluir `panel-sam/` en el bundle |

---

## Orden de ejecución recomendado

```
Fase 0 → Fase 1 → Fase 2 → Fase 3 → Fase 4 → Fase 5 → Fase 3 (test tools) → Fase 6 → Fase 7 → Fase 8 → Fase 9
```

Las fases 4 y 5 (gateway) se pueden trabajar en paralelo con las fases 2 y 3 (UI), ya que son independientes.
