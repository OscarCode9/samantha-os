# Resolución de Conexión de ChatGPT en Initial Setup (Onboarding)

Este documento detalla el diagnóstico y las soluciones implementadas durante la sesión para solucionar los errores surgidos al conectar una cuenta de ChatGPT al asistente de configuración inicial (`io.elementary.initial-setup`) en la máquina virtual.

---

## 1. Problemas Identificados y Causas Raíz

### A. Bloqueo de Cloudflare / Peticiones de Pasarela
*   **Problema:** Al enviar la URL de retorno del inicio de sesión de ChatGPT, la interfaz mostraba el error genérico `(unexpected response from gateway)`.
*   **Causa Raíz:** La pasarela en Go realiza la solicitud de intercambio del código de autorización PKCE contra el endpoint `https://auth.openai.com/oauth/token`. Al usar el cliente HTTP por defecto de Go, la cabecera `User-Agent` quedaba como `Go-http-client/1.1`. Cloudflare (que protege a OpenAI) bloquea estas firmas por defecto, respondiendo con un error HTTP `502 Bad Gateway` y el mensaje de texto `"unexpected response from gateway"`.

### B. Mismatch de Modelos en OpenAI Codex
*   **Problema:** Una vez superado el bloqueo, la pasarela devolvía un error indicando que el modelo `gpt-5.3-codex` no era compatible con la cuenta de ChatGPT.
*   **Causa Raíz:** Las cuentas de ChatGPT sobre Codex esperan el modelo `gpt-5.4`. Sin embargo, en la pasarela el modelo predeterminado para Codex estaba configurado como `gpt-5.3-codex`. Por otro lado, la interfaz gráfica del onboarding (`initial-setup`) envía cableado el modelo `gpt-5.4`. Al carecer de una traducción interna correcta, la petición fallaba.

### C. Conflicto de Usuarios en la VM ("Conexión Rehusada")
*   **Problema:** Tras recompilar el código, el asistente mostraba `no se puede conectar a 127 conexion rehusada`.
*   **Causa Raíz:** Al provisionar la VM con el script `./vm/provision-elementary-vm.sh` ejecutado por `oscarcode91`, la directiva `systemctl --user enable --now` activaba la pasarela bajo el usuario de SSH (`oscarcode91`). 
    Dado que la interfaz gráfica (el GUI de la VM) y el asistente de configuración `initial-setup` corren bajo la sesión del usuario principal **`oscar`**, este último intentaba conectar al puerto local `4389`, pero el daemon bajo `oscar` estaba inactivo (`dead`) al no poder enlazar el puerto que ya estaba ocupado por `oscarcode91`.

### D. Error de Parámetros Requeridos ("missing_required_parameter")
*   **Problema:** Tras corregir el puerto y el modelo, el chat del asistente no cargaba y reportaba:
    `One of "input" or "previous_response_id" or 'prompt' or 'conversation_id' must be provided.`
*   **Causa Raíz:** Al iniciar el chat de onboarding, `AIConnectView.vala` limpia el historial y envía únicamente el mensaje de sistema (`system` - `SYSTEM_PROMPT`).
    El API de ChatGPT Codex separa las instrucciones (`system`) del historial conversacional (`input`). Al ser el primer mensaje y no existir interacción previa del usuario, el arreglo `input` se enviaba vacío (`[]`). El endpoint de OpenAI rechaza peticiones con `input` vacío bajo esa regla de validación.

---

## 2. Soluciones Implementadas

### 1. Inyección de `User-Agent`
*   Agregamos la cabecera `User-Agent: elementary-claw/1.0` en todas las solicitudes salientes a los endpoints de OpenAI (intercambio de tokens, refresco de tokens y flujo de respuestas) en los archivos:
    *   [internal/providers/openaicodex/auth.go](file:///Users/oscarcode/elementary-claw/internal/providers/openaicodex/auth.go)
    *   [internal/providers/openaicodex/responses.go](file:///Users/oscarcode/elementary-claw/internal/providers/openaicodex/responses.go)

### 2. Actualización de Modelo Codex por Defecto
*   Se actualizó el `DefaultModel` a `"gpt-5.4"` en [internal/providers/openaicodex/auth.go](file:///Users/oscarcode/elementary-claw/internal/providers/openaicodex/auth.go).
*   Se modificó la función `normalizeModel` en [internal/providers/openaicodex/responses.go](file:///Users/oscarcode/elementary-claw/internal/providers/openaicodex/responses.go) para aceptar tanto `gpt-5.4` como `gpt-5.3-codex`, y revertir a `DefaultModel` (`gpt-5.4`) si el cliente solicita cualquier otro modelo genérico (como el hardcodeado `gpt-5.4` de Copilot) durante la conexión Codex.

### 3. Modificación del Flujo de Inicialización de Conversación (Fake Input)
*   En la función `convertMessages` en [internal/providers/openaicodex/responses.go](file:///Users/oscarcode/elementary-claw/internal/providers/openaicodex/responses.go), añadimos una validación: si la longitud de `input` es cero (es decir, el chat recién arranca y solo hay prompt de sistema), inyectamos un mensaje conversacional inicial del lado del usuario de forma transparente:
    ```go
    if len(input) == 0 {
        input = append(input, map[string]any{
            "type": "message",
            "role": "user",
            "content": []map[string]any{
                {"type": "input_text", "text": "Hello, let's start the onboarding setup."},
            },
        })
    }
    ```
    Esto cumple las reglas de OpenAI y desencadena el saludo inicial del asistente sin alterar el historial visual del usuario.

### 4. Manejo y Rediseño de Mensajes de Error en la UI (Nothing Style)
*   Modificamos `call_gateway` en [references/initial-setup/src/Views/AIConnectView.vala](file:///Users/oscarcode/elementary-claw/references/initial-setup/src/Views/AIConnectView.vala) para que valide el código de estado HTTP. Si no es `200`, ahora extrae el texto del mensaje de error real en lugar de ignorarlo.
*   Creamos una función `append_error_bubble` en Vala para que los mensajes de error del sistema no se atribuyan al asistente (`SEMANTHA`), sino que muestren el rol de **`SYSTEM`**.
*   Añadimos reglas en [nothing-installer.css](file:///Users/oscarcode/elementary-claw/references/initial-setup/data/styles/nothing-installer.css) (`.ai-msg-role-error` and `.ai-msg-text-error`) usando el color de advertencia coral suave `#FFD4CC` para que el error de cuota o cualquier fallo de red se apegue orgánicamente a la estética Nothing.

### 5. Configuración de Servicios en la VM
*   Se eliminó el inicio global (`--global`) del servicio `elementary-claw` en la VM para evitar que sesiones secundarias de SSH ocupen el puerto `4389`.
*   Se deshabilitó y detuvo el servicio en `oscarcode91`.
*   Se habilitó e inició de manera local para el usuario gráfico **`oscar`**, permitiendo la comunicación local sin interferencias.

---

## 3. Comandos Útiles para Futura Sincronización en VM

Si se vuelven a realizar cambios en el host y se desea actualizar el entorno de la VM:

```bash
# 1. Empaquetar el código en el Host macOS
./vm/package-vm-bundle.sh

# 2. En la terminal de la VM: Montar la carpeta compartida
sudo mkdir -p /mnt/utm
sudo mount -t 9p -o trans=virtio,version=9p2000.L share /mnt/utm 2>/dev/null || sudo mount -t virtiofs share /mnt/utm 2>/dev/null

# 3. Descomprimir el bundle
cd ~/
tar -xzf /mnt/utm/elementary-claw-vm-share.tar.gz

# 4. Provisionar e instalar
cd ~/elementary-claw-vm
chmod +x vm/provision-elementary-vm.sh
./vm/provision-elementary-vm.sh

# 5. Asegurar que el servicio corre en el usuario de la UI (oscar)
systemctl --user daemon-reload
systemctl --user restart elementary-claw

# 6. Lanzar el asistente
io.elementary.initial-setup
```
