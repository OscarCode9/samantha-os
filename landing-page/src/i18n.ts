import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';

const resources = {
  en: {
    translation: {
      nav: {
        home: 'Home',
        features: 'Features',
        howItWorks: 'How it works',
        docs: 'Docs'
      },
      hero: {
        eyebrow: 'Samantha OS',
        title: 'Your OS finally has a mind.',
        subtitle: 'An AI assistant that lives in your computer — not in a browser tab.',
        comingSoon: 'Coming soon',
        watchFilm: 'Watch the film',
        trust: 'Built for Samantha OS. GitHub Copilot powered.',
        mockupTitle: 'Samantha OS.',
        mockupSubtitle: 'The first AI-native operating system.',
        getStarted: 'Get Started'
      },
      features: {
        sectionTitle: 'Not a web app. A real OS citizen.',
        zeroSetupTitle: 'Zero setup.',
        zeroSetupBody: 'Detects your GitHub account. Device flow auth in under 60 seconds. No API keys to find. No config files to edit.',
        livesPanelTitle: 'Lives in your panel.',
        livesPanelBody: 'Always one click away from the Wingpanel. Ask anything without leaving your current app. Like Spotlight, but it talks back.',
        filesRulesTitle: 'Your files, your rules.',
        filesRulesBody: 'Runs locally on your machine. Your code, your repos, your context. Never leaves your OS unless you say so.',
        talksToolsTitle: 'Talks to your tools.',
        talksToolsBody: 'Terminal, file manager, code editor, settings. Samantha sees what you see and acts where you act.',
        remembersTitle: 'Remembers across sessions.',
        remembersBody: 'Picks up where you left off. Every conversation, every file, every project — all in one persistent workspace.',
        openModelTitle: 'Open provider model.',
        openModelBody: 'Starts with GitHub Copilot. Anthropic, OpenAI, and local models coming. You choose the brain.'
      },
      how: {
        title: 'How it works',
        step1Title: 'Install Samantha OS.',
        step1Desc: 'Samantha comes preloaded.',
        step2Title: 'Create your account.',
        step2Desc: 'The familiar Initial Setup now includes one new screen.',
        step3Title: 'Connect in under a minute.',
        step3Desc: 'GitHub device flow. One code. One browser tab. Done.',
        step4Title: 'Your OS thinks with you.',
        step4Desc: 'Open the panel, start a chat, and never look back.'
      },
      quote: {
        text: 'This is what an operating system should have always been.',
        author: 'Samantha OS user, 2026'
      },
      footer: {
        about: 'About',
        docs: 'Docs',
        github: 'GitHub',
        privacy: 'Privacy',
        finePrint: 'Samantha OS is an independent operating system. GitHub Copilot is a trademark of GitHub, Inc. Not affiliated with OpenAI, Anthropic, or any AI provider.'
      },
      docs: {
        overview: 'Overview',
        quickStart: 'Quick Start',
        quickstartTitle: 'Quick Start Guide',
        quickstartIntro: 'Learn how to build and install the Vala wingpanel indicator (panel-sam) and the background Go runtime (Samantha Engine) on your local machine or VM running elementary OS.',
        quickstartStep0Title: 'Step 0: Clone the Repository',
        quickstartStep0Desc: 'First, clone the official Samantha OS repository to your local machine or virtual machine:',
        quickstartStep1Title: 'Step 1: Install Build Dependencies',
        quickstartStep1Desc: 'Before building the panel and the runtime, make sure you have the required build tools and libraries installed:',
        quickstartStep2Title: 'Step 2: Build & Run the Samantha Engine Gateway',
        quickstartStep2Desc: 'The gateway handles communications with the LLM and executes system tools. Compile and start it as a user-level service:',
        quickstartStep3Title: 'Step 3: Build & Install the Vala Panel Indicator',
        quickstartStep3Desc: 'The panel-sam indicator provides the graphical interface in your top bar. Compile it and restart Wingpanel to load it:',
        searchPlaceholder: "Search tools... (Press '/' to focus)",
        title: 'Samantha OS Tooling System',
        lead: 'Samantha OS relies on a secure, robust Tool Calling system that links the LLM core directly to standard APIs, DBus channels, and command-line scripts on elementary OS.',
        howWorksTitle: 'How Tools Work',
        howWorksText: 'When a user makes a request (e.g. "What meetings do I have scheduled for today?"), Samantha analyzes the request, drafts a multi-step execution plan, and calls the appropriate system tools. The tool outputs are sent back as context in subsequent LLM turns, providing a natural planning and execution loop.',
        functionCallingTitle: 'Function Calling',
        functionCallingText: 'OpenAI/Copilot compatible JSON Schemas describe the parameters and inputs for all native tools.',
        securityTitle: 'Workspace Security',
        securityText: 'All file inputs are checked and sandboxed relative to the workspace root directory unless absolute privileges are explicitly granted.',
        asyncTitle: 'Async Design',
        asyncText: 'Long running operations (like downloads, searches, or scripts) run asynchronously to keep the UI fluid.',
        availableCategories: 'Available Categories',
        availableCategoriesText: 'Select a category on the left sidebar to drill down into the list of tools, parameter requirements, and call examples.',
        searchResultFor: 'Search results for',
        reference: 'Reference',
        emptyState: 'No tools found matching your criteria. Try adjusting your query or category filter.',
        parameters: 'Parameters',
        parameter: 'Parameter',
        type: 'Type',
        required: 'Required',
        description: 'Description',
        usageExample: 'Usage Example',
        copyArgs: 'Copy Args',
        copied: 'Copied!',
        arguments: 'Arguments',
        result: 'Result'
      }
    }
  },
  es: {
    translation: {
      nav: {
        home: 'Inicio',
        features: 'Características',
        howItWorks: 'Cómo funciona',
        docs: 'Docs'
      },
      hero: {
        eyebrow: 'Samantha OS',
        title: 'Tu sistema operativo finalmente tiene mente.',
        subtitle: 'Una asistente de IA que vive en tu computadora, no en una pestaña del navegador.',
        comingSoon: 'Próximamente',
        watchFilm: 'Ver el video',
        trust: 'Creado para Samantha OS. Potenciado por GitHub Copilot.',
        mockupTitle: 'Samantha OS.',
        mockupSubtitle: 'El primer sistema operativo nativo de IA.',
        getStarted: 'Comenzar'
      },
      features: {
        sectionTitle: 'No es una aplicación web. Un ciudadano real del sistema operativo.',
        zeroSetupTitle: 'Cero configuración.',
        zeroSetupBody: 'Detecta tu cuenta de GitHub. Autenticación por dispositivo en menos de 60 segundos. Sin llaves API que buscar ni archivos que editar.',
        livesPanelTitle: 'Vive en tu panel.',
        livesPanelBody: 'Siempre a un clic de distancia en el Wingpanel. Pregunta lo que sea sin salir de tu aplicación actual. Como Spotlight, pero responde.',
        filesRulesTitle: 'Tus archivos, tus reglas.',
        filesRulesBody: 'Ejecución local en tu máquina. Tu código, tus repositorios, tu contexto. Nunca sale de tu sistema a menos que lo autorices.',
        talksToolsTitle: 'Habla con tus herramientas.',
        talksToolsBody: 'Terminal, administrador de archivos, editor de código, ajustes. Samantha ve lo que tú ves y actúa donde tú actúas.',
        remembersTitle: 'Recuerda entre sesiones.',
        remembersBody: 'Retoma donde lo dejaste. Cada conversación, cada archivo, cada proyecto; todo en un espacio de trabajo persistente.',
        openModelTitle: 'Modelo de proveedor abierto.',
        openModelBody: 'Inicia con GitHub Copilot. Anthropic, OpenAI y modelos locales en camino. Tú eliges el cerebro.'
      },
      how: {
        title: 'Cómo funciona',
        step1Title: 'Instala Samantha OS.',
        step1Desc: 'Samantha viene preinstalada.',
        step2Title: 'Crea tu cuenta.',
        step2Desc: 'La configuración inicial de siempre ahora incluye una pantalla adicional.',
        step3Title: 'Conéctate en menos de un minuto.',
        step3Desc: 'Flujo por dispositivo de GitHub. Un código. Una pestaña del navegador. Listo.',
        step4Title: 'Tu sistema piensa contigo.',
        step4Desc: 'Abre el panel, inicia una conversación y no mires atrás.'
      },
      quote: {
        text: 'Esto es lo que un sistema operativo siempre debió haber sido.',
        author: 'Usuario de Samantha OS, 2026'
      },
      footer: {
        about: 'Acerca de',
        docs: 'Documentación',
        github: 'GitHub',
        privacy: 'Privacidad',
        finePrint: 'Samantha OS es un sistema operativo independiente. GitHub Copilot es una marca registrada de GitHub, Inc. No afiliado con OpenAI, Anthropic o proveedores de IA.'
      },
      docs: {
        overview: 'Introducción',
        quickStart: 'Guía Rápida',
        quickstartTitle: 'Guía de Inicio Rápido',
        quickstartIntro: 'Aprende a compilar e instalar el indicador de panel en Vala (panel-sam) y el motor de ejecución en Go (Motor de Samantha) en tu máquina local o máquina virtual con elementary OS.',
        quickstartStep0Title: 'Paso 0: Clonar el Repositorio',
        quickstartStep0Desc: 'Primero, clona el repositorio oficial de Samantha OS en tu máquina local o máquina virtual:',
        quickstartStep1Title: 'Paso 1: Instalar Dependencias',
        quickstartStep1Desc: 'Antes de compilar el panel y el motor de ejecución, asegúrate de tener instaladas las herramientas de compilación y librerías necesarias:',
        quickstartStep2Title: 'Paso 2: Compilar y Ejecutar el Motor de Samantha',
        quickstartStep2Desc: 'El gateway gestiona la comunicación con el LLM y ejecuta las herramientas. Compílalo e inícialo como un servicio de usuario:',
        quickstartStep3Title: 'Paso 3: Compilar e Instalar el Indicador de Panel en Vala',
        quickstartStep3Desc: 'El indicador panel-sam proporciona la interfaz gráfica en la barra superior. Compílalo y reinicia Wingpanel para cargarlo:',
        searchPlaceholder: "Buscar herramientas... (Presiona '/' para enfocar)",
        title: 'Sistema de Herramientas de Samantha OS',
        lead: 'Samantha OS cuenta con un sistema robusto y seguro de llamadas de herramientas (Tool Calling) que vincula el núcleo de IA directamente con APIs estándar, canales DBus y scripts de comandos en elementary OS.',
        howWorksTitle: 'Cómo Funcionan las Herramientas',
        howWorksText: 'Cuando haces una solicitud (ej. "¿Qué reuniones tengo programadas hoy?"), Samantha la analiza, diseña un plan de ejecución de múltiples pasos y ejecuta las herramientas del sistema pertinentes. Los resultados se envían de vuelta al modelo para formular la respuesta final.',
        functionCallingTitle: 'Llamada de Funciones',
        functionCallingText: 'Esquemas JSON compatibles con OpenAI/Copilot describen los parámetros y entradas de todas las herramientas.',
        securityTitle: 'Seguridad de Espacio',
        securityText: 'Las entradas de archivos se validan y limitan al directorio raíz del espacio de trabajo para mayor seguridad.',
        asyncTitle: 'Diseño Asíncrono',
        asyncText: 'Las operaciones de larga duración se ejecutan asíncronamente para mantener la fluidez de la interfaz.',
        availableCategories: 'Categorías Disponibles',
        availableCategoriesText: 'Selecciona una categoría en el panel izquierdo para explorar las herramientas, sus parámetros y ejemplos de uso.',
        searchResultFor: 'Resultados de búsqueda para',
        reference: 'Referencia',
        emptyState: 'No se encontraron herramientas. Intenta ajustar tu búsqueda o el filtro de categorías.',
        parameters: 'Parámetros',
        parameter: 'Parámetro',
        type: 'Tipo',
        required: 'Requerido',
        description: 'Descripción',
        usageExample: 'Ejemplo de Uso',
        copyArgs: 'Copiar Args',
        copied: '¡Copiado!',
        arguments: 'Argumentos',
        result: 'Resultado'
      }
    }
  }
};

// Find browser language or default to English
const getInitialLanguage = () => {
  const lang = navigator.language || (navigator as any).userLanguage || 'en';
  return lang.startsWith('es') ? 'es' : 'en';
};

i18n
  .use(initReactI18next)
  .init({
    resources,
    lng: getInitialLanguage(),
    fallbackLng: 'en',
    interpolation: {
      escapeValue: false // React already escapes values
    }
  });

export default i18n;
