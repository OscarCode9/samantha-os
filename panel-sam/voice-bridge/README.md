# Samantha Voice Bridge (Host macOS API)

Este directorio contiene el puente de voz para el indicador `panel-sam` de la máquina virtual. 

Dado que el audio en máquinas virtuales (UTM) suele tener latencia y distorsión, esta API corre directamente en tu Mac (Host), capturando tu micrófono y reproduciendo la voz de Samantha nativamente, pero sincronizada con el panel visual de la VM.

---

## Requisitos Previos en macOS (Mac Host)

1. **Instalar PortAudio** (requerido para capturar audio desde Python):
   ```bash
   brew install portaudio
   ```

2. **Crear Entorno Virtual de Python (Recomendado Python 3.11):**
   ```bash
   cd /Users/oscarcode/elementary-claw/panel-sam/voice-bridge
   python3 -m venv venv
   source venv/bin/activate
   ```

3. **Instalar Dependencias:**
   ```bash
   pip install -r requirements.txt
   ```

---

## Ejecutar la API en macOS

Con el entorno virtual activo, inicia el servidor:
```bash
python3 bridge.py
```

El servidor levantará en `http://0.0.0.0:5005` (escuchando en todas las interfaces para que la VM de UTM se conecte usando la IP del Mac: `192.168.64.1`).

---

## ¿Cómo Funciona la Voz Local?

*   **Texto a Voz (TTS):** El script intentará cargar el modelo neuronal **Chatterbox Multilingual V3** en la GPU del Mac (usando `mps` para Apple Silicon). 
    *   *Nota:* Si el paquete `chatterbox-tts` no está instalado o el modelo no está descargado aún, la API caerá automáticamente y con **cero latencia** en el comando nativo de voz de macOS (`say -v Paulina`), permitiéndote probar la integración de inmediato con una excelente voz en español.
*   **Voz a Texto (STT) con Silencio Inteligente (VAD):** Usa `speech_recognition` con calibración de ruido ambiental. Cuando haces clic en el botón de micrófono en el panel, comenzará a grabar y se detendrá automáticamente en cuanto dejes de hablar (1 segundo de silencio). La transcripción se envía directamente como pregunta a Samantha.
