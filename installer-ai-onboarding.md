# installer-ai-onboarding

Fecha: 2026-04-02

## 1. Objetivo

Documentar como hacer que elementary OS ofrezca conectar una IA desde el primer momento de uso del sistema, usando GitHub Copilot como v1 por ser barato, accesible y ya soportado como proveedor en OpenClaw.

Este documento responde a una pregunta especifica:

> Como hacemos que, desde la instalacion o primer arranque del sistema operativo, el usuario tenga la opcion de conectar su IA y salir del setup con el agente ya funcionando.

## 2. Respuesta corta

Si se puede, y para que quede funcional de verdad hay que dividirlo en dos capas que cooperan.

La implementacion correcta es esta:

1. preinstalar el stack de `elementary-claw` en la imagen o metapaquete del sistema
2. enganchar el flujo de conexion de IA en `Initial Setup` inmediatamente despues de crear la cuenta
3. crear y provisionar el agente del usuario en ese mismo paso
4. ejecutar login del proveedor a nivel usuario si ya hay internet
5. dejar sembrados estado, identidad, personalidad y sesion inicial del agente
6. continuar con el runtime persistente en el primer login

Conclusion:

- Si puede sentirse como “desde la instalacion”.
- La autenticacion real debe ocurrir despues de que exista el usuario, pero todavia dentro de `Initial Setup`.
- Para Copilot v1, la ruta correcta es device flow nativo, no VS Code proxy.
- El primer login no debe ser el momento donde nace el agente; debe ser el momento donde el runtime persistente retoma un agente ya creado.

## 3. Lo que validamos

## 3.1 elementary OS

Hallazgos confirmados:

- `elementary/os` construye la distribucion e imagen con Debian `live-build`.
- `elementary/initial-setup` es la app de primer uso para crear nuevos usuarios.
- `elementary/onboarding` es la app de onboarding para nuevos usuarios.
- Una imagen custom puede llevar paquetes preinstalados.
- Para persistir cosas en nuevos usuarios, hay que usar archivos del sistema o `/etc/skel/`.

Implicacion:

- El hook natural para “Conectar IA” no es el build de la ISO.
- El hook natural es `initial-setup` o `onboarding`.

## 3.2 OpenClaw

Hallazgos confirmados:

- OpenClaw tiene onboarding guiado para configurar modelo, auth, workspace, daemon y skills.
- OpenClaw soporta `github-copilot` como proveedor nativo.
- El login de GitHub Copilot usa device flow.
- OpenClaw tambien soporta `copilot-proxy`, pero esa ruta depende de VS Code y una extension local.

Implicacion:

- Para una experiencia de sistema operativo, el camino correcto es el proveedor nativo `github-copilot`.
- `copilot-proxy` no es buena opcion para v1 del sistema porque amarra la experiencia a VS Code abierto y al LM Proxy.

## 3.3 Patron tipo TenK

Patron util identificado:

- abrir URL de autorizacion
- mostrar codigo
- esperar aprobacion
- guardar token local por usuario
- operar despues con ese token

Implicacion:

- Ese mismo patron sirve perfecto para `elementary-claw` con GitHub Copilot.

## 4. Decision de arquitectura

## 4.1 Donde debe vivir el flujo “Conectar IA”

Opciones:

### Opcion A. Dentro del instalador puro

Ventajas:

- se siente mas “desde instalacion”

Problemas:

- todavia no existe usuario final
- no hay home directory del usuario listo para guardar auth
- mala ergonomia para device flow y navegador
- mezclar secretos personales con el proceso de instalacion es mala practica

Veredicto:

- no recomendado para v1

### Opcion B. En `Initial Setup` despues de crear usuario

Ventajas:

- ya existe el usuario
- ya existe su home
- ya se puede guardar config y credenciales por usuario
- se puede abrir navegador o mostrar un code flow
- sigue sintiendose “desde el primer momento”
- permite crear el agente y no solo la cuenta
- permite dejar lista una primera interaccion real si ya hay internet

Veredicto:

- mejor opcion tecnica
- esta es la opcion que mejor representa la diferenciacion del producto

### Opcion C. En `Onboarding` despues del primer login

Ventajas:

- aun mas limpio para UX y permisos
- permite mostrar explicaciones y opcion de saltar
- separa creacion de cuenta del setup de IA

Veredicto:

- tambien valida
- muy buena opcion si no quieren tocar `initial-setup` tan temprano

## 4.2 Recomendacion final

Para v1:

- preinstalar `elementary-claw`
- agregar pagina `Conectar IA` en `initial-setup` justo despues de crear la cuenta
- crear y provisionar el agente dentro de ese mismo flujo
- usar `onboarding` como continuidad o fallback si el usuario omite el paso o si no hay conectividad

Mi recomendacion:

- si quieren avanzar con la diferenciacion real del producto: modificar `initial-setup`
- si quieren reducir riesgo UX o tecnico: mantener `onboarding` como respaldo post-login, no como punto principal de creacion del agente

## 5. Como debe funcionar con GitHub Copilot v1

## 5.1 Por que GitHub Copilot

Ventajas:

- mucha gente ya tiene cuenta GitHub
- hay plan gratis y planes accesibles
- OpenClaw ya demuestra que el modelo se puede usar como proveedor
- no obliga a pedir API key manual al usuario en v1
- el device flow es apto para una pantalla de setup

## 5.2 Que NO debemos usar en v1

No usar `copilot-proxy` como ruta principal.

Razon:

- depende de VS Code
- depende de extension adicional
- depende de que el usuario mantenga el proxy vivo
- no es una experiencia limpia de sistema operativo

## 5.3 Flujo ideal de UX

### Flujo propuesto

1. El usuario instala o inicia por primera vez elementary OS.
2. `Initial Setup` crea el usuario.
3. Aparece la pantalla: `Conecta tu asistente de IA`.
4. Se ofrecen opciones:
   - GitHub Copilot
   - Omitir por ahora
   - Configurar otro proveedor despues
5. Si elige Copilot:
   - el sistema inicia device flow
   - muestra URL y codigo
   - opcionalmente abre el navegador por defecto
   - hace polling hasta autorizacion
6. Cuando GitHub aprueba:
   - se guarda el token GitHub del usuario
   - se escribe la configuracion inicial del agente
   - se escriben identidad, soul, contexto y bootstrap del agente
   - un comando one-shot del runtime genera el primer saludo o primer intercambio real
   - esa sesion inicial queda guardada para retomarse despues
   - se deja preparado el servicio de usuario para el primer login
7. Si el usuario omite el paso o no hay conectividad:
   - igual se crea el agente local con identidad y personalidad base
   - el proveedor queda en estado pendiente
   - el primer login ofrece reconectar o terminar la provision del agente
8. En el primer login, la app nativa de `elementary-claw` retoma la misma sesion inicial o abre la continuacion del onboarding.

## 5.4 Que se guarda y donde

Guardar por usuario, no por sistema.

Recomendado para v1 funcional:

- config del agente en el home del usuario
- auth profile con permisos `0600` y ownership del usuario
- workspace del agente en `~/.openclaw/workspace`
- sesion bootstrap persistida para retomarla en el primer login

Endurecimiento recomendado para v2:

- migrar el token GitHub a Secret Service en el primer login, cuando ya exista sesion grafica del usuario
- dejar en disco solo referencias o estado no sensible

No recomendado:

- meter tokens en la imagen del sistema
- guardar secretos en `/etc/skel/`
- compartir un solo token de sistema entre usuarios

## 6. Patron tecnico basado en OpenClaw

## 6.1 Lo que hace OpenClaw y nos conviene copiar

OpenClaw usa este modelo:

1. login de GitHub via device flow
2. guarda un auth profile del proveedor `github-copilot`
3. en runtime intercambia el token GitHub por un token Copilot de corta duracion
4. usa ese token runtime para llamar al proveedor

Esto es exactamente lo que conviene para `elementary-claw`.

## 6.2 Lo que eso significa para nuestro sistema

Nuestro stack deberia tener:

- `elementary-claw-setup`: modulo de onboarding
- `elementary-claw-agentd`: daemon o user service
- `elementary-claw-app`: app nativa
- `claw bootstrap first-message`: comando one-shot para generar el primer intercambio desde `Initial Setup`
- `elementary-claw-config`: config por usuario

Flujo interno:

```text
Initial Setup / Onboarding
        |
        v
GitHub Device Flow
        |
        v
Store GitHub token in user auth state
        |
        v
Write user config for elementary-claw
        |
        v
Generate first bootstrap exchange
        |
        v
Persist bootstrap session
        |
        v
Enable user service
        |
        v
Launch app or resume session on first login
```

## 7. Patron tecnico basado en TenK

El patron tipo TenK sirve como referencia simple:

- auth command
- browser approval
- polling
- token local por usuario
- operaciones posteriores con ese token

Para `elementary-claw`, la version equivalente seria:

```bash
claw auth copilot login
claw auth status
claw models set github-copilot/gpt-5.4
claw service enable
```

## 8. Componentes concretos de elementary OS a tocar

## 8.1 Minimo viable

- paquete `.deb` de `elementary-claw`
- user service
- app nativa
- integracion con `onboarding` o `initial-setup`

## 8.2 Si quieren integracion profunda

- `elementary/initial-setup`
  para ofrecer la pantalla de conexion IA justo despues de crear usuario

- `elementary/onboarding`
  para una experiencia post-login mas rica y menos invasiva

- `elementary/os`
  para incluir los paquetes por defecto en la imagen o metapaquete

## 8.3 Lo que NO hace falta tocar en v1

- `gala`
- `wingpanel`
- `settings`

Eso puede venir despues. Para v1 no bloquea el objetivo.

## 9. Como se mete esto en la instalacion del sistema

## 9.1 Camino recomendado

1. crear paquete `.deb` de `elementary-claw`
2. crear metapaquete `elementary-claw-defaults`
3. incluirlo en la imagen custom o en el proceso OEM
4. instalar `elementary-claw.service` como user service y un launcher de primer arranque
5. enganchar `initial-setup` para provisionar el agente apenas se crea la cuenta
6. sembrar solo config no sensible en `/etc/skel/` si hace falta

## 9.2 Que SI puede ir en `/etc/skel/`

- archivos base de config
- banderas como `showOnFirstLogin=true`
- desktop entries/autostart del onboarding del agente

## 9.3 Que NO debe ir en `/etc/skel/`

- tokens
- credenciales OAuth
- secretos por usuario

## 10. Propuesta de v1

## 10.1 Scope realista

La v1 deberia hacer solo esto:

1. venir preinstalada en la imagen
2. preguntar al usuario si quiere crear y conectar su IA justo despues de crear la cuenta
3. soportar solo GitHub Copilot al inicio
4. usar device flow
5. guardar credenciales por usuario
6. crear identidad, soul, contexto y bootstrap del agente
7. ejecutar un primer intercambio real desde el propio setup cuando haya internet
8. arrancar el agente local persistente en el primer login
9. abrir una app nativa retomando esa misma sesion

## 10.2 Scope que dejamos para despues

- Anthropic
- OpenAI directo
- MCP complejo desde onboarding
- multiple agents
- integracion con Wingpanel
- Switchboard plug
- politicas multiusuario avanzadas

## 11. Riesgos y decisiones

## 11.1 Riesgo principal

Confundir “desde instalacion” con “antes de crear usuario”.

Eso complica almacenamiento de secretos y UX.

Mitigacion:

- ejecutar la creacion y auth del agente justo despues de crear la cuenta en `initial-setup`
- dejar `onboarding` como continuidad o fallback, no como unico lugar donde nace el agente

## 11.2 Riesgo de producto

Hacer obligatoria la IA.

Mitigacion:

- debe existir `Omitir por ahora`
- debe poder configurarse despues

## 11.3 Riesgo tecnico

Usar VS Code proxy como dependencia del sistema.

Mitigacion:

- usar proveedor nativo `github-copilot`

## 11.4 Riesgo de seguridad

Guardar tokens en texto plano o a nivel sistema.

Mitigacion:

- almacenar por usuario con permisos estrictos en v1
- migrar a Secret Service o almacenamiento seguro equivalente cuando ya exista la sesion del usuario

## 12. Pruebas que hay que hacer

## 12.1 Casos funcionales

1. instalar imagen custom
2. crear usuario nuevo
3. ver pantalla `Conectar IA`
4. elegir GitHub Copilot
5. completar device flow
6. verificar que se crea el agente y se guarda la sesion bootstrap
7. verificar que el servicio se habilita
8. abrir app nativa y continuar el primer mensaje ya creado

## 12.2 Casos negativos

1. sin internet
2. usuario cancela el login
3. GitHub rechaza el device flow
4. usuario no tiene plan valido para cierto modelo
5. token expira
6. reinicio entre autorizacion y guardado

## 12.3 Criterio de exito

El usuario recien instalado debe poder:

- crear su usuario
- conectar Copilot en menos de 3 minutos
- terminar con el agente listo
- poder omitir el paso sin romper nada

## 13. Recomendacion final

Si la meta es “IA desde el primer momento del sistema operativo”, la implementacion correcta es esta:

- preinstalar `elementary-claw` en la imagen
- agregar el paso `Conectar IA` en `elementary/initial-setup` inmediatamente despues de crear la cuenta
- usar GitHub Copilot como primer proveedor
- hacer login con device flow
- guardar auth por usuario
- crear el agente en ese mismo flujo
- ejecutar el primer intercambio desde el setup si hay conectividad
- iniciar el servicio del agente para continuar en el primer login

La ruta correcta para v1 no es meter esto en el instalador duro antes de que exista el usuario, sino dentro de `Initial Setup` despues de crear la cuenta y antes de entregar el sistema al primer login.

## 14. Siguiente paso inmediato

Lo siguiente que conviene hacer es cerrar la implementacion funcional sobre el PoC actual.

Entregables recomendados:

1. comando `claw bootstrap first-message` para generar un primer saludo real desde `Initial Setup`
2. persistencia de una sesion bootstrap que luego retome la app nativa
3. user service `elementary-claw.service` preinstalado y listo para primer login
4. ruta de fallback cuando el usuario omite el login o no hay internet
5. decision de endurecimiento de secretos: archivo `0600` en v1 y migracion a Secret Service en v2

## 15. Implementacion funcional exacta

Para que esto quede funcional de verdad, el flujo tecnico debe quedar asi:

1. `AccountView` crea el usuario.
2. `AIConnectView` recoge proveedor, identidad y personalidad del agente.
3. `OpenClawBootstrap.provision_user(...)` escribe el estado inicial del agente en el home del nuevo usuario.
4. Si hay internet y el usuario completa device flow, `Initial Setup` invoca un comando real del runtime como el nuevo usuario para generar el primer mensaje.
5. Ese comando guarda una sesion inicial en el estado local del agente.
6. Al terminar `Initial Setup`, el sistema deja listo el servicio persistente y un autostart de primer arranque.
7. En el primer login, `elementary-claw-app` abre la sesion inicial ya creada en lugar de empezar desde cero.

Checklist tecnico concreto:

- mantener el paso `AIConnectView` justo despues de `AccountView`
- agregar un binario o subcomando del runtime que pueda correr one-shot desde setup
- persistir transcript o session seed en el home del usuario
- instalar `elementary-claw.service` como parte del paquete base
- instalar un `.desktop` de primer arranque que abra la app y luego se autodesactive
- si no hay internet, crear igual el agente y marcar `provider_pending=true`
- si el login falla, no romper el setup; dejar reintento disponible luego