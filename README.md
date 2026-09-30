# Movank — prueba técnica full stack

Un servicio Go y un frontend SvelteKit. PostgreSQL será la fuente de verdad;
Redis será una caché reconstruible. El backend alojará el worker de outbox y
distribuirá actualizaciones del dashboard por SSE. La PWA persistirá en IndexedDB
y sincronizará mediante un Web Worker.

## Estado actual: etapa 1

Solo está creada la estructura inicial y documentado el alcance. Todavía no hay
aplicación ejecutable, dependencias, contenedores, migraciones ni endpoints.
No se presentan comandos de arranque como si ya funcionaran. Cada etapa añadirá
sus comandos reales y verificaciones; el usuario revisa y realiza los commits.

```text
backend/
  cmd/api/         Entrada del único proceso Go
  internal/        Implementación privada del servicio
  migrations/      Evolución del esquema PostgreSQL
frontend/
  src/             Aplicación SvelteKit y workers
  static/          Recursos públicos e instalación PWA
infra/             Configuración de contenedores y desarrollo
docs/
  api/             Contrato OpenAPI, a incorporar con la API
  decisions/       Decisiones y limitaciones
  requirements.md  Requisitos del PDF y criterios de aceptación
  roadmap.md       Etapas y verificaciones previstas
tests/integration/ Pruebas entre componentes reales
```

## Cómo revisar esta etapa

Desde la raíz, en PowerShell:

```powershell
Get-ChildItem -Recurse -File -Force | Where-Object FullName -NotMatch '[\\/](\.git|\.scratch)[\\/]'
Get-Content README.md
Get-Content docs/requirements.md
Get-Content docs/decisions/0001-project-boundaries.md
```

Se verifica la organización y el alcance documental. No hay lógica sobre la que
ejecutar tests todavía. Las herramientas temporales de lectura del PDF están
excluidas mediante `.scratch/` y no son dependencias del proyecto.

Consulta [los requisitos](docs/requirements.md), [las decisiones iniciales](docs/decisions/0001-project-boundaries.md)
y [el plan por etapas](docs/roadmap.md). Las decisiones indican explícitamente
qué comportamiento está previsto y aún no implementado.
