# Probar el PoC en Mac con UTM

## Ruta recomendada para esta Mac

Esta maquina es:

- macOS
- Apple Silicon (`arm64`)
- sin hipervisor instalado al momento de preparar este repo

Para **probar hoy mismo** el flujo de `Initial Setup` con `Connect AI`, la ruta recomendada es:

1. instalar `UTM`
2. crear una VM `arm64` en UTM usando virtualizacion cuando este disponible
3. arrancar con la ISO oficial de elementary OS
4. copiar el bundle de este repo a la VM
5. ejecutar el script de provision dentro del guest
6. correr `io.elementary.initial-setup`

## Por que `arm64` primero y `x86_64` como fallback

Como esta Mac es Apple Silicon, si ya descargaste la ISO `arm64`, esa es la ruta mas directa: menos sobrecarga, arranque mas rapido y sin emulacion completa del guest.

Si UTM o esa ISO te dan guerra, el fallback sigue siendo `x86_64` emulado. Para este PoC cualquiera de las dos rutas sirve porque lo importante es validar:

1. el flujo visual del wizard
2. el device flow de GitHub
3. la escritura de `~/.openclaw` en el usuario nuevo

## 1. Instalar UTM en el host

Si Homebrew esta disponible:

```bash
brew install --cask utm
```

Luego abre `UTM.app`.

## 2. Descargar elementary OS

Puedes descargar la ISO manualmente desde:

- `https://elementary.io/`

O usar el helper del repo para resolver la URL actual y descargarla:

```bash
cd /Users/oscarcode/elementary-claw
chmod +x vm/download-elementary-iso.sh
./vm/download-elementary-iso.sh --arch amd64
```

Eso deja la ISO en una ruta simple por defecto:

```bash
~/Downloads/elementaryos-amd64.iso
```

Si elementary.io redirige la descarga a HTML en vez de servir la ISO, usa el flujo manual en el navegador y guarda el archivo en esa misma ruta.

Si ya descargaste una ISO como esta:

```bash
~/Downloads/elementaryos-8.1-stable-arm64.20260219.iso
```

puedes usarla directamente en UTM y saltarte el helper.

## 3. Crear el bundle que vas a pasar a la VM

Desde este repo en tu Mac:

```bash
cd /Users/oscarcode/elementary-claw
./vm/package-vm-bundle.sh
```

Eso genera:

```bash
/Users/oscarcode/elementary-claw/vm/dist/elementary-claw-vm-share.tar.gz
```

## 4. Crear la VM en UTM

En UTM:

1. `Create a New Virtual Machine`
2. si usas la ISO `arm64`, elige `Virtualize`; si usas `amd64`, elige `Emulate`
3. sistema `Linux`
4. conecta la ISO de elementary
5. si elegiste emulacion, usa arquitectura `x86_64`

Valores sugeridos:

- CPU: `4`
- RAM: `8192 MB`
- Disk: `40 GB`

No bajar de `20 GB`. En la practica, para esta PoC el guest termina quedandose sin espacio rapido por recompilaciones, caches de `apt`, fuentes y multiples bundles extraidos.

En `Shared Directory`, monta una carpeta del host como esta:

```bash
/Users/oscarcode/elementary-claw/vm/dist
```

## 5. Instalar elementary OS dentro de la VM

Haz la instalacion normal del sistema.

Cuando termines y llegues al escritorio, copia o extrae el bundle compartido dentro del guest.

## 6. Extraer el bundle dentro de la VM

Ya dentro de elementary OS:

Si el panel de la VM muestra `Machine: QEMU ...`, monta el shared folder con `9p` asi:

```bash
sudo mkdir -p /mnt/utm
sudo mount -t 9p -o trans=virtio,version=9p2000.L share /mnt/utm
sudo ls -la /mnt/utm
```

Si usas UTM con Apple Virtualization y el shared folder no aparece automaticamente, montalo con `virtiofs` asi:

```bash
sudo mkdir -p /mnt/utm
sudo mount -t virtiofs share /mnt/utm
ls /mnt/utm
```

Deberias ver `elementary-claw-vm-share.tar.gz` dentro de `/mnt/utm`.

Luego extrae el bundle:

```bash
mkdir -p ~/elementary-claw-vm
cd ~/elementary-claw-vm
sudo cp /mnt/utm/elementary-claw-vm-share.tar.gz ~/
tar -xzf ~/elementary-claw-vm-share.tar.gz
```

Si ese mount falla o `/mnt/utm` sale vacio, revisa que la VM tenga configurado el `Shared Directory` apuntando a `/Users/oscarcode/elementary-claw/vm/dist`. Como fallback, tambien puedes arrastrar el tarball al escritorio de la VM o usar `scp`.

## 7. Provisionar la VM para el PoC

Dentro del guest:

```bash
cd ~/elementary-claw-vm
chmod +x vm/provision-elementary-vm.sh
./vm/provision-elementary-vm.sh
```

Ese script:

1. instala dependencias de build
2. compila `references/initial-setup`
3. instala el binario parcheado
4. compila e instala `claw` desde este repo
5. instala el servicio `elementary-claw.service` para usuarios futuros

## 8. Ejecutar la prueba

Luego:

```bash
io.elementary.initial-setup
```

El PoC instala una regla de Polkit para permitir esta prueba desde una sesion admin local dentro de la VM, sin tener que lanzar la UI completa como root.

Si esta VM fue provisionada antes de ese ajuste, reinstala el build con:

```bash
sudo meson install -C ~/elementary-claw-vm/references/initial-setup/build
```

Haz el flujo completo:

1. idioma
2. teclado
3. red
4. crear usuario
5. pantalla `Connect AI`

## 9. Validacion esperada

Despues de autorizar GitHub Copilot:

1. aparece estado `Connected`
2. `Finish Setup` cierra el wizard
3. existen estos archivos en el home del usuario nuevo:

```bash
~/.openclaw/openclaw.json
~/.openclaw/agents/main/agent/auth-profiles.json
```

Despues del primer login del usuario nuevo, el servicio `elementary-claw` debe arrancar y preparar la sesion `bootstrap` automaticamente.

La validacion detallada ya esta en:

- `/Users/oscarcode/elementary-claw/initial-setup-vm-runbook.md`

## 10. Si quieres acelerar aun mas

La prueba mas rapida no es reinstalar la VM desde cero cada vez. Despues de la primera instalacion:

1. crea un snapshot justo antes de correr `io.elementary.initial-setup`
2. repite el PoC restaurando ese snapshot

## 11. Notas de campo de esta iteracion

El resumen operativo de lo aprendido en la sesion del 2026-04-03 esta en:

- `/Users/oscarcode/elementary-claw/vm/2026-04-03-field-notes.md`

## Lo que no estamos haciendo todavia

1. no estamos generando una ISO custom
2. no estamos metiendo el parche en la instalacion real de elementary OS
3. no estamos haciendo autostart en primer login

Eso viene despues. Para hoy, esto valida el flujo correcto en hardware Apple Silicon usando macOS como host.