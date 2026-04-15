# elementary-claw: investigacion y plan maestro

Fecha: 2026-04-02

## 1. Respuesta corta

Si, es posible, y la aclaracion importante es que el objetivo correcto no es navegador primero sino sistema operativo primero.

La parte importante es esta:

- Si quieren que la herramienta venga integrada desde la instalacion de elementary OS, entonces el centro del sistema tiene que ser nativo del sistema operativo: daemon, app, servicios, paquetes e integracion con el shell.
- El navegador puede existir, pero como superficie secundaria local, no como runtime principal del producto.
- La arquitectura correcta es: servicio local en Go preinstalado, app nativa de elementary OS preinstalada, integracion con Wingpanel/Switchboard cuando madure, y una ISO custom o metapaquete del sistema para distribuirlo desde instalacion.

Conclusion practica:

- Si se puede construir algo inspirado en OpenClaw con Go.
- Si se puede integrar a elementary OS desde la instalacion del sistema.
- Si se puede distribuir preinstalado via paquetes o via ISO custom.
- No conviene tratar el navegador como entorno principal del runtime.

## 2. Que investigamos

Se hizo investigacion sobre 5 frentes:

1. elementary OS y su sistema de construccion
2. arquitectura de OpenClaw
3. Go + WebAssembly en navegador
4. MCP y conectores MCP en Go
5. ecosistema Go para CLI, Gateway, desktop app y WebView

Hallazgos clave:

- elementary OS usa una base Ubuntu/Debian y construye sus ISOs con live-build en el repo del sistema operativo.
- El desktop shell se compone principalmente de proyectos como Gala, Wingpanel, Switchboard y Granite.
- La mayor parte del stack de elementary OS esta en Vala + GTK + Granite + Meson.
- OpenClaw hoy depende fuertemente de TypeScript/Node.js, WebSocket control plane, tools, skills y runtimes de agentes.
- Go puede reemplazar bien el Gateway, CLI, WebSocket server, procesos, daemon, MCP servers y varias capas de infraestructura.
- Go en WASM funciona bien para logica compartida, parsers, rendering ligero, colas de eventos, clientes delgados y paneles interactivos, pero no para todo lo que requiere acceso privilegiado al host.

## 3. Vision de producto para elementary-claw

Objetivo:

Construir una plataforma tipo OpenClaw pero orientada a elementary OS, donde la app nativa de escritorio ES el sistema desde el primer momento. No es una interfaz secundaria ni una fase posterior: es la superficie principal desde el primer boot. El backend en Go existe para servir a esa app.

Superficies del producto, en orden de prioridad:

- app nativa de elementary OS (ES la interfaz principal, desde el dia 1)
- daemon local del sistema (sirve a la app)
- CLI operativa (complemento para operacion y desarrollo)
- panel HTTP/WebSocket opcional (solo para diagnostico avanzado, nunca como ruta principal)

Capacidades objetivo:

- chat con agentes
- tools locales
- skills instalables
- conectores MCP
- sesiones persistentes
- integracion con archivos, procesos, notificaciones y sistema
- empaquetado nativo para elementary OS
- preinstalacion en imagen custom del sistema

## 4. Arquitectura recomendada

## 4.1 Componentes principales

### A. Gateway local en Go

Responsabilidades:

- WebSocket control plane
- API HTTP local
- autenticacion local
- registro de tools
- ejecucion de procesos
- sesiones y estado
- integracion MCP client/server
- logs, health, config y eventos

Stack recomendado:

- Go 1.24+
- Gin o Echo para HTTP
- gorilla/websocket o nhooyr/websocket para WS
- Cobra + Viper para CLI/config
- SQLite o BoltDB para estado ligero
- JSONL para transcripts si quieren simplicidad tipo OpenClaw
- kardianos/service o unidades systemd/launchd para daemon

### B. App nativa de elementary OS (interfaz principal)

Esta es LA interfaz del sistema. No es una fase posterior ni un wrapper sobre una UI web. Desde el primer arranque del sistema, el usuario interactua con esta app.

Responsabilidades:

- chat con agentes (superficie principal)
- panel de sesiones
- panel de tools
- panel MCP
- panel de skills
- settings
- logs y debugging
- notificaciones del sistema
- integracion con archivos y procesos locales

Stack:

- GTK4 + Granite (nativo de elementary OS)
- WebKitGTK para vistas ricas donde haga falta (rendering de markdown, previews)
- Comunicacion con el Gateway via socket local o HTTP local

Nota sobre UI web auxiliar:

Si en algun momento se necesita un panel HTTP accesible desde navegador (debugging remoto, admin avanzado), se puede construir despues como superficie secundaria. Pero nunca sera la ruta principal del producto ni un prerequisito para la app nativa.

### C. Modulo WASM en Go

Responsabilidades razonables:

- parsing y transformaciones rapidas
- viewers
- mini runtime de comandos seguros
- visualizadores de estado
- componentes que requieran compartir logica entre browser y backend

No meter aqui:

- procesos del sistema
- acceso privilegiado al host
- integraciones con systemd/polkit
- sockets privilegiados
- control total del filesystem fuera del sandbox del browser

### D. Integracion profunda con elementary OS

Estas son extensiones que se agregan sobre la app nativa ya existente, no la app en si.

Responsabilidades:

- app nativa GTK/Granite preinstalada
- daemon local preinstalado
- servicio systemd de usuario o servicio de sistema segun el modelo final
- indicator/menu panel en Wingpanel
- settings plug en Switchboard
- notificaciones del sistema
- desktop entry, autostart y componentes de sesion

### D.1 Secuencia real desde instalacion

Para que no haya confusion con el flujo de producto, la secuencia correcta es esta:

1. La ISO o el metapaquete instala elementary-claw como parte nativa del sistema.
2. En cuanto se crea la cuenta del usuario dentro de Initial Setup, el sistema tambien crea y configura su agente de IA base.
3. Ese mismo flujo recoge identidad inicial, conexion a proveedor, secretos y personalidad del agente.
4. Si ya existe conexion a internet, Initial Setup puede autenticar al proveedor, dejar el agente provisionado y ejecutar el primer saludo o intercambio inicial desde ahi mismo.
5. En el primer login del usuario arranca el servicio persistente de elementary-claw y continua con ese agente ya creado.
6. Ese servicio levanta sesiones, estado, tools y conectores sobre la identidad y configuracion definidas durante el setup.
7. La app nativa o shell surface abre una conversacion viva sobre el mismo runtime del producto.

Importante:

- Initial Setup no reemplaza al runtime persistente, pero si forma parte real del onboarding del agente.
- Initial Setup sirve para crear la cuenta del usuario y, en el mismo paso, crear tambien el agente con permisos, secretos, proveedor y personalidad inicial.
- La diferenciacion del producto empieza desde la creacion de la cuenta: usuario y agente quedan seteados juntos.
- Si hay conectividad, el agente puede quedar autenticado y tener su primer intercambio desde el propio setup.
- El autostart o arranque controlado en la primera sesion sigue siendo necesario para continuidad, integracion completa con el sistema y runtime persistente.

### E. Conectores MCP

Responsabilidades:

- consumir servidores MCP externos
- exponer el sistema local como un servidor MCP propio
- convertir tools internas en tools MCP cuando convenga

## 4.2 Arquitectura final sugerida

```text
elementary OS Installation Image
                |
                v
   Preinstalled packages and services
                |
                v
      Native elementary-claw stack
                |
    +-----------+-----------+
    |                       |
    v                       v
Native App  <=======>  Go Gateway + Daemon
GTK4/Granite           (sirve a la app)
    |                       |
    |    +---------+--------+
    |    |         |        |
    v    v         v        v
  Chat  Tools   Sessions  MCP Client/Server
    |    |         |        |
    +----+---------+--------+
                |
                v
        Local OS + external systems
                |
    +-----------+-----------+
    |                       |
    v                       v
Shell Integration       CLI operativa
Wingpanel/Switchboard   (complemento)
```

Nota: la app nativa y el gateway se construyen juntos desde el inicio. La app no es un cliente que se agrega despues; es co-primaria con el daemon.

## 5. Que si conviene portar de OpenClaw y que no

## 5.1 Cosas que si vale la pena replicar

- Gateway central como control plane
- modelo de sesiones
- tools tipadas
- skills cargadas desde archivos
- cliente web
- CLI operativa
- eventos por WebSocket
- separacion entre runtime, UI y conectores

## 5.2 Cosas que no conviene copiar 1:1

- dependencia fuerte en Node/TS
- todo el runtime metido en browser
- suposiciones multiplataforma pensadas para macOS/iOS/Android desde el dia 1
- cantidad de canales externos antes de tener un core estable

## 5.3 Recomendacion realista

Hacer un proyecto nuevo inspirado en OpenClaw, no un port literal.

## 6. elementary OS: que hay que modificar para integrarlo de verdad

elementary OS no se modifica en un solo repo. La integracion fuerte cae en varias capas.

## 6.1 Repos y piezas relevantes

- elementary/os: construccion de ISO y metapaquetes del sistema
- elementary/gala: window manager/compositor
- elementary/wingpanel: panel superior
- elementary/settings: app de configuracion del sistema; los modulos siguen el modelo de Switchboard plugs
- elementary/granite: libreria UI base para apps del ecosistema

## 6.2 Niveles de integracion posibles

### Nivel 1. App nativa preinstalada

Es el minimo correcto si el producto debe existir desde la instalacion del sistema.

Incluye:

- una app GTK4 + Granite o GTK4 + WebKitGTK
- icono
- desktop file
- notificaciones
- launch desde aplicaciones
- inclusion en la imagen o metapaquete base del sistema

Ya implica empaquetado del sistema, aunque todavia no requiera tocar Wingpanel ni Switchboard.

### Nivel 2. Servicio preinstalado + app + autostart

Incluye:

- daemon local preinstalado
- app UI
- socket o HTTP local
- integracion con systemd user service
- arranque automatico en login o segun politica del sistema

Este es el punto donde ya existe una experiencia real "nativa desde instalacion".

### Nivel 3. Indicator en Wingpanel

Incluye:

- plugin o indicator para acceso rapido
- estado del Gateway
- acciones como abrir dashboard, reiniciar daemon, ver logs, cambiar perfil

Aqui ya tocan componentes mas cercanos al shell de Pantheon.

### Nivel 4. Settings plug en Switchboard

Incluye:

- pagina de configuracion del agente
- permisos
- conectores MCP
- rutas workspace
- modelos y credenciales

### Nivel 5. ISO custom de elementary-claw

Incluye:

- fork de elementary/os
- paquete del daemon
- paquete de la app
- autostart y configuracion inicial
- branding opcional
- preinstalacion de dependencias

Esta via si modifica la distribucion desde la imagen.

Si la meta es que venga listo desde el primer boot, este nivel debe ser objetivo formal de release, aunque no sea el primer entregable tecnico.

## 6.3 Recomendacion de orden

No empiecen rehaciendo media distro, pero tampoco diseñen como si fuera una app web cualquiera.

Empiecen asi:

1. App nativa + Gateway en Go (juntos desde el inicio)
2. servicio preinstalable
3. paquete .deb y metapaquete
4. indicator y settings plug
5. ISO custom

## 7. Como construir y probar sobre elementary OS desde ya

## 7.1 Entorno de desarrollo minimo

Necesitan:

- una maquina con elementary OS 8.x o una VM
- otra VM limpia para pruebas de instalacion
- una VM dedicada a pruebas de actualizacion entre versiones
- Go 1.24+
- Node solo para la UI, si usan React/Vue/Svelte
- Meson + Ninja si hacen app GTK/Vala o componentes nativos del sistema
- WebKitGTK si embeben una UI web
- Git
- Docker o Podman para reproducibilidad

Paquetes base aproximados:

```bash
sudo apt update
sudo apt install -y \
  golang-go git curl jq sqlite3 \
  build-essential pkg-config \
  meson ninja-build \
  libgtk-4-dev libgranite-dev \
  libwebkitgtk-6.0-dev || true
```

Nota:

- La version exacta de WebKitGTK y algunas librerias puede variar segun la base Ubuntu de elementary OS.
- Si van a tocar Wingpanel, Gala o Switchboard, van a necesitar tambien las dev libs especificas de cada repo.

## 7.2 Entorno de pruebas recomendado

Usen 3 entornos:

1. Dev host
2. VM de elementary OS para pruebas de app, servicio e integracion shell
3. VM limpia para probar instalacion, primer boot, upgrades e ISO custom

## 7.3 Tipos de pruebas que deben existir

- unit tests del core Go
- integration tests del Gateway
- tests de MCP client/server
- tests de CLI
- tests de UI web
- pruebas manuales en elementary OS real
- pruebas de empaquetado .deb
- pruebas de arranque de servicio de usuario
- pruebas de primer boot tras instalacion
- pruebas de autostart post-login
- pruebas de upgrade de paquete sin perder estado
- pruebas de recovery tras reinicio

## 8. Roadmap paso a paso

## Fase 0. Decision de arquitectura

Entregable:

- ADR con arquitectura nativa-install-first confirmada

Decisiones:

- Go como lenguaje principal del backend
- UI web solo como superficie auxiliar
- integracion con elementary OS como producto nativo preinstalable
- ISO custom como objetivo de distribucion final

## Fase 1. Monorepo y esqueleto del proyecto

Propuesta de estructura:

```text
elementary-claw/
  apps/
    gateway/
    desktop/
    panel/
  packages/
    sdk-go/
    protocol/
    mcp/
    tools/
    skills/
  deployments/
    systemd/
    deb/
    iso/
  docs/
  scripts/
```

Entregables:

- repo inicial
- Makefile o Taskfile
- CI minima
- config base

## Fase 2. Gateway core + App nativa minima (juntos)

El Gateway y la app nativa se construyen en paralelo desde esta fase. No se construye primero el backend y despues la interfaz: la interfaz ES el producto.

### 2A. Gateway core en Go

Entregables:

- server HTTP local (socket o puerto)
- config loader
- auth local
- session manager
- event bus
- logger estructurado

### 2B. App nativa para elementary OS

Entregables:

- ventana GTK4 + Granite funcional
- conexion al Gateway local
- vista de chat basica (enviar mensaje, recibir respuesta)
- vista de estado del daemon
- desktop entry e icono
- instalable como .deb desde esta fase

La app arranca, se conecta al daemon, y permite una conversacion basica. Esto es el primer entregable visible.

CLI minima sugerida (complemento, no interfaz principal):

```bash
claw gateway start
claw gateway doctor
claw gateway status
claw sessions list
claw sessions inspect <id>
claw config validate
```

## Fase 3. Tools runtime + vista de tools en la app

Entregables backend:

- tool registry
- tool schemas
- exec tool
- filesystem tool
- web fetch tool
- HTTP client tool
- notifications tool
- systemd integration tool
- journal inspection tool

Entregables en la app nativa:

- vista de tools disponibles
- ejecucion de tools desde la app
- visualizacion de resultados de tool runs en el chat

CLI sugerida:

```bash
claw tools list
claw tools inspect exec
claw tools test exec -- "echo hola"
```

## Fase 4. Skills system + vista de skills en la app

Entregables backend:

- formato de skill
- loader de skills desde filesystem
- validacion
- precedence por workspace y usuario

Formato propuesto:

```yaml
---
name: shell-helper
description: Ejecuta tareas locales de shell con seguridad
version: 0.1.0
inputs:
  - gateway
permissions:
  - exec
  - files
---

Instrucciones para el agente...
```

Entregables en la app nativa:

- vista de skills instaladas
- habilitar/deshabilitar skills desde la app

CLI sugerida (complemento):

## Fase 5. MCP + vista de conectores en la app

Entregables backend:

- MCP client dentro del Gateway
- MCP server local de elementary-claw
- bridge entre tools internas y tools MCP

Libreria recomendada:

- github.com/mark3labs/mcp-go

Conectores MCP prioritarios:

1. filesystem
2. github
3. postgres
4. docker
5. browser automation
6. docs/search
7. slack o discord si luego quieren canal externo

Entregables en la app nativa:

- vista de servidores MCP conectados
- agregar/remover servidores MCP desde la app
- ver tools MCP disponibles

CLI sugerida (complemento):

## Fase 6. App nativa madura + persistencia

En este punto la app ya lleva varias fases de desarrollo incremental. Esta fase la solidifica.

Entregables:

- persistencia completa de sesiones y estado
- logs en vivo dentro de la app
- settings completos desde la app
- manejo de errores y reconexion al daemon
- primera version pulida de la experiencia de chat
- soporte para multiples sesiones simultaneas

### Panel web auxiliar (opcional, no bloquea nada)

Si se necesita un panel accesible por navegador para diagnostico remoto o admin avanzado, se puede construir aqui. Pero es estrictamente auxiliar.

Entregables opcionales:

- login local
- dashboard de health
- logs viewer
- NO replica la experiencia completa de la app nativa

## Fase 7. Servicio de usuario

Entregables:

- unit file systemd --user
- autostart
- health checks
- logs

Ejemplo de comandos:

```bash
systemctl --user daemon-reload
systemctl --user enable elementary-claw.service
systemctl --user start elementary-claw.service
systemctl --user status elementary-claw.service
journalctl --user -u elementary-claw.service -f
```

## Fase 8. Integracion shell de elementary

Entregables:

- indicator para Wingpanel
- Switchboard plug
- notificaciones y acciones rapidas

Solo hagan esto cuando el core ya este estable.

## Fase 9. Paquete .deb

Entregables:

- paquete del gateway
- paquete de la app
- paquete de integracion shell
- dependencias declaradas
- postinst limpio
- migraciones de config/estado

## Fase 10. Metapaquete e instalacion nativa

Entregables:

- metapaquete `elementary-claw`
- dependencias sobre daemon, app e integraciones
- presets de config inicial
- hooks de instalacion y upgrade

Meta:

- que con instalar el paquete o venir en la ISO, el sistema quede listo desde primer arranque.

## Fase 11. ISO custom

Entregables:

- fork de elementary/os
- inclusion de paquetes
- configuracion inicial
- pipeline reproducible

Proceso alto nivel:

1. fork de elementary/os
2. agregar sus paquetes al build
3. ajustar configuracion de la imagen
4. construir ISO con live-build
5. probar en VM limpia

## 9. Conectores MCP que debemos agregar desde el inicio

## 9.1 MCP client dentro del Gateway

Esto permite que elementary-claw consuma herramientas externas.

Servidores MCP utiles:

- filesystem
- github
- git local
- postgres
- mysql
- docker
- kubernetes
- sentry
- grafana/loki
- slack
- notion
- browser automation

## 9.2 MCP server propio de elementary-claw

Esto permite exponer al sistema local como capacidades MCP.

Tools MCP propias sugeridas:

- local.files.read
- local.files.write
- local.process.run
- local.notifications.send
- local.sessions.list
- local.sessions.send
- local.logs.tail
- local.settings.get
- local.settings.set

## 9.3 Seguridad MCP

Minimo obligatorio:

- allowlist de tools sensibles
- permisos por workspace
- confirmacion del usuario para exec, delete y network sensible
- auditoria de llamadas
- timeouts
- limites de salida

## 10. Tools que debe tener el sistema

## 10.1 Tools base

- read_file
- write_file
- apply_patch equivalente
- list_dir
- grep_search
- web_fetch
- http_request
- exec
- process_background
- notifications
- open_url

## 10.2 Tools para elementary OS

- appcenter_search
- system_package_status
- first_boot_status
- systemd_user_status
- journal_tail
- notify_send
- dbus_call seguro
- settings_portal

## 10.3 Tools para desarrollo

- git_status
- git_diff
- test_run
- build_run
- package_deb
- iso_build

## 10.4 Tools para operacion del agente

- sessions_list
- sessions_history
- sessions_send
- tasks_list
- tasks_run
- health_report

## 11. CLI calls que debemos diseñar

Nombre tentativo del binario:

```bash
claw
```

Comandos principales:

```bash
claw gateway start
claw gateway stop
claw gateway status
claw gateway doctor

claw ui open

claw sessions list
claw sessions inspect <id>
claw sessions send <id> --message "hola"

claw tools list
claw tools run <tool> --json '{"cmd":"echo hola"}'

claw skills list
claw skills enable <name>
claw skills disable <name>
claw skills validate

claw mcp servers list
claw mcp servers add <name>
claw mcp servers remove <name>
claw mcp tools list

claw dev watch
claw dev logs
claw dev reset

claw package deb
claw package meta
claw iso build
```

## 12. Skills que debemos definir

Las skills aqui no son "magia"; son paquetes de instrucciones, permisos y dependencias para el runtime del agente.

Skills iniciales sugeridas:

1. shell-helper
2. git-operator
3. elementary-os-builder
4. deb-packager
5. mcp-admin
6. docs-researcher
7. test-runner
8. ui-builder
9. incident-debugger
10. log-analyzer

Ejemplos concretos:

### shell-helper

- permisos: exec, files
- uso: automatizar comandos locales seguros

### elementary-os-builder

- permisos: exec, files, process_background
- uso: construir paquetes, servicios y eventualmente ISO

### mcp-admin

- permisos: mcp, files, network
- uso: dar de alta servidores MCP y validar conectividad

## 13. Stack tecnico recomendado

## 13.1 Backend

- Go
- Gin o Echo
- WebSocket library
- SQLite o BoltDB
- Cobra + Viper
- zerolog o slog
- mcp-go

## 13.2 App nativa (interfaz principal)

- GTK4 + Granite (nativo de elementary OS)
- Vala como lenguaje principal de la app
- WebKitGTK para rendering de contenido rico donde haga falta
- Comunicacion con Gateway via socket local / HTTP local

## 13.3 Panel web auxiliar (opcional)

- HTMX o Svelte ligero
- Solo para diagnostico/admin remoto
- No reemplaza la app nativa

## 13.4 Packaging

- .deb
- systemd user service
- app desktop entry
- metapaquete del sistema
- ISO custom

## 14. Riesgos tecnicos reales

## 14.1 Riesgo 1

Construir el backend primero y dejar la app nativa para despues.

Impacto:

- el producto se convierte en un servidor sin cara
- se termina usando el navegador como muleta temporal que se vuelve permanente
- mala integracion con instalacion, boot y experiencia de usuario

Mitigacion:

- la app nativa se construye en la misma fase que el gateway, desde el dia 1
- cada nueva capacidad del backend se expone inmediatamente en la app

## 14.2 Riesgo 2

Empezar modificando elementary OS demasiado pronto.

Impacto:

- se frena el avance
- debugging mucho mas costoso

Mitigacion:

- primero app y daemon
- despues indicator y settings
- al final ISO custom

## 14.3 Riesgo 3

Intentar paridad total con OpenClaw desde la primera version.

Mitigacion:

- empezar con un subconjunto del producto

## 14.4 Riesgo 4

No definir modelo de permisos para tools y MCP.

Mitigacion:

- policy engine desde el dia 1

## 15. Plan de pruebas desde ahorita

## 15.1 Primeras pruebas utiles

1. levantar un Gateway minimo en Go
2. registrar un servicio local arrancable por systemd --user
3. ejecutar una tool simple de shell
4. registrar una session y persistirla
5. conectar un servidor MCP filesystem
6. exponer una tool local via MCP server propio
7. empaquetar e instalar una app minima para elementary OS
8. validar primer arranque tras instalacion

## 15.2 Proof of concept minimo viable

El primer PoC bueno debe poder hacer esto:

- abrir la app nativa de elementary-claw en el escritorio
- la app se conecta al daemon local automaticamente
- escribir un mensaje en el chat de la app
- el agente ejecuta una tool (`echo hola`) y muestra el resultado en la app
- la sesion queda guardada y se puede reabrir
- `claw gateway status` confirma que el daemon esta corriendo
- `systemctl --user status elementary-claw.service` muestra el servicio activo

Si eso funciona, ya tienen el esqueleto correcto: app nativa + daemon trabajando juntos desde el primer momento.

## 16. Respuesta concreta a la pregunta original

Si, si es posible hacer una herramienta tipo OpenClaw inspirada en ese modelo, pero escrita principalmente en Go y pensada como componente nativo de elementary OS desde instalacion.

La estrategia correcta no es:

- portar todo OpenClaw literal a Go
- ni meter el runtime principal dentro del navegador

La estrategia correcta es:

- App nativa GTK4/Granite como interfaz principal desde el dia 1
- Gateway local en Go que sirve a la app
- ambos se construyen juntos, no en fases separadas
- servicio del sistema o de usuario preconfigurado
- metapaquete y luego ISO custom
- MCP como capa de extensibilidad estandar
- UI web solo como panel auxiliar opcional, nunca como ruta principal

## 17. Lo que vamos a necesitar tu y yo para desarrollarlo

## Personas

- 1 persona enfocada en backend Go (gateway, daemon, tools, MCP)
- 1 persona enfocada en app nativa GTK/Granite (interfaz principal)
- nota: idealmente la misma persona o equipo trabaja ambos desde el inicio para que avancen juntos
- opcional: 1 persona para CLI, tooling auxiliar y panel web de diagnostico

## Infraestructura

- 1 maquina dev
- 2 VMs de elementary OS
- repositorio Git
- CI minima
- almacenamiento de secretos local seguro

## Herramientas

- Go
- Node para frontend
- Meson/Ninja
- systemd user services
- Docker/Podman
- mcp-go
- SQLite
- WebSocket tooling

## Entregables iniciales

- App nativa funcional (chat basico)
- Gateway que sirve a la app
- servicio systemd
- CLI complementaria
- tools base
- skills loader
- MCP client/server
- empaquetado basico .deb

## 18. Recomendacion final de ejecucion

Orden exacto recomendado:

1. definir protocolo interno de Gateway y comunicacion app-daemon
2. construir Gateway en Go + App nativa GTK4/Granite juntos
3. convertir el Gateway en servicio instalable
4. agregar tools locales base y exponerlas en la app
5. agregar skills y exponerlas en la app
6. agregar MCP client/server y exponerlo en la app
7. solidificar persistencia, logs y settings en la app
8. agregar CLI como complemento operativo
9. empaquetar como .deb y metapaquete
10. integrar con Wingpanel/Switchboard
11. construir una ISO custom

## 19. Siguiente paso inmediato recomendado

Lo siguiente mas util no es seguir investigando; es arrancar el repo con la arquitectura base.

Orden del siguiente sprint:

1. crear monorepo
2. crear `claw gateway start` + app nativa minima GTK4/Granite que se conecta al gateway
3. crear servicio `elementary-claw.service`
4. implementar tools `exec`, `read_file`, `list_dir` y mostrarlas en la app
5. conectar primer server MCP
6. la app ya debe poder: abrir, chatear, ver tools, ver estado del daemon

---

Si retomamos desde este documento, la mejor siguiente accion es construir el esqueleto real del proyecto en este workspace.

## 20. Doble check de la investigacion

Se hizo una segunda validacion contra documentacion oficial y repos principales. Resultado: la direccion tecnica del documento se sostiene.

### Confirmaciones fuertes

- elementary OS construye la ISO con la version Debian de `live-build`, no con la variante parcheada de Ubuntu.
- el repo de build del sistema es `elementary/os` y sigue usando `build.sh`, `etc/terraform-*.conf`, hooks de `live-build` y empaquetado basado en listas de paquetes.
- Wingpanel es efectivamente el panel superior extensible de Pantheon y se construye con Vala + Meson + GTK4 + Granite.
- System Settings vive hoy en `elementary/settings`; funciona como contenedor extensible para plugs de configuracion.
- Granite sigue siendo la libreria clave que extiende GTK con widgets y utilidades del ecosistema elementary.
- OpenClaw realmente gira alrededor de un Gateway always-on con WebSocket control plane, eventos, clientes y nodos conectados al mismo puerto.
- MCP sigue siendo la mejor capa estandar para conectores externos: host, client y server sobre JSON-RPC 2.0 con transportes `stdio` y `Streamable HTTP`.

### Correccion importante

- En una version anterior se hablaba de `elementary/switchboard` como repo principal actual de Settings. El repo vigente que hay que tomar como referencia es `elementary/settings`.

### Conclusiones despues del doble check

- La propuesta de hacer `elementary-claw` como componente nativo del sistema operativo sigue siendo correcta.
- La ruta de integracion recomendada no cambia: paquete nativo + servicio + app + integracion con shell + metapaquete + ISO custom.
- MCP si debe formar parte del diseño desde el inicio.
- Go si encaja bien para Gateway, CLI, servicios, tooling local y conectores.
- Si usan panel web auxiliar, debe quedar subordinado a la experiencia nativa de la app de escritorio.
- La app nativa es la interfaz del sistema desde el primer momento, no una fase posterior.