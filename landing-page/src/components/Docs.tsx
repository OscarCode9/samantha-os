import { useState, useMemo, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

interface ToolParam {
  name: string;
  type: string;
  required: boolean;
  description: { en: string; es: string };
}

interface ToolExample {
  prompt: { en: string; es: string };
  arguments: string;
  response: string;
}

interface ToolData {
  name: string;
  category: string;
  description: { en: string; es: string };
  parameters: ToolParam[];
  example: ToolExample;
}

const CATEGORIES = [
  'Overview',
  'Quick Start',
  'Files & Directories',
  'System & OS Control',
  'Productivity Suite',
  'Tasks & Scheduling',
  'Smart Utilities'
];

const TOOLS_DATA: ToolData[] = [
  // 1. Files & Directories
  {
    name: 'read_file',
    category: 'Files & Directories',
    description: {
      en: 'Read the contents of a file. Returns numbered lines. Use offset and limit to read specific sections of large files.',
      es: 'Lee el contenido de un archivo. Devuelve líneas numeradas. Usa offset y limit para leer secciones específicas de archivos grandes.'
    },
    parameters: [
      { name: 'path', type: 'string', required: true, description: { en: 'Absolute or workspace-relative path to the file to read.', es: 'Ruta absoluta o relativa al espacio de trabajo del archivo a leer.' } },
      { name: 'limit', type: 'number', required: false, description: { en: 'Maximum number of lines to return. Defaults to 2000.', es: 'Número máximo de líneas a devolver. Por defecto 2000.' } },
      { name: 'offset', type: 'number', required: false, description: { en: '1-based line number to start reading from. Defaults to 1.', es: 'Línea de inicio basada en 1. Por defecto 1.' } }
    ],
    example: {
      prompt: { en: 'Read config.json from line 10', es: 'Lee el archivo config.json desde la línea 10' },
      arguments: '{\n  "path": "config.json",\n  "offset": 10,\n  "limit": 50\n}',
      response: '{\n  "content": "10:   \"port\": 4389,\\n11:   \"debug\": true\\n"\n}'
    }
  },
  {
    name: 'write_file',
    category: 'Files & Directories',
    description: {
      en: 'Write content to a file. Creates the file and any parent directories if they do not exist. Overwrites existing content.',
      es: 'Escribe contenido en un archivo. Crea el archivo y directorios padres si no existen. Sobrescribe el contenido existente.'
    },
    parameters: [
      { name: 'path', type: 'string', required: true, description: { en: 'Absolute or workspace-relative path to the file to write.', es: 'Ruta absoluta o relativa al espacio de trabajo del archivo a escribir.' } },
      { name: 'content', type: 'string', required: true, description: { en: 'The content to write to the file.', es: 'El contenido a escribir en el archivo.' } }
    ],
    example: {
      prompt: { en: 'Write "hello world" to saludo.txt', es: 'Escribe "hello world" en saludo.txt' },
      arguments: '{\n  "path": "saludo.txt",\n  "content": "hello world\\n"\n}',
      response: '{\n  "content": "Successfully wrote 12 bytes to saludo.txt"\n}'
    }
  },
  {
    name: 'edit_file',
    category: 'Files & Directories',
    description: {
      en: 'Edit a file by replacing an exact string with a new string. The old_string must match exactly (including whitespace and indentation). Use replace_all to replace every occurrence.',
      es: 'Edita un archivo reemplazando una cadena exacta por una nueva. La cadena original debe coincidir exactamente.'
    },
    parameters: [
      { name: 'path', type: 'string', required: true, description: { en: 'Absolute or workspace-relative path to the file to edit.', es: 'Ruta absoluta o relativa al espacio de trabajo del archivo a editar.' } },
      { name: 'old_string', type: 'string', required: true, description: { en: 'The exact string to find and replace. Must match exactly.', es: 'La cadena exacta a buscar y reemplazar.' } },
      { name: 'new_string', type: 'string', required: true, description: { en: 'The replacement string. Can be empty to delete.', es: 'La cadena de reemplazo. Puede estar vacía para borrar.' } },
      { name: 'replace_all', type: 'string', required: false, description: { en: 'Set to "true" to replace all occurrences. Defaults to only the first.', es: 'Establece a "true" para reemplazar todas las ocurrencias. Por defecto solo la primera.' } }
    ],
    example: {
      prompt: { en: 'Replace debug: false with debug: true in settings.yaml', es: 'Reemplaza debug: false con debug: true en settings.yaml' },
      arguments: '{\n  "path": "settings.yaml",\n  "old_string": "debug: false",\n  "new_string": "debug: true"\n}',
      response: '{\n  "content": "Successfully patched settings.yaml"\n}'
    }
  },
  {
    name: 'list_dir',
    category: 'Files & Directories',
    description: {
      en: 'List the contents of a directory. Returns one entry per line. Directories have a trailing /.',
      es: 'Lista el contenido de un directorio. Devuelve una entrada por línea. Los directorios terminan con /.'
    },
    parameters: [
      { name: 'path', type: 'string', required: true, description: { en: 'Absolute or workspace-relative path to the directory to list.', es: 'Ruta absoluta o relativa al espacio de trabajo del directorio.' } }
    ],
    example: {
      prompt: { en: 'Show what is in ~/Documents', es: 'Muestra lo que hay en ~/Documentos' },
      arguments: '{\n  "path": "~/Documentos"\n}',
      response: '[\n  "contrato.pdf",\n  "proyectos/",\n  "notas.txt"\n]'
    }
  },
  {
    name: 'glob',
    category: 'Files & Directories',
    description: {
      en: 'Find files matching a glob pattern. Supports patterns like "**/*.go" or "src/**/*.ts". Returns matching file paths.',
      es: 'Busca archivos que coincidan con un patrón glob. Soporta patrones como "**/*.go" o "src/**/*.ts". Devuelve las rutas.'
    },
    parameters: [
      { name: 'pattern', type: 'string', required: true, description: { en: 'Glob pattern to match files against (e.g. "**/*.go").', es: 'Patrón glob para emparejar archivos (ej. "**/*.go").' } },
      { name: 'path', type: 'string', required: false, description: { en: 'Base directory to search from. Defaults to workspace root.', es: 'Directorio base de búsqueda. Por defecto la raíz.' } }
    ],
    example: {
      prompt: { en: 'Find go files in internal folder', es: 'Busca archivos go en la carpeta internal' },
      arguments: '{\n  "pattern": "**/*.go",\n  "path": "internal"\n}',
      response: '[\n  "internal/app/app.go",\n  "internal/tools/registry.go"\n]'
    }
  },
  {
    name: 'grep_search',
    category: 'Files & Directories',
    description: {
      en: 'Search file contents using a regular expression pattern. Returns matching file paths and line numbers with context.',
      es: 'Busca patrones de texto usando expresiones regulares. Devuelve rutas de archivos, números de línea y contexto.'
    },
    parameters: [
      { name: 'pattern', type: 'string', required: true, description: { en: 'Regular expression pattern to search for.', es: 'Patrón de expresión regular a buscar.' } },
      { name: 'path', type: 'string', required: false, description: { en: 'Directory or file to search in. Defaults to workspace root.', es: 'Directorio o archivo a buscar. Por defecto la raíz.' } },
      { name: 'include', type: 'string', required: false, description: { en: 'Glob pattern to filter files (e.g. "*.go").', es: 'Patrón glob para filtrar archivos (ej. "*.go").' } }
    ],
    example: {
      prompt: { en: 'Search for "Registry" in Go files', es: 'Busca "Registry" en archivos Go' },
      arguments: '{\n  "pattern": "Registry",\n  "include": "*.go"\n}',
      response: '[\n  { "file": "internal/tools/registry.go", "line": 94, "text": "type Registry struct {" }\n]'
    }
  },
  {
    name: 'trash_file',
    category: 'Files & Directories',
    description: {
      en: 'Move a local file or folder to the desktop trash using gio trash. This is reversible from the file manager trash.',
      es: 'Mueve un archivo o carpeta local a la papelera del escritorio usando gio trash. Reversible desde la papelera de archivos.'
    },
    parameters: [
      { name: 'path', type: 'string', required: true, description: { en: 'Local file or folder path to move to trash.', es: 'Ruta de archivo o carpeta local a mover a la papelera.' } }
    ],
    example: {
      prompt: { en: 'Send delete.txt to the trash bin', es: 'Manda el archivo borrar.txt a la papelera' },
      arguments: '{\n  "path": "borrar.txt"\n}',
      response: '{\n  "content": "Moved borrar.txt to trash successfully"\n}'
    }
  },
  {
    name: 'open_folder',
    category: 'Files & Directories',
    description: {
      en: 'Open a folder in the Files app (elementary Files / Nautilus). Accepts a local path or a file:// URI.',
      es: 'Abre una carpeta en la aplicación Archivos (elementary Files / Nautilus). Acepta rutas locales o URIs file://.'
    },
    parameters: [
      { name: 'path', type: 'string', required: true, description: { en: 'Local folder path or file:// URI to open.', es: 'Ruta de carpeta local o URI file:// a abrir.' } }
    ],
    example: {
      prompt: { en: 'Open projects folder', es: 'Abre la carpeta de proyectos' },
      arguments: '{\n  "path": "~/Proyectos"\n}',
      response: '{\n  "content": "Opened folder ~/Proyectos in Files"\n}'
    }
  },

  // 2. System & OS Control
  {
    name: 'exec',
    category: 'System & OS Control',
    description: {
      en: 'Execute a shell command and return its output. Executed via /bin/sh -c.',
      es: 'Ejecuta un comando en la terminal y devuelve su salida. Ejecutado vía /bin/sh -c.'
    },
    parameters: [
      { name: 'command', type: 'string', required: true, description: { en: 'The shell command to execute.', es: 'El comando de terminal a ejecutar.' } },
      { name: 'workdir', type: 'string', required: false, description: { en: 'Working directory for the command. Defaults to workspace root.', es: 'Directorio de trabajo para el comando. Por defecto la raíz.' } },
      { name: 'timeout', type: 'number', required: false, description: { en: 'Timeout in seconds. Defaults to 30.', es: 'Tiempo de expiración en segundos. Por defecto 30.' } }
    ],
    example: {
      prompt: { en: 'Run uname -a', es: 'Ejecuta uname -a' },
      arguments: '{\n  "command": "uname -a"\n}',
      response: '{\n  "content": "Linux elementary-desktop 6.5.0-val-amd64 x86_64 GNU/Linux\\n"\n}'
    }
  },
  {
    name: 'audio_volume',
    category: 'System & OS Control',
    description: {
      en: 'Get or change the default output volume using PulseAudio/PipeWire. Supports get, set, mute, unmute, and toggle_mute.',
      es: 'Obtiene o cambia el volumen de salida predeterminado usando PulseAudio/PipeWire. Soporta get, set, mute, unmute y toggle_mute.'
    },
    parameters: [
      { name: 'action', type: 'string', required: false, description: { en: 'Volume action. Defaults to get.', es: 'Acción de volumen. Por defecto get.' } },
      { name: 'volume_percent', type: 'integer', required: false, description: { en: 'Target output volume percentage (0-150) for set.', es: 'Porcentaje de volumen objetivo (0-150) para la acción set.' } }
    ],
    example: {
      prompt: { en: 'Mute the main volume', es: 'Silencia el volumen principal' },
      arguments: '{\n  "action": "mute"\n}',
      response: '{\n  "action": "mute",\n  "ok": true\n}'
    }
  },
  {
    name: 'media_control',
    category: 'System & OS Control',
    description: {
      en: 'Control MPRIS media players like Spotify, Music, or browsers. Supports play, pause, play_pause, stop, next, previous, and status.',
      es: 'Controla reproductores de medios MPRIS como Spotify, Música o navegadores. Soporta controles básicos.'
    },
    parameters: [
      { name: 'action', type: 'string', required: true, description: { en: 'Playback action to run.', es: 'Acción de reproducción a ejecutar.' } },
      { name: 'player', type: 'string', required: false, description: { en: 'Optional player name or DBus service substring (e.g. spotify).', es: 'Nombre opcional de reproductor o subcadena de servicio DBus (ej. spotify).' } }
    ],
    example: {
      prompt: { en: 'Skip song in Spotify', es: 'Pasa a la siguiente canción en Spotify' },
      arguments: '{\n  "action": "next",\n  "player": "spotify"\n}',
      response: '{\n  "action": "next",\n  "ok": true\n}'
    }
  },
  {
    name: 'inhibit_sleep',
    category: 'System & OS Control',
    description: {
      en: 'Prevent the system from sleeping or idling for a limited time, useful during presentations or long tasks.',
      es: 'Evita que el sistema se suspenda o apague la pantalla por un tiempo limitado, útil en presentaciones.'
    },
    parameters: [
      { name: 'duration_minutes', type: 'integer', required: false, description: { en: 'How long to prevent sleep/idle (max 480).', es: 'Cuánto tiempo prevenir suspensión/inactividad (máx 480).' } },
      { name: 'mode', type: 'string', required: false, description: { en: 'What to inhibit. Enum: sleep, idle, sleep_and_idle', es: 'Qué prevenir. Enum: sleep, idle, sleep_and_idle' } },
      { name: 'reason', type: 'string', required: false, description: { en: 'Human-readable reason for inhibition.', es: 'Razón de la inhibición para mostrar en sistema.' } }
    ],
    example: {
      prompt: { en: 'Prevent screen off for 30 minutes', es: 'No dejes que se apague la pantalla por 30 minutos' },
      arguments: '{\n  "duration_minutes": 30,\n  "mode": "idle",\n  "reason": "Viendo un tutorial largo"\n}',
      response: '{\n  "inhibited": true,\n  "duration": 30\n}'
    }
  },
  {
    name: 'concentration_mode',
    category: 'System & OS Control',
    description: {
      en: 'Enable or disable system concentration mode on elementary OS. Enabling DND and preventing idle/sleep.',
      es: 'Activa o desactiva el modo concentración en elementary OS, encendiendo No Molestar y previniendo suspensión.'
    },
    parameters: [
      { name: 'enabled', type: 'boolean', required: false, description: { en: 'Whether to enable concentration mode.', es: 'Si se activa o desactiva el modo concentración.' } },
      { name: 'duration_minutes', type: 'integer', required: false, description: { en: 'Duration to prevent sleep while active.', es: 'Duración para prevenir suspensión mientras esté activo.' } },
      { name: 'reason', type: 'string', required: false, description: { en: 'Reason shown on system inhibition.', es: 'Razón mostrada en la inhibición de suspensión.' } }
    ],
    example: {
      prompt: { en: 'Put me in focus mode for an hour', es: 'Ponme en modo focus durante una hora' },
      arguments: '{\n  "enabled": true,\n  "duration_minutes": 60,\n  "reason": "Focus session"\n}',
      response: '{\n  "concentrationMode": true,\n  "sleepInhibited": true\n}'
    }
  },
  {
    name: 'get_battery_status',
    category: 'System & OS Control',
    description: {
      en: 'Get the current battery status of the device: charge percentage, state, and time remaining.',
      es: 'Obtiene el estado actual de la batería: porcentaje, estado de carga y tiempo restante.'
    },
    parameters: [],
    example: {
      prompt: { en: 'Check battery status', es: '¿Cómo va mi batería?' },
      arguments: '{}',
      response: '{\n  "percentage": 82,\n  "state": "discharging",\n  "timeToEmpty": "5h 12m"\n}'
    }
  },
  {
    name: 'get_network_status',
    category: 'System & OS Control',
    description: {
      en: 'Get the current network connectivity status: connected, type, SSID, and IP address.',
      es: 'Obtiene el estado de conexión de red actual: conexión, tipo, SSID e IP.'
    },
    parameters: [],
    example: {
      prompt: { en: 'Am I connected to internet?', es: '¿Estoy conectado a internet?' },
      arguments: '{}',
      response: '{\n  "connected": true,\n  "type": "wifi",\n  "ssid": "MiRed_5G",\n  "ipAddress": "192.168.1.55"\n}'
    }
  },
  {
    name: 'list_wifi_networks',
    category: 'System & OS Control',
    description: {
      en: 'List visible Wi-Fi networks through NetworkManager, including SSID, signal, and security details.',
      es: 'Lista redes Wi-Fi visibles usando NetworkManager, con SSID, señal y seguridad.'
    },
    parameters: [
      { name: 'rescan', type: 'boolean', required: false, description: { en: 'Rescan access points before listing.', es: 'Escanear puntos de acceso antes de listar.' } }
    ],
    example: {
      prompt: { en: 'Scan for Wi-Fi networks', es: 'Escanea redes inalámbricas' },
      arguments: '{\n  "rescan": true\n}',
      response: '[\n  { "ssid": "MiRed_5G", "signal": 98, "active": true },\n  { "ssid": "Vecino_WiFi", "signal": 45, "active": false }\n]'
    }
  },
  {
    name: 'connect_wifi',
    category: 'System & OS Control',
    description: {
      en: 'Connect to a Wi-Fi network through NetworkManager.',
      es: 'Conéctate a una red Wi-Fi mediante NetworkManager.'
    },
    parameters: [
      { name: 'ssid', type: 'string', required: true, description: { en: 'Wi-Fi SSID to connect to.', es: 'SSID de red Wi-Fi a conectar.' } },
      { name: 'password', type: 'string', required: false, description: { en: 'Optional Wi-Fi password.', es: 'Contraseña opcional de red Wi-Fi.' } },
      { name: 'interface', type: 'string', required: false, description: { en: 'Optional interface name (e.g. wlan0).', es: 'Interfaz opcional de red (ej. wlan0).' } },
      { name: 'rescan', type: 'boolean', required: false, description: { en: 'Rescan before connecting.', es: 'Escanear antes de conectar.' } }
    ],
    example: {
      prompt: { en: 'Connect to Guest Wi-Fi with password 12345678', es: 'Conéctame al WiFi Invitados con clave 12345678' },
      arguments: '{\n  "ssid": "Invitados",\n  "password": "12345678"\n}',
      response: '{\n  "connected": true,\n  "ssid": "Invitados"\n}'
    }
  },
  {
    name: 'bluetooth_device',
    category: 'System & OS Control',
    description: {
      en: 'Control Bluetooth through BlueZ: list devices, power, scan, connect, and disconnect.',
      es: 'Controla Bluetooth mediante BlueZ: listar dispositivos, encendido, escanear, conectar y desconectar.'
    },
    parameters: [
      { name: 'action', type: 'string', required: true, description: { en: 'Bluetooth command action.', es: 'Acción de comando Bluetooth.' } },
      { name: 'target', type: 'string', required: false, description: { en: 'Device MAC address or name substring.', es: 'Dirección MAC del dispositivo o subcadena de nombre.' } }
    ],
    example: {
      prompt: { en: 'Turn bluetooth off', es: 'Apaga el bluetooth' },
      arguments: '{\n  "action": "power_off"\n}',
      response: '{\n  "ok": true,\n  "powered": false\n}'
    }
  },
  {
    name: 'get_active_window',
    category: 'System & OS Control',
    description: {
      en: 'Get the currently focused window/app: title, app name, app id, and PID.',
      es: 'Obtiene la ventana enfocada: título, nombre de app, ID de app y PID.'
    },
    parameters: [],
    example: {
      prompt: { en: 'What window is currently open?', es: '¿Qué ventana está activa?' },
      arguments: '{}',
      response: '{\n  "appName": "Vite Dev Server",\n  "title": "Samantha OS Documentation - Chromium",\n  "pid": 45192\n}'
    }
  },
  {
    name: 'get_current_user',
    category: 'System & OS Control',
    description: {
      en: 'Get information about the currently logged-in user: username, display name, and home.',
      es: 'Obtiene datos del usuario activo: nombre de usuario, nombre visible e inicio.'
    },
    parameters: [],
    example: {
      prompt: { en: 'Who is logged in?', es: 'Dime mis datos de usuario local' },
      arguments: '{}',
      response: '{\n  "username": "oscarcode",\n  "displayName": "Oscar",\n  "home": "/home/oscarcode"\n}'
    }
  },
  {
    name: 'developer_mode',
    category: 'System & OS Control',
    description: {
      en: 'Open a developer workspace for the user. Opens browser, terminal, editor, and optionally Files.',
      es: 'Abre un espacio de trabajo de desarrollo: navegador, terminal, editor y opcionalmente archivos.'
    },
    parameters: [
      { name: 'path', type: 'string', required: true, description: { en: 'Project folder or file path.', es: 'Carpeta de proyecto o ruta de archivo.' } },
      { name: 'open_browser', type: 'boolean', required: false, description: { en: 'Open default browser.', es: 'Abrir navegador por defecto.' } },
      { name: 'open_editor', type: 'boolean', required: false, description: { en: 'Open editor in path.', es: 'Abrir editor en la ruta.' } },
      { name: 'open_folder', type: 'boolean', required: false, description: { en: 'Open path in Files app.', es: 'Abrir ruta en gestor de archivos.' } },
      { name: 'open_terminal', type: 'boolean', required: false, description: { en: 'Open terminal in path.', es: 'Abrir terminal en la ruta.' } },
      { name: 'url', type: 'string', required: false, description: { en: 'Local URL to open in browser.', es: 'URL local a abrir en navegador.' } }
    ],
    example: {
      prompt: { en: 'Open developer workspace for landing-page', es: 'Abre el entorno de desarrollo para el proyecto landing-page' },
      arguments: '{\n  "path": "landing-page",\n  "url": "http://localhost:5174"\n}',
      response: '{\n  "editorOpened": true,\n  "terminalOpened": true,\n  "browserOpened": true\n}'
    }
  },
  {
    name: 'clean_cache',
    category: 'System & OS Control',
    description: {
      en: 'Analyze cache usage and clean up selected paths after user confirmation.',
      es: 'Analiza el espacio de caché del sistema y elimina rutas seleccionadas tras confirmar.'
    },
    parameters: [
      { name: 'action', type: 'string', required: false, description: { en: 'Action to perform: analyze or delete.', es: 'Acción a realizar: analyze o delete.' } },
      { name: 'confirm', type: 'boolean', required: false, description: { en: 'Required true before deleting.', es: 'Requerido true antes de borrar.' } },
      { name: 'limit', type: 'integer', required: false, description: { en: 'Max cache entries to return.', es: 'Límite de entradas a retornar.' } },
      { name: 'mode', type: 'string', required: false, description: { en: 'Cleanup mode: standard or super_clean.', es: 'Modo de limpieza: standard o super_clean.' } },
      { name: 'paths', type: 'array', required: false, description: { en: 'Paths to clear during delete.', es: 'Rutas a limpiar durante el delete.' } }
    ],
    example: {
      prompt: { en: 'Analyze system cache directories', es: 'Analiza el espacio en cache de mi sistema' },
      arguments: '{\n  "action": "analyze"\n}',
      response: '[\n  { "path": "~/.cache/thumbnails", "sizeBytes": 420912 },\n  { "path": "~/.cache/flatpak", "sizeBytes": 204910398 }\n]'
    }
  },

  // 3. Productivity Suite
  {
    name: 'connect_gmail',
    category: 'Productivity Suite',
    description: {
      en: 'Starts Google OAuth flow to grant Gmail access to Samantha.',
      es: 'Inicia el flujo de autenticación OAuth para dar acceso de Gmail a Samantha.'
    },
    parameters: [
      { name: 'client_id', type: 'string', required: false, description: { en: 'Client ID from Google Console.', es: 'Client ID de Google Cloud Console.' } },
      { name: 'client_secret', type: 'string', required: false, description: { en: 'Client Secret from Google Console.', es: 'Client Secret de Google Console.' } },
      { name: 'credentials_json', type: 'string', required: false, description: { en: 'Google credentials JSON content.', es: 'Contenido JSON de credenciales de Google.' } },
      { name: 'credentials_path', type: 'string', required: false, description: { en: 'Path to credentials JSON.', es: 'Ruta al archivo JSON de credenciales.' } }
    ],
    example: {
      prompt: { en: 'Connect my Gmail account using credentials.json', es: 'Conecta mi cuenta de Gmail usando el json credentials.json' },
      arguments: '{\n  "credentials_path": "credentials.json"\n}',
      response: '{\n  "ok": true,\n  "auth_url": "https://accounts.google.com/o/oauth2/v2/auth?..."\n}'
    }
  },
  {
    name: 'list_emails',
    category: 'Productivity Suite',
    description: {
      en: 'List recent emails from Gmail. Returns ID, subject, sender, and snippet.',
      es: 'Lista emails recientes de Gmail. Devuelve ID, asunto, remitente y fragmento.'
    },
    parameters: [
      { name: 'label', type: 'string', required: false, description: { en: 'Gmail label filter (default INBOX).', es: 'Etiqueta de Gmail (por defecto INBOX).' } },
      { name: 'max_results', type: 'integer', required: false, description: { en: 'Max emails to return (1-50).', es: 'Máximo de correos a retornar (1-50).' } },
      { name: 'unread_only', type: 'boolean', required: false, description: { en: 'Return unread only.', es: 'Retornar solo no leídos.' } }
    ],
    example: {
      prompt: { en: 'List my unread emails', es: 'Muestra mis correos no leídos' },
      arguments: '{\n  "unread_only": true,\n  "max_results": 5\n}',
      response: '[\n  { "message_id": "18f9b1", "subject": "Código de verificación", "from": "noreply@github.com", "snippet": "Tu código de acceso es..." }\n]'
    }
  },
  {
    name: 'read_email',
    category: 'Productivity Suite',
    description: {
      en: 'Read the full content of a Gmail message by ID.',
      es: 'Lee el contenido completo de un email de Gmail por su ID.'
    },
    parameters: [
      { name: 'message_id', type: 'string', required: true, description: { en: 'Gmail message ID to read.', es: 'ID del mensaje de Gmail a leer.' } }
    ],
    example: {
      prompt: { en: 'Read email 18f9b1', es: 'Lee el email 18f9b1' },
      arguments: '{\n  "message_id": "18f9b1"\n}',
      response: '{\n  "subject": "Código de verificación",\n  "from": "noreply@github.com",\n  "body": "Hola, para terminar de vincular tu cuenta usa el código: 928491."\n}'
    }
  },
  {
    name: 'send_email',
    category: 'Productivity Suite',
    description: {
      en: 'Send an email via Gmail on behalf of the user.',
      es: 'Envía un correo mediante la cuenta de Gmail vinculada.'
    },
    parameters: [
      { name: 'to', type: 'string', required: true, description: { en: 'Recipient email address.', es: 'Dirección del destinatario.' } },
      { name: 'subject', type: 'string', required: true, description: { en: 'Email subject line.', es: 'Asunto del correo.' } },
      { name: 'body', type: 'string', required: true, description: { en: 'Plain-text email body.', es: 'Cuerpo de texto plano del correo.' } },
      { name: 'cc', type: 'string', required: false, description: { en: 'Optional CC recipient.', es: 'Destinatario CC opcional.' } },
      { name: 'reply_to_message_id', type: 'string', required: false, description: { en: 'Message ID to reply to.', es: 'ID de mensaje opcional a responder.' } }
    ],
    example: {
      prompt: { en: 'Send email to maria@company.com with subject Report', es: 'Escríbele a maria@company.com con asunto Reporte diciendo Adjunto informe' },
      arguments: '{\n  "to": "maria@company.com",\n  "subject": "Reporte",\n  "body": "Hola María, adjunto el informe solicitado. Saludos."\n}',
      response: '{\n  "ok": true,\n  "message_id": "msg_982491a"\n}'
    }
  },
  {
    name: 'search_emails',
    category: 'Productivity Suite',
    description: {
      en: 'Search Gmail using standard Gmail query filters.',
      es: 'Busca correos en Gmail usando filtros de búsqueda estándar.'
    },
    parameters: [
      { name: 'query', type: 'string', required: true, description: { en: 'Gmail query string (e.g. from:boss).', es: 'Filtro de búsqueda de Gmail (ej. from:jefe).' } },
      { name: 'max_results', type: 'integer', required: false, description: { en: 'Max emails to return.', es: 'Máximo de correos a retornar.' } }
    ],
    example: {
      prompt: { en: 'Find emails from billing with attachments', es: 'Busca correos sobre facturas que tengan archivos adjuntos' },
      arguments: '{\n  "query": "subject:factura has:attachment"\n}',
      response: '[\n  { "message_id": "92f89", "subject": "Factura de Compra", "from": "billing@vendor.com" }\n]'
    }
  },
  {
    name: 'list_calendar_events',
    category: 'Productivity Suite',
    description: {
      en: 'List events currently stored in elementary Calendar.',
      es: 'Lista eventos almacenados en el Calendario de elementary OS.'
    },
    parameters: [
      { name: 'summary_contains', type: 'string', required: false, description: { en: 'Filter events containing text.', es: 'Filtra eventos que contengan el texto.' } }
    ],
    example: {
      prompt: { en: 'Find events containing "Dentist"', es: 'Busca eventos que contengan "Dentista"' },
      arguments: '{\n  "summary_contains": "Dentista"\n}',
      response: '[\n  { "uid": "cal_11", "title": "Cita Dentista", "start": "2026-06-15T10:00:00-06:00" }\n]'
    }
  },
  {
    name: 'create_calendar_event',
    category: 'Productivity Suite',
    description: {
      en: 'Create a new event in elementary Calendar.',
      es: 'Crea un nuevo evento en el Calendario de elementary OS.'
    },
    parameters: [
      { name: 'title', type: 'string', required: true, description: { en: 'Event title.', es: 'Título del evento.' } },
      { name: 'start', type: 'string', required: true, description: { en: 'Start RFC3339 datetime.', es: 'Fecha y hora de inicio en formato RFC3339.' } },
      { name: 'duration_minutes', type: 'integer', required: false, description: { en: 'Duration in minutes.', es: 'Duración en minutos.' } },
      { name: 'description', type: 'string', required: false, description: { en: 'Event description.', es: 'Descripción del evento.' } },
      { name: 'location', type: 'string', required: false, description: { en: 'Event location.', es: 'Ubicación del evento.' } },
      { name: 'timezone', type: 'string', required: false, description: { en: 'IANA timezone name.', es: 'Zona horaria IANA.' } },
      { name: 'daily', type: 'boolean', required: false, description: { en: 'Create daily recurring event.', es: 'Crear evento recurrente diario.' } }
    ],
    example: {
      prompt: { en: 'Schedule meeting tomorrow at 11:30 am', es: 'Programa una videollamada mañana a las 11:30 am' },
      arguments: '{\n  "title": "Videollamada",\n  "start": "2026-06-12T11:30:00-06:00",\n  "duration_minutes": 45\n}',
      response: '{\n  "ok": true,\n  "uid": "cal_99a823b"\n}'
    }
  },
  {
    name: 'modify_calendar_event',
    category: 'Productivity Suite',
    description: {
      en: 'Modify an existing elementary Calendar event by UID.',
      es: 'Modifica un evento de Calendario existente usando su UID.'
    },
    parameters: [
      { name: 'uid', type: 'string', required: true, description: { en: 'Calendar event UID.', es: 'UID del evento de Calendario a modificar.' } },
      { name: 'title', type: 'string', required: false, description: { en: 'New title.', es: 'Nuevo título.' } },
      { name: 'start', type: 'string', required: false, description: { en: 'New start datetime.', es: 'Nueva fecha y hora de inicio.' } },
      { name: 'duration_minutes', type: 'integer', required: false, description: { en: 'New duration.', es: 'Nueva duración en minutos.' } },
      { name: 'description', type: 'string', required: false, description: { en: 'New description.', es: 'Nueva descripción.' } },
      { name: 'location', type: 'string', required: false, description: { en: 'New location.', es: 'Nueva ubicación.' } },
      { name: 'timezone', type: 'string', required: false, description: { en: 'New timezone.', es: 'Nueva zona horaria.' } },
      { name: 'daily', type: 'boolean', required: false, description: { en: 'Change daily recurrence.', es: 'Modificar recurrencia diaria.' } }
    ],
    example: {
      prompt: { en: 'Move videollamada to 12:00 pm', es: 'Pasa la videollamada con UID cal_99a823b a las 12:00 pm' },
      arguments: '{\n  "uid": "cal_99a823b",\n  "start": "2026-06-12T12:00:00-06:00"\n}',
      response: '{\n  "ok": true\n}'
    }
  },
  {
    name: 'list_contacts',
    category: 'Productivity Suite',
    description: {
      en: 'List contacts from the personal Evolution Data Server address book.',
      es: 'Lista los contactos de la libreta de direcciones personal de Evolution Data Server.'
    },
    parameters: [
      { name: 'include_vcard', type: 'boolean', required: false, description: { en: 'Include raw vCard payload.', es: 'Incluir contenido vCard completo.' } },
      { name: 'limit', type: 'integer', required: false, description: { en: 'Limit result count.', es: 'Límite de contactos a retornar.' } }
    ],
    example: {
      prompt: { en: 'List my contacts', es: 'Lista mis contactos' },
      arguments: '{\n  "limit": 10\n}',
      response: '[\n  { "uid": "cont_1", "fullName": "Sofía López", "emails": ["sofia@company.com"], "phones": ["555-9824"] }\n]'
    }
  },
  {
    name: 'search_contacts',
    category: 'Productivity Suite',
    description: {
      en: 'Search contacts in the Address Book by name or email.',
      es: 'Busca contactos en la libreta de direcciones por nombre o email.'
    },
    parameters: [
      { name: 'query', type: 'string', required: true, description: { en: 'Name or email fraction to find.', es: 'Fragmento de nombre o correo a buscar.' } },
      { name: 'include_vcard', type: 'boolean', required: false, description: { en: 'Include raw vCard.', es: 'Incluir vCard completo.' } },
      { name: 'limit', type: 'integer', required: false, description: { en: 'Limit result count.', es: 'Límite de resultados.' } }
    ],
    example: {
      prompt: { en: 'Search contact Sofia', es: 'Busca el contacto de Sofía' },
      arguments: '{\n  "query": "Sofía"\n}',
      response: '[\n  { "uid": "cont_1", "fullName": "Sofía López", "emails": ["sofia@company.com"] }\n]'
    }
  },
  {
    name: 'create_contact',
    category: 'Productivity Suite',
    description: {
      en: 'Create a contact in the Evolution Data Server address book.',
      es: 'Crea un contacto en la libreta de direcciones personal.'
    },
    parameters: [
      { name: 'full_name', type: 'string', required: false, description: { en: 'Display name.', es: 'Nombre a mostrar del contacto.' } },
      { name: 'given_name', type: 'string', required: false, description: { en: 'Given name.', es: 'Primer nombre.' } },
      { name: 'family_name', type: 'string', required: false, description: { en: 'Family name.', es: 'Apellido.' } },
      { name: 'nickname', type: 'string', required: false, description: { en: 'Nickname.', es: 'Apodo.' } },
      { name: 'note', type: 'string', required: false, description: { en: 'Notes.', es: 'Notas.' } },
      { name: 'organization', type: 'string', required: false, description: { en: 'Company name.', es: 'Nombre de compañía.' } },
      { name: 'title', type: 'string', required: false, description: { en: 'Job title.', es: 'Puesto de trabajo.' } },
      { name: 'email', type: 'string', required: false, description: { en: 'Primary email.', es: 'Email principal.' } },
      { name: 'emails', type: 'array', required: false, description: { en: 'Multiple emails.', es: 'Lista de correos.' } },
      { name: 'phone', type: 'string', required: false, description: { en: 'Primary phone.', es: 'Teléfono principal.' } },
      { name: 'phones', type: 'array', required: false, description: { en: 'Multiple phones.', es: 'Lista de teléfonos.' } },
      { name: 'raw_vcard', type: 'string', required: false, description: { en: 'Raw vCard data.', es: 'Datos raw vCard.' } }
    ],
    example: {
      prompt: { en: 'Add Pedro to contacts with email pedro@mail.com', es: 'Guarda el contacto de Pedro con correo pedro@mail.com' },
      arguments: '{\n  "full_name": "Pedro",\n  "email": "pedro@mail.com"\n}',
      response: '{\n  "ok": true,\n  "uid": "cont_99a8b1"\n}'
    }
  },
  {
    name: 'modify_contact',
    category: 'Productivity Suite',
    description: {
      en: 'Modify fields of an existing contact by UID.',
      es: 'Modifica los campos de un contacto existente usando su UID.'
    },
    parameters: [
      { name: 'uid', type: 'string', required: true, description: { en: 'Contact UID to edit.', es: 'UID del contacto a modificar.' } },
      { name: 'full_name', type: 'string', required: false, description: { en: 'New full name.', es: 'Nuevo nombre completo.' } },
      { name: 'given_name', type: 'string', required: false, description: { en: 'New given name.', es: 'Nuevo primer nombre.' } },
      { name: 'family_name', type: 'string', required: false, description: { en: 'New family name.', es: 'Nuevo apellido.' } },
      { name: 'nickname', type: 'string', required: false, description: { en: 'New nickname.', es: 'Nuevo apodo.' } },
      { name: 'note', type: 'string', required: false, description: { en: 'New note.', es: 'Nueva nota.' } },
      { name: 'organization', type: 'string', required: false, description: { en: 'New company.', es: 'Nueva compañía.' } },
      { name: 'title', type: 'string', required: false, description: { en: 'New job title.', es: 'Nuevo puesto.' } },
      { name: 'email', type: 'string', required: false, description: { en: 'New primary email.', es: 'Nuevo email principal.' } },
      { name: 'emails', type: 'array', required: false, description: { en: 'New email set.', es: 'Nuevos emails.' } },
      { name: 'phone', type: 'string', required: false, description: { en: 'New primary phone.', es: 'Nuevo teléfono principal.' } },
      { name: 'phones', type: 'array', required: false, description: { en: 'New phone set.', es: 'Nuevos teléfonos.' } },
      { name: 'raw_vcard', type: 'string', required: false, description: { en: 'New raw vCard.', es: 'Nueva vCard.' } }
    ],
    example: {
      prompt: { en: 'Update Pedros phone to 555-0199', es: 'Actualiza el teléfono de Pedro (UID cont_99a8b1) a 555-0199' },
      arguments: '{\n  "uid": "cont_99a8b1",\n  "phone": "555-0199"\n}',
      response: '{\n  "ok": true\n}'
    }
  },
  {
    name: 'delete_contact',
    category: 'Productivity Suite',
    description: {
      en: 'Delete a contact from the address book by UID.',
      es: 'Elimina un contacto de la libreta de direcciones por su UID.'
    },
    parameters: [
      { name: 'uid', type: 'string', required: true, description: { en: 'Contact UID to delete.', es: 'UID del contacto a eliminar.' } }
    ],
    example: {
      prompt: { en: 'Delete contact with UID cont_99a8b1', es: 'Borra el contacto con UID cont_99a8b1' },
      arguments: '{\n  "uid": "cont_99a8b1"\n}',
      response: '{\n  "ok": true\n}'
    }
  },

  // 4. Tasks & Scheduling
  {
    name: 'list_tasks',
    category: 'Tasks & Scheduling',
    description: {
      en: 'List tasks currently stored in elementary Tasks.',
      es: 'Lista las tareas guardadas en la aplicación de Tareas de elementary OS.'
    },
    parameters: [
      { name: 'include_completed', type: 'boolean', required: false, description: { en: 'Include completed checklist tasks.', es: 'Incluir tareas ya completadas.' } },
      { name: 'summary_contains', type: 'string', required: false, description: { en: 'Filter tasks containing text.', es: 'Filtrar tareas que contengan el texto.' } }
    ],
    example: {
      prompt: { en: 'Show my pending tasks', es: 'Muéstrame mis tareas pendientes' },
      arguments: '{\n  "include_completed": false\n}',
      response: '[\n  { "uid": "task_2", "title": "Revisar pull request", "completed": false }\n]'
    }
  },
  {
    name: 'create_task',
    category: 'Tasks & Scheduling',
    description: {
      en: 'Create a task in elementary Tasks (EDS list).',
      es: 'Crea una tarea en elementary Tasks (lista de Evolution Data Server).'
    },
    parameters: [
      { name: 'title', type: 'string', required: true, description: { en: 'Task title.', es: 'Título de la tarea.' } },
      { name: 'description', type: 'string', required: false, description: { en: 'Optional description.', es: 'Descripción opcional de la tarea.' } },
      { name: 'due', type: 'string', required: false, description: { en: 'Optional RFC3339 due datetime.', es: 'Fecha y hora límite opcional en formato RFC3339.' } },
      { name: 'timezone', type: 'string', required: false, description: { en: 'Optional IANA timezone.', es: 'Zona horaria IANA opcional.' } }
    ],
    example: {
      prompt: { en: 'Add task Send invoice before Friday', es: 'Agrega una tarea llamada Enviar cotización antes del viernes' },
      arguments: '{\n  "title": "Enviar cotización",\n  "due": "2026-06-12T18:00:00-06:00"\n}',
      response: '{\n  "ok": true,\n  "uid": "task_a92b"\n}'
    }
  },
  {
    name: 'complete_task',
    category: 'Tasks & Scheduling',
    description: {
      en: 'Mark a task in elementary Tasks as completed (or reopen it).',
      es: 'Marca una tarea en elementary Tasks como completada (o vuélvela a abrir).'
    },
    parameters: [
      { name: 'uid', type: 'string', required: true, description: { en: 'Task UID.', es: 'UID de la tarea.' } },
      { name: 'completed', type: 'boolean', required: false, description: { en: 'Mark as completed (false to reopen).', es: 'Marcar completado (false para volver a abrir).' } }
    ],
    example: {
      prompt: { en: 'Mark task task_a92b as completed', es: 'Completa la tarea con UID task_a92b' },
      arguments: '{\n  "uid": "task_a92b",\n  "completed": true\n}',
      response: '{\n  "ok": true\n}'
    }
  },
  {
    name: 'delete_task',
    category: 'Tasks & Scheduling',
    description: {
      en: 'Delete a task from elementary Tasks by UID.',
      es: 'Elimina una tarea de elementary Tasks por su UID.'
    },
    parameters: [
      { name: 'uid', type: 'string', required: true, description: { en: 'Task UID to delete.', es: 'UID de la tarea a eliminar.' } }
    ],
    example: {
      prompt: { en: 'Delete task task_a92b', es: 'Elimina la tarea task_a92b' },
      arguments: '{\n  "uid": "task_a92b"\n}',
      response: '{\n  "ok": true\n}'
    }
  },
  {
    name: 'list_cron_jobs',
    category: 'Tasks & Scheduling',
    description: {
      en: 'List scheduled cron jobs configured in Samantha OS.',
      es: 'Lista tareas automatizadas (cron) configuradas en Samantha OS.'
    },
    parameters: [
      { name: 'enabled_only', type: 'boolean', required: false, description: { en: 'Return only enabled jobs.', es: 'Retornar solo tareas activadas.' } }
    ],
    example: {
      prompt: { en: 'Show my configured daily notifications', es: 'Muestra mis recordatorios automáticos configurados' },
      arguments: '{\n  "enabled_only": true\n}',
      response: '[\n  { "job_id": "job_1", "name": "Reporte diario", "time": "09:00", "message": "Recuerda enviar el reporte." }\n]'
    }
  },
  {
    name: 'create_daily_notification_job',
    category: 'Tasks & Scheduling',
    description: {
      en: 'Create a daily cron job that sends a desktop system notification.',
      es: 'Crea una tarea programada diaria para enviar una notificación de escritorio.'
    },
    parameters: [
      { name: 'message', type: 'string', required: true, description: { en: 'Notification body text.', es: 'Texto del cuerpo de la notificación.' } },
      { name: 'name', type: 'string', required: false, description: { en: 'Job display name.', es: 'Nombre visible de la tarea.' } },
      { name: 'time', type: 'string', required: false, description: { en: 'Daily time in HH:MM.', es: 'Hora de ejecución diaria en formato HH:MM.' } },
      { name: 'timezone', type: 'string', required: false, description: { en: 'IANA timezone.', es: 'Zona horaria IANA.' } },
      { name: 'title', type: 'string', required: false, description: { en: 'Notification title.', es: 'Título de la notificación.' } },
      { name: 'urgency', type: 'string', required: false, description: { en: 'Urgency: low, normal, critical.', es: 'Urgencia de la notificación: low, normal, critical.' } }
    ],
    example: {
      prompt: { en: 'Remind me to stretch every day at 4 pm', es: 'Recuérdame estirarme todos los días a las 4 pm con urgencia normal' },
      arguments: '{\n  "message": "¡Hora de estirarse!",\n  "time": "16:00",\n  "title": "Salud Física",\n  "urgency": "normal"\n}',
      response: '{\n  "ok": true,\n  "jobId": "job_2910"\n}'
    }
  },
  {
    name: 'create_daily_notification_job_with_calendar',
    category: 'Tasks & Scheduling',
    description: {
      en: 'Create a daily reminder cron job and mirror it in elementary Calendar.',
      es: 'Crea un recordatorio programado diario y agrégalo en el Calendario.'
    },
    parameters: [
      { name: 'message', type: 'string', required: true, description: { en: 'Notification body.', es: 'Cuerpo de la notificación.' } },
      { name: 'name', type: 'string', required: false, description: { en: 'Cron job name.', es: 'Nombre de la tarea automática.' } },
      { name: 'time', type: 'string', required: false, description: { en: 'Time in HH:MM.', es: 'Hora en formato HH:MM.' } },
      { name: 'timezone', type: 'string', required: false, description: { en: 'IANA timezone.', es: 'Zona horaria IANA.' } },
      { name: 'title', type: 'string', required: false, description: { en: 'Notification title.', es: 'Título de la notificación.' } },
      { name: 'urgency', type: 'string', required: false, description: { en: 'Urgency level.', es: 'Nivel de urgencia.' } }
    ],
    example: {
      prompt: { en: 'Schedule daily alarm at 8 am Take medicine with calendar syncing', es: 'Programa una alarme diaria a las 8 am diciendo Tomar medicina que esté en el calendario' },
      arguments: '{\n  "message": "Tomar medicina",\n  "time": "08:00",\n  "name": "Medicina diaria"\n}',
      response: '{\n  "ok": true,\n  "jobId": "job_med_8",\n  "calendarUid": "cal_med_8"\n}'
    }
  },
  {
    name: 'update_cron_job',
    category: 'Tasks & Scheduling',
    description: {
      en: 'Update an existing Samantha cron job details or enabled state.',
      es: 'Actualiza los detalles o estado de activación de una tarea programada Samantha.'
    },
    parameters: [
      { name: 'job_id', type: 'string', required: true, description: { en: 'Cron job ID to update.', es: 'ID de la tarea programada a modificar.' } },
      { name: 'name', type: 'string', required: false, description: { en: 'New job name.', es: 'Nuevo nombre de tarea.' } },
      { name: 'time', type: 'string', required: false, description: { en: 'New time HH:MM.', es: 'Nueva hora en HH:MM.' } },
      { name: 'timezone', type: 'string', required: false, description: { en: 'New timezone.', es: 'Nueva zona horaria.' } },
      { name: 'message', type: 'string', required: false, description: { en: 'New notification message.', es: 'Nuevo mensaje de la notificación.' } },
      { name: 'title', type: 'string', required: false, description: { en: 'New title.', es: 'Nuevo título.' } },
      { name: 'urgency', type: 'string', required: false, description: { en: 'New urgency.', es: 'Nueva urgencia.' } },
      { name: 'enabled', type: 'boolean', required: false, description: { en: 'Enable or disable job.', es: 'Activar o desactivar tarea.' } }
    ],
    example: {
      prompt: { en: 'Disable automatic task job_med_8', es: 'Desactiva la tarea automática con ID job_med_8' },
      arguments: '{\n  "job_id": "job_med_8",\n  "enabled": false\n}',
      response: '{\n  "ok": true\n}'
    }
  },
  {
    name: 'delete_cron_job',
    category: 'Tasks & Scheduling',
    description: {
      en: 'Delete an existing Samantha cron job by ID.',
      es: 'Elimina una tarea programada Samantha existente por su ID.'
    },
    parameters: [
      { name: 'job_id', type: 'string', required: true, description: { en: 'ID of the cron job to delete.', es: 'ID de la tarea programada a borrar.' } }
    ],
    example: {
      prompt: { en: 'Delete alarm job_med_8', es: 'Borra el recordatorio automático job_med_8' },
      arguments: '{\n  "job_id": "job_med_8"\n}',
      response: '{\n  "ok": true\n}'
    }
  },
  {
    name: 'delete_all_jobs_and_events',
    category: 'Tasks & Scheduling',
    description: {
      en: 'Delete all cron jobs and their linked calendar events created by Samantha.',
      es: 'Elimina todas las tareas automáticas y sus eventos del calendario creados por Samantha.'
    },
    parameters: [
      { name: 'delete_all_calendar_events', type: 'boolean', required: false, description: { en: 'Delete ALL events in calendar (destructive).', es: 'Eliminar TODOS los eventos del calendario (destructivo).' } },
      { name: 'delete_all_calendar_sam_events', type: 'boolean', required: false, description: { en: 'Delete all events starting with "sam-".', es: 'Eliminar todos los eventos que inicien con "sam-".' } }
    ],
    example: {
      prompt: { en: 'Clear all reminders created by Samantha', es: 'Limpia todos los recordatorios creados por Sam' },
      arguments: '{\n  "delete_all_calendar_sam_events": true\n}',
      response: '{\n  "ok": true,\n  "jobsDeleted": 4,\n  "eventsDeleted": 4\n}'
    }
  },

  // 5. Smart Utilities
  {
    name: 'pdf_text',
    category: 'Smart Utilities',
    description: {
      en: 'Extract text content from a PDF file. Returns the first 8000 characters of extracted text.',
      es: 'Extrae contenido de texto de un archivo PDF. Devuelve los primeros 8000 caracteres.'
    },
    parameters: [
      { name: 'path', type: 'string', required: true, description: { en: 'Absolute or workspace-relative path to PDF file.', es: 'Ruta absoluta o relativa al espacio de trabajo del archivo PDF.' } }
    ],
    example: {
      prompt: { en: 'Summarize the PDF contract.pdf', es: 'Resume el texto del pdf contrato_arriendo.pdf' },
      arguments: '{\n  "path": "contrato_arriendo.pdf"\n}',
      response: '{\n  "content": "CONTRATO DE ARRENDAMIENTO... Cláusula primera..."\n}'
    }
  },
  {
    name: 'take_screenshot',
    category: 'Smart Utilities',
    description: {
      en: 'Take a full-screen screenshot using GNOME Shell and save it to ~/Pictures.',
      es: 'Captura una imagen completa de la pantalla mediante GNOME Shell y la guarda en ~/Pictures.'
    },
    parameters: [
      { name: 'filename', type: 'string', required: false, description: { en: 'Output filename saved in ~/Pictures.', es: 'Nombre de archivo de salida en ~/Pictures.' } },
      { name: 'include_cursor', type: 'boolean', required: false, description: { en: 'Include mouse cursor.', es: 'Incluir cursor de ratón en captura.' } }
    ],
    example: {
      prompt: { en: 'Take a screenshot including the cursor', es: 'Captura la pantalla incluyendo el cursor' },
      arguments: '{\n  "include_cursor": true\n}',
      response: '{\n  "path": "/home/oscarcode/Pictures/screenshot_20260611_1550.png"\n}'
    }
  },
  {
    name: 'inspect_image',
    category: 'Smart Utilities',
    description: {
      en: 'Attach a local image file so the model can inspect it visually.',
      es: 'Adjunta un archivo de imagen local para que el modelo lo inspeccione de forma visual.'
    },
    parameters: [
      { name: 'path', type: 'string', required: true, description: { en: 'Path to PNG, JPG, or SVG file.', es: 'Ruta al archivo PNG, JPG o SVG.' } }
    ],
    example: {
      prompt: { en: 'Inspect image graph.png visually', es: 'Examina visualmente la imagen grafico.png' },
      arguments: '{\n  "path": "grafico.png"\n}',
      response: '{\n  "content": "Successfully attached image grafico.png to turn context"\n}'
    }
  },
  {
    name: 'save_memory',
    category: 'Smart Utilities',
    description: {
      en: 'Save a durable user memory in the local categorized memory store.',
      es: 'Guarda un dato de memoria persistente del usuario en almacén local categorizado.'
    },
    parameters: [
      { name: 'category', type: 'string', required: true, description: { en: 'Memory category (e.g. preference, routine).', es: 'Categoría de memoria (ej. preference, routine).' } },
      { name: 'content', type: 'string', required: true, description: { en: 'Durable fact or preference to remember.', es: 'Dato o preferencia a recordar de forma duradera.' } }
    ],
    example: {
      prompt: { en: 'Remember my download folder is ~/Downloads/Projects', es: 'Recuerda que mi directorio de descargas favorito es ~/Descargas/Proyectos' },
      arguments: '{\n  "category": "important_folder",\n  "content": "Favorito: ~/Descargas/Proyectos"\n}',
      response: '{\n  "ok": true,\n  "category": "important_folder",\n  "content": "Favorito: ~/Descargas/Proyectos"\n}'
    }
  },
  {
    name: 'web_fetch',
    category: 'Smart Utilities',
    description: {
      en: 'Fetch content from a URL via HTTP GET. Returns response body as text.',
      es: 'Descarga contenido desde una URL por medio de una petición HTTP GET.'
    },
    parameters: [
      { name: 'url', type: 'string', required: true, description: { en: 'HTTP or HTTPS URL to fetch.', es: 'URL de tipo HTTP o HTTPS a descargar.' } },
      { name: 'headers', type: 'string', required: false, description: { en: 'Optional HTTP headers separated by newlines.', es: 'Cabeceras HTTP opcionales separadas por saltos de línea.' } },
      { name: 'timeout', type: 'number', required: false, description: { en: 'Request timeout in seconds.', es: 'Límite de tiempo de petición en segundos.' } }
    ],
    example: {
      prompt: { en: 'Get health status of Samantha local server', es: 'Haz un GET a la api de Samantha en http://127.0.0.1:4389/healthz' },
      arguments: '{\n  "url": "http://127.0.0.1:4389/healthz"\n}',
      response: '{\n  "content": "{\\"status\\":\\"ok\\",\\"version\\":\\"0.1.0\\"}"\n}'
    }
  },
  {
    name: 'notify',
    category: 'Smart Utilities',
    description: {
      en: 'Send a desktop notification to the user using notify-send or osascript.',
      es: 'Envía una notificación de escritorio al usuario usando notify-send o osascript.'
    },
    parameters: [
      { name: 'summary', type: 'string', required: true, description: { en: 'Notification title.', es: 'Título de la notificación.' } },
      { name: 'body', type: 'string', required: false, description: { en: 'Notification description.', es: 'Cuerpo opcional de la notificación.' } },
      { name: 'urgency', type: 'string', required: false, description: { en: 'Urgency: low, normal, critical.', es: 'Urgencia de la notificación: low, normal, critical.' } }
    ],
    example: {
      prompt: { en: 'Send a desktop notification Copy completed', es: 'Manda una alerta simple de escritorio diciendo Copia terminada' },
      arguments: '{\n  "summary": "Copia terminada",\n  "urgency": "low"\n}',
      response: '{\n  "content": "notification sent: Copia terminada"\n}'
    }
  },
  {
    name: 'send_notification',
    category: 'Smart Utilities',
    description: {
      en: 'Send a desktop notification to the user via DBus (org.freedesktop.Notifications).',
      es: 'Envía una notificación de escritorio al usuario mediante DBus (org.freedesktop.Notifications).'
    },
    parameters: [
      { name: 'summary', type: 'string', required: true, description: { en: 'Notification short title.', es: 'Título corto de la notificación.' } },
      { name: 'body', type: 'string', required: false, description: { en: 'Notification body text.', es: 'Texto del cuerpo de la notificación.' } },
      { name: 'icon', type: 'string', required: false, description: { en: 'DBus icon name (e.g. dialog-warning).', es: 'Nombre de icono DBus (ej. dialog-warning).' } },
      { name: 'expire_seconds', type: 'integer', required: false, description: { en: 'Auto-dismiss delay in seconds.', es: 'Tiempo de auto-ocultado en segundos.' } }
    ],
    example: {
      prompt: { en: 'Send DBus notification warning Disk is full', es: 'Envia una notificación DBus con título Alerta, cuerpo El disco está lleno e icono dialog-warning' },
      arguments: '{\n  "summary": "Alerta",\n  "body": "El disco está lleno",\n  "icon": "dialog-warning"\n}',
      response: '{\n  "ok": true,\n  "id": 14\n}'
    }
  }
];

export default function Docs() {
  const [selectedCategory, setSelectedCategory] = useState('Overview');
  const [searchQuery, setSearchQuery] = useState('');
  const [copiedText, setCopiedText] = useState<string | null>(null);
  const { t, i18n } = useTranslation();

  const searchInputRef = useRef<HTMLInputElement>(null);
  const currentLang = i18n.language === 'es' ? 'es' : 'en';

  // Keyboard shortcut '/' to focus search input
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === '/' && document.activeElement !== searchInputRef.current) {
        e.preventDefault();
        searchInputRef.current?.focus();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  // Get translated category name
  const getCategoryLabel = (cat: string) => {
    const isEs = currentLang === 'es';
    switch (cat) {
      case 'Overview': return isEs ? 'Introducción' : 'Overview';
      case 'Quick Start': return isEs ? 'Guía Rápida' : 'Quick Start';
      case 'Files & Directories': return isEs ? 'Archivos y Directorios' : 'Files & Directories';
      case 'System & OS Control': return isEs ? 'Control de Sistema' : 'System & OS Control';
      case 'Productivity Suite': return isEs ? 'Productividad' : 'Productivity Suite';
      case 'Tasks & Scheduling': return isEs ? 'Tareas y Planificación' : 'Tasks & Scheduling';
      case 'Smart Utilities': return isEs ? 'Servicios Inteligentes' : 'Smart Utilities';
      default: return cat;
    }
  };

  // Filter tools based on query & category
  const filteredTools = useMemo(() => {
    let list = TOOLS_DATA;
    if (selectedCategory !== 'Overview' && selectedCategory !== 'Quick Start') {
      list = list.filter(t => t.category === selectedCategory);
    }
    if (searchQuery.trim() !== '') {
      const q = searchQuery.toLowerCase();
      list = list.filter(t => 
        t.name.toLowerCase().includes(q) || 
        t.description[currentLang].toLowerCase().includes(q)
      );
    }
    return list;
  }, [selectedCategory, searchQuery, currentLang]);

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedText(id);
    setTimeout(() => setCopiedText(null), 2000);
  };

  return (
    <div className="docs-page dot-grid-subtle fade-in">
      {/* Sidebar navigation */}
      <aside className="docs-sidebar">
        <div className="sidebar-brand">
          <span className="brand-dot"></span>
          {t('docs.overview')}
        </div>
        <nav className="sidebar-nav">
          {CATEGORIES.map(cat => (
            <button
              key={cat}
              className={`sidebar-nav-item ${selectedCategory === cat ? 'active' : ''}`}
              onClick={() => {
                setSelectedCategory(cat);
                if (searchQuery !== '') {
                  setSearchQuery('');
                }
              }}
            >
              {getCategoryLabel(cat)}
            </button>
          ))}
        </nav>
      </aside>

      {/* Main documentation content */}
      <main className="docs-content">
        <div className="docs-header">
          <div className="docs-search-wrapper">
            <svg className="search-icon" viewBox="0 0 24 24" width="16" height="16">
              <path fill="currentColor" d="M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"/>
            </svg>
            <input
              ref={searchInputRef}
              type="text"
              placeholder={t('docs.searchPlaceholder')}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="docs-search-input"
            />
            {searchQuery && (
              <button onClick={() => setSearchQuery('')} className="search-clear-btn">&times;</button>
            )}
          </div>
        </div>

        {selectedCategory === 'Overview' && !searchQuery ? (
          <div className="docs-overview">
            <div className="label">{t('docs.overview')}</div>
            <h1 className="docs-title">{t('docs.title')}</h1>
            
            <p className="docs-lead">
              {t('docs.lead')}
            </p>

            <div className="overview-card">
              <h3>{t('docs.howWorksTitle')}</h3>
              <p>{t('docs.howWorksText')}</p>
            </div>

            <div className="overview-grid">
              <div className="overview-item">
                <span className="number">01</span>
                <h4>{t('docs.functionCallingTitle')}</h4>
                <p>{t('docs.functionCallingText')}</p>
              </div>
              <div className="overview-item">
                <span className="number">02</span>
                <h4>{t('docs.securityTitle')}</h4>
                <p>{t('docs.securityText')}</p>
              </div>
              <div className="overview-item">
                <span className="number">03</span>
                <h4>{t('docs.asyncTitle')}</h4>
                <p>{t('docs.asyncText')}</p>
              </div>
            </div>

            <h3 style={{ marginTop: '48px', marginBottom: '16px' }}>{t('docs.availableCategories')}</h3>
            <p style={{ color: 'rgba(255,255,255,0.7)', marginBottom: '24px' }}>
              {t('docs.availableCategoriesText')}
            </p>
          </div>
        ) : selectedCategory === 'Quick Start' && !searchQuery ? (
          <div className="docs-quickstart animate-fade-in">
            <div className="label">{t('docs.quickStart')}</div>
            <h1 className="docs-title">{t('docs.quickstartTitle')}</h1>
            
            <p className="docs-lead">
              {t('docs.quickstartIntro')}
            </p>

            <div className="overview-card">
              <h3>{t('docs.quickstartStep1Title')}</h3>
              <p style={{ color: 'rgba(255,255,255,0.7)', marginBottom: '16px' }}>
                {t('docs.quickstartStep1Desc')}
              </p>
              <div>
                <div className="example-header" style={{ marginBottom: '8px' }}>
                  <span className="code-label label">BASH</span>
                  <button 
                    onClick={() => copyToClipboard('sudo apt update && sudo apt install -y build-essential meson ninja-build valac libwingpanel-dev libsoup-3.0-dev libjson-glib-dev libgranite-7-dev libgtk-3-dev golang-go', 'dep-cmd')}
                    className="copy-btn font-mono"
                  >
                    {copiedText === 'dep-cmd' ? t('docs.copied') : t('docs.copyArgs')}
                  </button>
                </div>
                <pre style={{ background: 'rgba(0, 0, 0, 0.25)', border: '1px solid rgba(255, 255, 255, 0.05)', borderRadius: '8px', padding: '16px', fontSize: '13px', overflowX: 'auto', color: '#ECEFF1', fontFamily: 'Space Mono, monospace' }}>
                  {`sudo apt update\nsudo apt install -y \\\n  build-essential \\\n  meson \\\n  ninja-build \\\n  valac \\\n  libwingpanel-dev \\\n  libsoup-3.0-dev \\\n  libjson-glib-dev \\\n  libgranite-7-dev \\\n  libgtk-3-dev \\\n  golang-go`}
                </pre>
              </div>
            </div>

            <div className="overview-card">
              <h3>{t('docs.quickstartStep2Title')}</h3>
              <p style={{ color: 'rgba(255,255,255,0.7)', marginBottom: '16px' }}>
                {t('docs.quickstartStep2Desc')}
              </p>
              <div>
                <div className="example-header" style={{ marginBottom: '8px' }}>
                  <span className="code-label label">BASH</span>
                  <button 
                    onClick={() => copyToClipboard('go build -o claw ./cmd/claw\nsudo install -m 0755 claw /usr/local/bin/claw\nsudo install -d /etc/systemd/user\nsudo install -m 0644 deployments/systemd/elementary-claw.service /etc/systemd/user/elementary-claw.service\nsystemctl --user daemon-reload\nsystemctl --user enable --now elementary-claw.service', 'go-cmd')}
                    className="copy-btn font-mono"
                  >
                    {copiedText === 'go-cmd' ? t('docs.copied') : t('docs.copyArgs')}
                  </button>
                </div>
                <pre style={{ background: 'rgba(0, 0, 0, 0.25)', border: '1px solid rgba(255, 255, 255, 0.05)', borderRadius: '8px', padding: '16px', fontSize: '13px', overflowX: 'auto', color: '#ECEFF1', fontFamily: 'Space Mono, monospace' }}>
                  {`# Build the Samantha engine\ngo build -o claw ./cmd/claw\n\n# Install the binary\nsudo install -m 0755 claw /usr/local/bin/claw\n\n# Register and enable the systemd user service for Samantha\nsudo install -d /etc/systemd/user\nsudo install -m 0644 deployments/systemd/elementary-claw.service /etc/systemd/user/elementary-claw.service\nsystemctl --user daemon-reload\nsystemctl --user enable --now elementary-claw.service`}
                </pre>
              </div>
            </div>

            <div className="overview-card">
              <h3>{t('docs.quickstartStep3Title')}</h3>
              <p style={{ color: 'rgba(255,255,255,0.7)', marginBottom: '16px' }}>
                {t('docs.quickstartStep3Desc')}
              </p>
              <div>
                <div className="example-header" style={{ marginBottom: '8px' }}>
                  <span className="code-label label">BASH</span>
                  <button 
                    onClick={() => copyToClipboard('cd panel-sam\nmeson setup build --prefix=/usr\nninja -C build\nsudo ninja -C build install\nkillall io.elementary.wingpanel', 'vala-cmd')}
                    className="copy-btn font-mono"
                  >
                    {copiedText === 'vala-cmd' ? t('docs.copied') : t('docs.copyArgs')}
                  </button>
                </div>
                <pre style={{ background: 'rgba(0, 0, 0, 0.25)', border: '1px solid rgba(255, 255, 255, 0.05)', borderRadius: '8px', padding: '16px', fontSize: '13px', overflowX: 'auto', color: '#ECEFF1', fontFamily: 'Space Mono, monospace' }}>
                  {`# Navigate to panel directory\ncd panel-sam\n\n# Configure build system with system prefix\nmeson setup build --prefix=/usr\n\n# Compile the project\nninja -C build\n\n# Install the indicator module\nsudo ninja -C build install\n\n# Restart Wingpanel to load the new indicator\nkillall io.elementary.wingpanel`}
                </pre>
              </div>
            </div>
          </div>
        ) : (
          <div className="docs-tools-list">
            <div className="label">
              {searchQuery ? `${t('docs.searchResultFor')} "${searchQuery}"` : getCategoryLabel(selectedCategory)}
            </div>
            <h1 className="docs-title">
              {searchQuery ? t('docs.searchPlaceholder').split('...')[0] : `${getCategoryLabel(selectedCategory)} ${t('docs.reference')}`}
            </h1>

            {filteredTools.length === 0 ? (
              <div className="docs-empty-state">
                {t('docs.emptyState')}
              </div>
            ) : (
              filteredTools.map(tool => (
                <div key={tool.name} className="tool-card">
                  <div className="tool-card-header">
                    <h2 className="tool-name font-mono">{tool.name}</h2>
                    <span className="tool-badge">{getCategoryLabel(tool.category)}</span>
                  </div>
                  
                  <p className="tool-description">{tool.description[currentLang]}</p>

                  {tool.parameters.length > 0 && (
                    <div className="tool-section">
                      <h4>{t('docs.parameters')}</h4>
                      <div className="tool-params-table-wrapper">
                        <table className="tool-params-table">
                          <thead>
                            <tr>
                              <th>{t('docs.parameter')}</th>
                              <th>{t('docs.type')}</th>
                              <th>{t('docs.required')}</th>
                              <th>{t('docs.description')}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {tool.parameters.map(param => (
                              <tr key={param.name}>
                                <td className="param-name font-mono">{param.name}</td>
                                <td className="param-type font-mono">{param.type}</td>
                                <td>
                                  <span className={`param-req ${param.required ? 'req-true' : 'req-false'}`}>
                                    {param.required ? t('docs.required').toLowerCase() : 'no'}
                                  </span>
                                </td>
                                <td className="param-desc">{param.description[currentLang]}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}

                  <div className="tool-section">
                    <div className="example-header">
                      <h4>{t('docs.usageExample')}</h4>
                      <button 
                        onClick={() => copyToClipboard(tool.example.arguments, tool.name)}
                        className="copy-btn font-mono"
                      >
                        {copiedText === tool.name ? t('docs.copied') : t('docs.copyArgs')}
                      </button>
                    </div>
                    <div className="example-prompt">
                      <strong>Prompt:</strong> <i>"{tool.example.prompt[currentLang]}"</i>
                    </div>
                    <div className="example-code-grid">
                      <div>
                        <div className="code-label label">{t('docs.arguments')} (JSON)</div>
                        <pre className="font-mono">{tool.example.arguments}</pre>
                      </div>
                      <div>
                        <div className="code-label label">{t('docs.result')} (JSON)</div>
                        <pre className="font-mono">{tool.example.response}</pre>
                      </div>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        )}
      </main>
    </div>
  );
}
