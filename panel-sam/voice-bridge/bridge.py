import os
import torch
import numpy as np
import sounddevice as sd
import torchaudio as ta
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
import speech_recognition as sr

app = FastAPI(title="Samantha Voice Bridge")

# Lazy loading of Chatterbox model to speed up startup
tts_model = None

def get_tts_model():
    global tts_model
    if tts_model is None:
        provider = os.getenv("TTS_PROVIDER", "macos").lower()
        if provider == "macos":
            print("Using macOS native TTS (say -v Paulina).")
            tts_model = "fallback"
            return tts_model
        
        print("Loading Chatterbox Multilingual V3 model...")
        try:
            from chatterbox.mtl_tts import ChatterboxMultilingualTTS
            device = "cuda" if torch.cuda.is_available() else "mps" if torch.backends.mps.is_available() else "cpu"
            print(f"Using device: {device}")
            tts_model = ChatterboxMultilingualTTS.from_pretrained(device=device)
            print("Model loaded successfully.")
        except Exception as e:
            print(f"Error loading Chatterbox Multilingual V3: {e}")
            print("Falling back to macOS native say.")
            tts_model = "fallback"
    return tts_model

class SpeakRequest(BaseModel):
    text: str

@app.post("/speak")
async def speak(request: SpeakRequest):
    text = request.text
    print(f"Speaking text: {text}")
    model = get_tts_model()
    if model == "fallback" or model is None:
        # Fallback using macOS 'say' command which is built-in and has zero latency
        # Clean quotes to avoid syntax injection in system command
        safe_text = text.replace('"', '\\"').replace('$', '\\$').replace('`', '\\`')
        os.system(f'say -v Paulina "{safe_text}"')
        return {"status": "ok", "mode": "macos_say"}
    
    try:
        # Generate audio using Chatterbox Multilingual TTS
        wav = model.generate(text, language_id="es")
        # Save to a temporary WAV file and play using macOS native 'afplay'
        import tempfile
        import subprocess
        
        with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp_file:
            tmp_path = tmp_file.name
            
        try:
            ta.save(tmp_path, wav.cpu(), model.sr)
            subprocess.run(["afplay", tmp_path], check=True)
        finally:
            if os.path.exists(tmp_path):
                os.remove(tmp_path)
                
        return {"status": "ok", "mode": "chatterbox"}
    except Exception as e:
        print(f"Error in TTS: {e}")
        safe_text = text.replace('"', '\\"').replace('$', '\\$').replace('`', '\\`')
        os.system(f'say -v Paulina "{safe_text}"')
        return {"status": "ok", "mode": "error_fallback"}

@app.post("/listen")
async def listen():
    print("Listening from Mac microphone (VAD active)...")
    r = sr.Recognizer()
    # Adjust thresholds for silence detection
    r.dynamic_energy_threshold = True
    r.pause_threshold = 1.0  # 1 second of silence stops recording
    
    try:
        with sr.Microphone() as source:
            print("Calibrating ambient noise...")
            r.adjust_for_ambient_noise(source, duration=0.5)
            print("Listening...")
            audio = r.listen(source, timeout=10, phrase_time_limit=15)
            print("Processing audio...")
            
        # Transcribe using Google Web Speech API (zero-configuration local fallback)
        try:
            print("Transcribing via Google API...")
            transcription = r.recognize_google(audio, language="es-MX")
            print(f"Transcribed: {transcription}")
            return {"status": "ok", "text": transcription}
        except sr.UnknownValueError:
            print("Google could not understand audio")
            return {"status": "error", "message": "No se entendió el audio"}
        except Exception as google_err:
            print(f"Google error: {google_err}, trying Whisper local...")
            try:
                transcription = r.recognize_whisper(audio, language="es")
                print(f"Transcribed via Whisper: {transcription}")
                return {"status": "ok", "text": transcription}
            except Exception as whisper_err:
                print(f"Whisper error: {whisper_err}")
                raise HTTPException(status_code=500, detail="Error de transcripción")
    except Exception as e:
        print(f"Error in microphone capture: {e}")
        raise HTTPException(status_code=500, detail=f"Error al capturar micrófono: {e}")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=5005)
