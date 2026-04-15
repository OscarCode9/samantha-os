# PoC de GitHub Copilot en Initial Setup

## Objetivo de hoy

Probar en una VM un flujo real donde `io.elementary.initial-setup`:

1. crea el usuario
2. muestra una nueva pantalla `Connect AI`
3. ejecuta el device flow de GitHub
4. escribe `~/.openclaw/openclaw.json`
5. escribe `~/.openclaw/agents/main/agent/auth-profiles.json`
6. siembra `AGENTS.md`, `SOUL.md`, `IDENTITY.md`, `USER.md`, `TOOLS.md`, `HEARTBEAT.md` y `BOOTSTRAP.md`
7. deja al usuario nuevo listo para usar `elementary-claw` con GitHub Copilot y un primer saludo personalizado

## Alcance real de este PoC

Esto **no** modifica todavía el ISO ni el arranque del instalador oficial. La forma más rápida de probarlo hoy es:

1. levantar una VM de elementary OS
2. compilar e instalar esta variante de `initial-setup`
3. ejecutar `io.elementary.initial-setup` manualmente dentro de la VM
4. crear un usuario de prueba
5. conectar GitHub Copilot desde la nueva pantalla
6. precargar la personalidad base del agente y el primer onboarding de workspace

Eso valida el flujo crítico sin esperar empaquetado del sistema completo.

## Código tocado

- `references/initial-setup/src/MainWindow.vala`
- `references/initial-setup/src/Views/AccountView.vala`
- `references/initial-setup/src/Views/AIConnectView.vala`
- `references/initial-setup/src/Helpers/OpenClawBootstrap.vala`
- `references/initial-setup/src/meson.build`
- `references/initial-setup/meson.build`

## Precondiciones en la VM

- VM con elementary OS 8.x
- acceso a internet
- una cuenta con GitHub Copilot activo
- snapshot de la VM antes de instalar cambios

## 1. Instalar dependencias de compilacion

```bash
sudo apt update
sudo apt install -y \
  build-essential \
  curl \
  desktop-file-utils \
  gettext \
  jq \
  libaccountsservice-dev \
  libadwaita-1-dev \
  libgnomekbd-dev \
  libgranite-7-dev \
  libgtk-4-dev \
  libjson-glib-dev \
  libpantheon-wayland-1-dev \
  libpwquality-dev \
  libsoup-3.0-dev \
  libx11-dev \
  libxkbregistry-dev \
  meson \
  ninja-build \
  valac
```

## 2. Llevar el repo a la VM

La ruta esperada en este runbook es:

```bash
~/elementary-claw-vm
```

Si ya tienes el repo en otra ruta, ajusta los comandos.

## 3. Compilar e instalar la variante modificada de Initial Setup

```bash
cd ~/elementary-claw-vm/references/initial-setup
meson setup build --prefix=/usr
ninja -C build
sudo meson install -C build
sudo fc-cache -f /usr/share/fonts/truetype/io.elementary.initial-setup
```

## 4. Instalar el runtime Go de elementary-claw en la VM

Para la prueba de hoy, usa el runtime local del repo:

```bash
cd ~/elementary-claw-vm
export GOTOOLCHAIN=auto
go build -o claw ./cmd/claw
sudo install -d /usr/local/bin
sudo install -m 0755 claw /usr/local/bin/claw
sudo install -d /etc/systemd/user
sudo install -m 0644 deployments/systemd/elementary-claw.service /etc/systemd/user/elementary-claw.service
sudo systemctl --global enable elementary-claw.service
claw --help | head
```

Si prefieres el camino corto, `./vm/provision-elementary-vm.sh` ya hace esto automaticamente.

## 5. Ejecutar Initial Setup manualmente

Desde una sesion admin dentro de la VM:

```bash
io.elementary.initial-setup
```

El PoC instala una regla de Polkit para permitir esta prueba desde una sesion admin local dentro de la VM, sin tener que lanzar la UI completa como root.

Si esta VM fue provisionada antes de ese ajuste, reinstala el build con:

```bash
sudo meson install -C ~/elementary-claw-vm/references/initial-setup/build
```

Recorre el wizard normal:

1. idioma
2. teclado
3. red
4. crear usuario con el boton `Create Account`

En la PoC manual desde una sesion admin, la escritura final en el home del usuario nuevo ahora se hace con un helper privilegiado instalado junto con `initial-setup`. Si el wizard no corre ya como root, puede aparecer una autenticacion local al terminar la conexion de GitHub.

Usa un usuario de prueba, por ejemplo:

- nombre: `Copilot Test`
- username: `copilottest`

## 6. Probar la nueva pantalla `Connect AI`

Despues de crear el usuario, ya no debe cerrar inmediatamente. Debe aparecer una pantalla nueva con:

- boton `Connect GitHub Copilot`
- boton `Skip for Now`
- codigo temporal de GitHub cuando arranca el device flow
- campos para `Assistant name`, `Assistant nature`, `Assistant vibe`, `Your preferred name`, `Soul directives` y `User context`

Haz esta prueba:

1. pulsa `Connect GitHub Copilot`
2. confirma que abre navegador
3. autoriza el device flow con tu cuenta GitHub
4. si el wizard fue lanzado desde una sesion admin normal, acepta la elevacion local cuando el helper de provision lo pida
4. espera a que cambie el estado a `Connected`
5. pulsa `Finish Setup`

Resultado esperado adicional:

- se muestra un preview del primer welcome message
- el workspace del usuario queda presembrado con los archivos de onboarding

## 7. Verificar que los archivos quedaron en el home correcto

Sin imprimir el token completo, valida estructura y modo:

```bash
sudo ls -la /home/copilottest/.openclaw
sudo ls -la /home/copilottest/.openclaw/agents/main/agent
sudo ls -la /home/copilottest/.openclaw/workspace
sudo stat -c '%U %G %a %n' /home/copilottest/.openclaw/openclaw.json
sudo stat -c '%U %G %a %n' /home/copilottest/.openclaw/agents/main/agent/auth-profiles.json
sudo jq -r '.profiles | keys[]' /home/copilottest/.openclaw/agents/main/agent/auth-profiles.json
sudo jq -r '.profiles["github-copilot:default"].mode' /home/copilottest/.openclaw/agents/main/agent/auth-profiles.json
sudo jq -r '.agent.model' /home/copilottest/.openclaw/openclaw.json
sudo jq -r '.agent.provider' /home/copilottest/.openclaw/openclaw.json
sudo jq -r '.setup.bootstrapReady' /home/copilottest/.openclaw/openclaw.json
```

Resultado esperado:

- owner `copilottest`
- permisos `600` para ambos JSON
- existe el profile id `github-copilot:default`
- el modo configurado es `token`
- el proveedor configurado es `github-copilot`
- el modelo configurado es `github-copilot/gpt-5.4`
- `bootstrapReady` queda en `true`
- existen `AGENTS.md`, `SOUL.md`, `IDENTITY.md`, `USER.md`, `TOOLS.md`, `HEARTBEAT.md`, `BOOTSTRAP.md`

Tambien revisa rapido el contenido sembrado:

```bash
sudo sed -n '1,80p' /home/copilottest/.openclaw/workspace/IDENTITY.md
sudo sed -n '1,120p' /home/copilottest/.openclaw/workspace/SOUL.md
sudo sed -n '1,120p' /home/copilottest/.openclaw/workspace/USER.md
sudo sed -n '1,160p' /home/copilottest/.openclaw/workspace/BOOTSTRAP.md
```

## 8. Verificar ya dentro del usuario nuevo

Cierra sesion y entra con `copilottest`.

No hagas esta validacion desde el usuario admin que lanzo `io.elementary.initial-setup`. El estado de la PoC se escribe para el usuario nuevo creado por el wizard, no para la sesion admin desde la que corriste la UI manual.

Luego ejecuta:

```bash
systemctl --user status elementary-claw --no-pager
claw gateway status
curl -s http://127.0.0.1:4389/healthz | jq
curl -s http://127.0.0.1:4389/v1/sessions/bootstrap | jq
claw providers github-copilot exchange
```

Lo que quieres comprobar:

1. que el servicio `elementary-claw` arranca en el login del usuario nuevo
2. que `gateway status` reporta config y auth presentes
3. que existe la sesion `bootstrap` en `/v1/sessions/bootstrap`
4. que `claw providers github-copilot exchange` reutiliza el token de GitHub sin pedir re-login inmediato

## 9. Prueba negativa rapida

Repite el flujo con otro usuario y usa `Skip for Now`.

Ese usuario:

1. no debe bloquear el final del setup
2. no debe tener `auth-profiles.json` poblado con GitHub Copilot

## 10. Qué valida este PoC

Si todo sale bien, ya queda validado que:

1. `Initial Setup` es un punto correcto para ofrecer conexion de IA despues de crear usuario
2. el device flow de GitHub puede ejecutarse desde la UX de onboarding
3. los secretos pueden caer directamente en el home del usuario correcto
4. `elementary-claw` puede arrancar despues usando ese estado sin pedir configuracion manual extra

## Limitaciones actuales

1. El token queda en `auth-profiles.json`. No hay integracion con keyring todavia.
2. El runtime actual levanta gateway y sesion bootstrap, pero todavia no hay app nativa consumiendo esa sesion en el primer login.
3. No hay empaquetado `.deb`, metapaquete, ni integracion en ISO todavia.
4. El modelo por defecto queda fijo en `github-copilot/gpt-5.4` para que el agente arranque con el modelo objetivo del producto.

## Siguiente paso recomendado

Si esta prueba pasa, el siguiente bloque de trabajo deberia ser:

1. empaquetar `initial-setup` parcheado como build reproducible
2. empaquetar `elementary-claw` para que venga preinstalado
3. mover el almacenamiento de token a Secret Service o mecanismo equivalente
4. enganchar una app nativa de primer login al gateway y a la sesion `bootstrap`