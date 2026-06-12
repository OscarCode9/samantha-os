---
name: compile-samantha
description: "Instructions on how to compile and install all components of Samantha OS: the Go engine runtime, the Vala Wingpanel indicator, and the custom Initial Setup wizard."
---

# Compilar Samantha OS

Esta guía describe cómo compilar e instalar los tres componentes principales del proyecto Samantha OS en una máquina local o VM con elementary OS.

---

## 1. Clonar el Repositorio (Paso Inicial)

Si no lo has hecho aún, clona el repositorio del proyecto:

```bash
git clone https://github.com/OscarCode9/samantha-os.git
cd samantha-os
```

---

## 2. Compilar e Instalar el Motor de Samantha (Go Backend)

El backend de Samantha es un binario en Go que expone la API local en el puerto `4389` y ejecuta herramientas del sistema.

### Requisitos de Go:
Asegúrate de tener Go instalado (`golang-go` o superior).

### Pasos de compilación:
1. Compila el binario:
   ```bash
   go build -o claw ./cmd/claw
   ```

2. Instala el binario en la ruta del sistema:
   ```bash
   sudo install -m 0755 claw /usr/local/bin/claw
   ```

3. Registra e inicia el servicio de usuario de systemd:
   ```bash
   sudo install -d /etc/systemd/user
   sudo install -m 0644 deployments/systemd/elementary-claw.service /etc/systemd/user/elementary-claw.service
   systemctl --user daemon-reload
   systemctl --user enable --now elementary-claw.service
   ```

4. Verifica el estado del servicio:
   ```bash
   systemctl --user status elementary-claw
   claw gateway status
   ```

---

## 3. Compilar e Instalar el Indicador de Panel en Vala (panel-sam)

El indicador del panel se integra con el Wingpanel de elementary OS y proporciona la interfaz de chat en la barra superior.

### Requisitos:
Requiere dependencias de GTK3 y Wingpanel:
```bash
sudo apt install -y build-essential meson ninja-build valac libwingpanel-dev libsoup-3.0-dev libjson-glib-dev libgranite-7-dev libgtk-3-dev
```

### Pasos de compilación:
1. Entra al directorio del panel:
   ```bash
   cd panel-sam
   ```

2. Configura el sistema de construcción Meson (con prefijo del sistema `/usr`):
   ```bash
   meson setup build --prefix=/usr
   ```

3. Compila el proyecto:
   ```bash
   ninja -C build
   ```

4. Instala el módulo del indicador y hojas de estilo:
   ```bash
   sudo ninja -C build install
   ```

5. Reinicia Wingpanel para cargar el nuevo indicador:
   ```bash
   killall io.elementary.wingpanel
   ```

---

## 4. Compilar e Instalar el Initial Setup (references/initial-setup)

El Initial Setup es el asistente de bienvenida del sistema operativo que ha sido modificado para incluir la conexión inicial con GitHub Copilot o ChatGPT.

### Requisitos de construcción adicionales:
```bash
sudo apt install -y libaccountsservice-dev libadwaita-1-dev libgnomekbd-dev libpantheon-wayland-1-dev libpwquality-dev libx11-dev libxkbregistry-dev
```

### Pasos de compilación:
1. Entra al directorio del submódulo:
   ```bash
   cd references/initial-setup
   ```

2. Configura Meson:
   ```bash
   meson setup build --prefix=/usr
   ```

3. Compila el asistente:
   ```bash
   ninja -C build
   ```

4. Instala el ejecutable y los recursos del sistema:
   ```bash
   sudo ninja -C build install
   sudo fc-cache -f /usr/share/fonts/truetype/io.elementary.initial-setup
   ```

5. Para ejecutarlo manualmente en una sesión de administrador:
   ```bash
   io.elementary.initial-setup
   ```
