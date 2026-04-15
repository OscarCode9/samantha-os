# Field Notes - 2026-04-03

Este documento resume lo que se valido, lo que fallo y lo que ya quedo corregido durante la sesion de hoy en la VM de elementary OS.

## 1. Resultado del dia

Se valido el flujo principal del PoC:

1. `initial-setup` crea un usuario nuevo.
2. En vez de cerrar inmediatamente, avanza a `Connect AI`.
3. El device flow de GitHub Copilot puede completarse desde esa pantalla.
4. El estado de `elementary-claw` se escribe en el home del usuario nuevo, no en la sesion admin que lanzo el wizard.
5. El wizard llega a estado `Connected` y deja preview del welcome inicial.

La VM termino quedandose sin espacio despues de varias iteraciones de reinstalacion, recompilacion y nuevas extracciones del bundle. La recomendacion para el siguiente intento es borrar esta VM y retomar desde cero con un guest mas amplio y snapshots.

## 2. Lo que quedo confirmado

### 2.1 Product / architecture

- El punto correcto para crear el agente es `Initial Setup` despues de crear la cuenta, no el instalador puro ni el primer login.
- La diferenciacion del producto sigue siendo valida: el agente nace durante setup y el primer login solo retoma ese estado.
- La ruta correcta para v1 es `github-copilot` nativo, no `copilot-proxy` como camino principal del producto.
- El runtime correcto para esta PoC en guest es `claw` en Go, no instalar OpenClaw en la VM.

### 2.2 Runtime / state

- El estado escrito por la PoC usa `~/.openclaw`.
- El config principal vive en `~/.openclaw/openclaw.json`.
- El auth store principal vive en `~/.openclaw/agents/main/agent/auth-profiles.json`.
- El workspace sembrado vive en `~/.openclaw/workspace`.
- El proveedor por defecto queda en `github-copilot`.
- El modelo por defecto queda en `github-copilot/gpt-5.4`.
- El runtime de usuario esperado sigue siendo `elementary-claw.service` escuchando en `127.0.0.1:4389`.

### 2.3 Installer / onboarding

- `AccountView` ya no es el final del flujo. Emite `account_ready` y el wizard empuja `AIConnectView`.
- La pantalla `Connect AI` ya soporta device flow de GitHub y captura identidad base del agente.
- La vista ya no depende solo del layout original del installer: ahora tiene una hoja de estilo propia y una composicion mas fuerte.
- Las fuentes reales del sistema visual Nothing que se empaquetaron son:
  - `Doto`
  - `Space Grotesk`
  - `Space Mono`

### 2.4 VM / host setup

- La Mac host corre macOS en Apple Silicon.
- UTM si fue suficiente para probar la PoC.
- El guest real usado en las pruebas termino siendo QEMU, no Apple Virtualization.
- En QEMU el shared folder correcto se monta con `9p`, no `virtiofs`.
- Si mas adelante se usa Apple Virtualization, ahi si aplica `virtiofs`.

## 3. Problemas reales encontrados hoy

### 3.1 ISO y host tooling

- `vm/download-elementary-iso.sh` puede resolver la URL, pero elementary.io puede responder HTML en vez de una ISO descargable por CLI.
- Cuando eso pase, la descarga manual en navegador sigue siendo el fallback correcto.

### 3.2 Shared folders

- Si el guest muestra `Machine: QEMU ...`, el mount correcto es:

```bash
sudo mkdir -p /mnt/utm
sudo mount -t 9p -o trans=virtio,version=9p2000.L share /mnt/utm
```

- Si `/mnt/utm` sale vacio, lo primero que hay que revisar es el `Shared Directory` configurado en UTM, no el script de guest.

### 3.3 Disk sizing

- 10 GB no son suficientes para iterar con esta PoC.
- 20 GB es apenas aceptable para pruebas muy cuidadas.
- 40 GB es la recomendacion real para retomar sin perder tiempo limpiando guest constantemente.

### 3.4 APT y compilacion en guest

- El guest llego a tener metadata rota de `apt`.
- El provision script ya repara listas y refresca metadata antes de instalar dependencias.
- El camino que funciono mejor fue volver al flujo source-based y compilar dentro del guest.

### 3.5 Permisos y polkit

- Lanzar `io.elementary.initial-setup` manualmente desde una sesion admin normal no se comporta igual que una sesion stock de `lightdm`.
- `pkexec` directo para toda la UI fue mala ruta por problemas de display, bus y sesion.
- El fix correcto fue ampliar la regla de Polkit para permitir sesiones admin locales activas, ademas de `lightdm`.

### 3.6 Escritura del home del usuario nuevo

- El wizard manual podia crear el usuario, pero luego fallaba al intentar crear `~/.openclaw` dentro del home nuevo.
- La causa era que el proceso interactivo no tenia permisos reales de filesystem sobre el home privado del usuario recien creado.
- El fix implementado fue un helper privilegiado instalado junto con `initial-setup` para provisionar el home del usuario nuevo cuando el wizard no corre ya como root.

### 3.7 Prueba en el usuario equivocado

- La validacion del runtime no debe hacerse desde el usuario admin que lanzo el wizard.
- Si el wizard creo `oscarmartinez`, el estado y el servicio relevantes viven para `oscarmartinez`, no para `oscar`.
- Si se intenta leer `~/.openclaw` desde el usuario admin, el resultado esperado es que no exista.

### 3.8 UI / visual quality

- La primera version de `Connect AI` tenia un problema real de layout: el contenido empujaba el footer fuera de vista.
- Luego aparecio un segundo problema: aunque funcionalmente correcta, la pantalla seguia viendose como formulario GTK maquillado.
- Para corregir eso se hicieron varios cambios acumulados:
  - `Gtk.ScrolledWindow` en el contenido
  - layout asimetrico para la vista AI
  - ventana mas amplia al entrar al paso AI
  - stylesheet propia
  - fonts reales vendoreadas e instaladas
  - `fc-cache` obligatorio despues de reinstalar

## 4. Cambios concretos que ya existen en el repo

### 4.1 Initial Setup PoC

- `references/initial-setup/src/MainWindow.vala`
- `references/initial-setup/src/Views/AccountView.vala`
- `references/initial-setup/src/Views/AIConnectView.vala`
- `references/initial-setup/src/Views/AbstractInstallerView.vala`
- `references/initial-setup/src/Application.vala`
- `references/initial-setup/src/Helpers/OpenClawBootstrap.vala`
- `references/initial-setup/src/ProvisionHelper.vala`

### 4.2 Data / install assets

- `references/initial-setup/data/styles/nothing-installer.css`
- `references/initial-setup/data/fonts/Doto-ROND-wght.ttf`
- `references/initial-setup/data/fonts/SpaceGrotesk-wght.ttf`
- `references/initial-setup/data/fonts/SpaceMono-Regular.ttf`
- `references/initial-setup/data/fonts/SpaceMono-Bold.ttf`

### 4.3 Guest automation

- `vm/provision-elementary-vm.sh`
- `vm/package-vm-bundle.sh`
- `vm/macos-utm-runbook.md`
- `initial-setup-vm-runbook.md`

## 5. Flujo que quedo como referencia valida

### 5.1 Host

1. Descargar la ISO manual o automaticamente si la web no bloquea.
2. Crear bundle con `./vm/package-vm-bundle.sh`.
3. Crear VM en UTM con 40 GB de disco.

### 5.2 Guest bootstrap

1. Montar shared folder correcto.
2. Extraer `elementary-claw-vm-share.tar.gz`.
3. Ejecutar `./vm/provision-elementary-vm.sh`.

### 5.3 Manual test del installer

1. Ejecutar `io.elementary.initial-setup` desde sesion admin local.
2. Crear el usuario con `Create Account`.
3. Pasar a `Connect AI`.
4. Completar device flow de GitHub.
5. Aceptar elevacion local si la pide el helper de provision.
6. Confirmar estado `Connected`.
7. Terminar el wizard.

### 5.4 Validacion del runtime

La validacion de runtime debe hacerse entrando en la sesion del usuario nuevo creado por el wizard.

Comandos utiles:

```bash
systemctl --user status elementary-claw --no-pager
claw gateway status
curl -s http://127.0.0.1:4389/healthz | jq
curl -s http://127.0.0.1:4389/v1/sessions/bootstrap | jq
claw providers github-copilot exchange
```

Prueba de chat real:

```bash
MODEL="$(jq -r '.agent.model' ~/.openclaw/openclaw.json)"

curl -s http://127.0.0.1:4389/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg model "$MODEL" \
    '{model:$model,session_id:"smoke",messages:[{role:"user",content:"Presentate en una frase y dime que ya quedaste configurado durante setup."}]}' \
  )" | jq

curl -s http://127.0.0.1:4389/v1/sessions/smoke | jq
```

## 6. Lo que sigue pendiente

- No hay integracion en ISO real.
- No hay `.deb` ni metapaquete de producto.
- El storage sensible sigue siendo archivo `0600`, no Secret Service.
- Falta la app nativa del primer login consumiendo la sesion `bootstrap`.
- La UI de `Connect AI` mejoro, pero todavia necesita refinamiento visual para que deje de sentirse como PoC.

## 7. Recomendaciones para retomar desde cero

1. Crear una VM nueva con 40 GB, no menos.
2. Crear snapshot justo despues de `./vm/provision-elementary-vm.sh` y antes de lanzar `io.elementary.initial-setup`.
3. No validar runtime desde el usuario admin del guest; validar siempre desde el usuario nuevo creado por el wizard.
4. Mantener el flujo source-based del guest mientras no exista empaquetado real.
5. Si la UI vuelve a verse igual que antes, confirmar primero que se reinstalo el build nuevo y que se corrio `fc-cache`.

## 8. Resumen corto

Hoy quedo validado que el concepto central si funciona: el agente puede crearse durante `Initial Setup`, conectarse a GitHub Copilot en ese mismo flujo y dejar al usuario nuevo con estado persistido antes de su primer login.

Lo que mato esta iteracion no fue un fallo conceptual sino deuda de entorno: montaje de shared folder, espacio de disco, permisos del wizard lanzado manualmente y varias reinstalaciones del guest.