# Despliegue guiado — registro de diseño

> Ola cerrada 2026-10-20, archivada desde `webtyp/docs/DEPLOY_WIZARD_MASTER_PLAN.md`. `deploy`
> define los contratos de esta ola; este es el registro permanente de por qué y cómo.

**Disparador:** desplegar `veltylabs/misitio` exigía pasos manuales repartidos en tres documentos:
crear D1 y R2 a mano, crear un token de Cloudflare a mano, `gh secret set` cuatro veces, cargar
secretos del Worker en el dashboard. El último intento falló porque el token hecho a mano no tenía
`Workers Scripts:Edit` (`PUT workers/scripts/misitio → 403`), y nada lo detectó antes del deploy.

**Objetivo:** cada vez que se quiere desplegar, `webtyp/app` abre un wizard que sabe qué falta, lo
pide, lo crea donde hay API, lo entrega a cada destino (CI, Worker) y despliega. El agente ve lo
mismo por MCP. Funciona para un equipo: cualquier persona autorizada aprovisiona y despliega sin
recibir nada de otra. Nadie edita `.env`, nadie corre `gh secret set`, nadie crea un token a mano.

**Alcance:** `webtyp/wizard`, `webtyp/deploy`, `webtyp/goflare`, `webtyp/github` (nuevo),
`webtyp/env`, `webtyp/keyring`, `webtyp/app`, y `veltylabs/misitio` como caso de aceptación.

## Decisiones

| # | Decisión | Consecuencia |
|---|---|---|
| D1 (reescrita por D29) | Un secreto de producción vive solo en el almacén de su destino (GitHub Actions o Worker); ningún desarrollador tiene una copia; recuperar es siempre rotar | Skill **secrets** reescrita con este modelo |
| D2 | `webtyp/app` es el orquestador: no sabe *cómo* se despliega nada, recibe inyectadas las formas | `app` es la raíz de composición |
| D3 | `webtyp/deploy` no depende de ninguna forma de desplegar; solo define el contrato | Se borran `providers/cloudflare/`, `pusher_cloudflare.go`, el registro global en `init()` |
| D7 | La declaración es `config/deploy.go`, un `deploy.Plan` literal leído por AST (como `config/routes.go`) | `.github/workflows/deploy.yml` se genera desde la declaración |
| D9 (reescrita por D29) | Los secretos se entregan al destino por su API en el momento en que se crean; runtime va directo al Worker, nunca a GitHub | Un `PUT` de script conserva los bindings existentes |
| D10 | Token de arranque de Cloudflare en el keyring de la persona; el wizard crea un token por proyecto con exactamente los permisos que implica la declaración | `goflare` deja de exigir un token hecho a mano |
| D16 | Puller de Windows, SSH y webhook se borran: sin consumidor; `webtyp/selfhost` no se crea | — |
| D17 | La pestaña DEPLOY corre el diagnóstico al abrir el proyecto: checklist ✅/❌, cada ❌ es un paso del wizard | El agente ve la misma checklist por MCP |
| D21 | El deploy es herramienta MCP; los valores secretos solo se escriben en el wizard de la TUI, ninguna herramienta MCP recibe ni devuelve un secreto | `deploy_status`, `deploy_guide`, `deploy_provision`, `deploy_rotate`, `deploy_release` |
| D29 | Ningún secreto de producción se guarda en el keyring de nadie; "al día" se lee de metadatos no secretos del destino | Sin huellas locales |
| D30 | Cada persona tiene su propio token de arranque (miembro de la cuenta de Cloudflare); los tokens por proyecto son del ámbito de la cuenta | Sobreviven a que su creador deje el equipo |
| D32 | Permiso de despliegue = permiso de GitHub; webtyp no tiene roles propios | misitio: quien tiene escritura en el repo despliega |
| D33 | Aceptación con dos personas, dos máquinas, sin recibir nada una de la otra | Ver «Aceptación» |
| D34 | `config/deploy.go` con `//go:build deploy`; `Target`/`CI` como selector de constante exportada (`goflare.Worker`), resuelto por `deploy.Scan` vía ruta de import | Mismo enfoque que `routescan`; cualquier otra expresión es error con archivo y línea |
| D35 | Contrato: `Plan{Name, Domain, Target, CI, Databases, Buckets, Secrets, Vars}`, `Deployment{Plan, Target, CI}` con `Check`, `Provision`, `Rotate`, `Release` | Firma exacta en `deploy` |
| D38 | `goflare` resuelve D1 y R2 por nombre, no por ID | `D1_DATABASE_ID`/`R2_BUCKET_ID` dejan de ser secretos de GitHub |
| D42 | Variables de runtime no secretas en `deploy.Plan.Vars`; el `Target` las envía como bindings `plain_text` en cada deploy | `IAM_BASE_URL`, `MISITIO_BASE_URL`, etc. |

## Qué se publicó

| Repo | Tag |
|---|---|
| `webtyp/wizard` | v0.0.30 |
| `webtyp/deploy` | v0.2.111 |
| `webtyp/goflare` | v0.5.42 |
| `webtyp/github` (nuevo) | v0.0.3 |
| `webtyp/env` | v0.1.1 |
| `webtyp/keyring` | v0.2.9 |
| `webtyp/app` | v0.7.0 |
| `veltylabs/iam` | v0.5.42 (workflow con `d1-name`, D38) |

## Comprobación real (2026-10-20)

Sobre un proyecto de prueba con `config/deploy.go` (`goflare.Worker`, `github.Actions`), `app`
construyó el `Deployment` de verdad y `Check()` devolvió la lista correcta: repositorio de GitHub
(manual), workflow de despliegue (automático), workflow desactualizado (manual) y token de
arranque de Cloudflare (se escribe en el asistente). Sin fallas.

**No verificado, y por qué:** la TUI real con botones/asistentes reales (las pruebas usan una TUI
simulada), y los pasos de «Aceptación» que necesitan una cuenta de Cloudflare real, `gh auth login`
y dos personas — eso queda para el usuario, en
[DEPLOY_WIZARD_MASTER_PLAN.md](../../docs/DEPLOY_WIZARD_MASTER_PLAN.md).

## Industria — cómo despliegan otros

| Herramienta | Declaración | Secretos | Aprovisiona | Diagnóstico |
|---|---|---|---|---|
| Vercel / Netlify | detecta el framework | en la plataforma | sí (proyecto) | log del build |
| Fly.io | `fly.toml` generado por `fly launch` | `fly secrets set`, cifrados, nunca en archivo | sí | `fly doctor` |
| Kamal 2 (Rails) | `config/deploy.yml` | cofre compartido con adaptadores | no | `kamal config` |
| SST / Encore | código (`sst.config.ts`) | gestor del proveedor de nube | sí, todo | errores tipados al compilar |
| Cloudflare Wrangler | `wrangler.toml` | `wrangler secret put` | parcial | — |

Donde webtyp difiere: Cloudflare no ofrece OAuth a terceros (D10); la declaración es Go leído por
AST; el agente es un usuario de primera clase por MCP (D21).

## Lecciones

- El PR de `app` de Jules (segundo intento) no compilaba: inventó firmas de `deploy`, `goflare`,
  `github`, `keyring/auto` y `mcp` sin verificarlas. Corregido localmente contra las APIs reales.
  Regla agregada a **plan-authoring**: nombrar la firma exacta de cada API que un plan toque.
- Revisiones de PR (`wizard`, `deploy`, `goflare`, `github`, `keyring`) encontraron: `go.mod` con
  dependencias directas marcadas `indirect`, símbolos exportados solo para tests, traducciones
  `es` vacías, construcción triplicada de un mismo valor. Ver commits de cada repo.
