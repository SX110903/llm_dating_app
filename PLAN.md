# LLMatch v2 — plan de ejecución

Estado: **aprobado el 5 de agosto de 2026**. Las Fases 0, 1, 2 y 3 están implementadas y validadas. La ejecución queda detenida antes de la Fase 4 hasta recibir una nueva confirmación.

## 1. Alcance y decisiones cerradas

- Se creará un monorepo nuevo llamado `llmatch-v2`. El ZIP de v1 se usa solo como referencia funcional; no se copiará su código, su configuración ni su falsa auditoría de seguridad.
- Backend: Go, `chi`, arquitectura hexagonal, `pgx/v5`/`pgxpool`, `sqlc`, `golang-migrate`, Postgres + PostGIS, Redis, Argon2id, JWT RS256, `validator`, `bluemonday` y `zerolog`.
- Frontend: Vite, React, TypeScript, Tailwind CSS v4, shadcn/ui, Framer Motion, TanStack Query, Zustand, React Hook Form, Zod y React Router.
- Paquetes frontend gestionados con `pnpm`; dependencias y acciones de CI quedarán fijadas a versiones concretas al iniciar la Fase 0.
- API HTTP versionada bajo `/api/v1`. Errores con un formato único: `code`, `message`, `request_id` y `details` opcional. Fechas en UTC/RFC 3339 e identificadores UUID.
- Contraseñas con Argon2id y parámetros calibrados y versionados en el propio hash PHC. Si el coste necesita cambiar, se hará rehash transparente tras un login válido.
- JWT de acceso RS256, TTL 15 minutos, con `sub`, `jti`, `iat`, `exp`, `iss` y `aud`. Las claves vendrán de archivos/secret mounts indicados por variables; nunca del repositorio.
- Refresh token opaco de 256 bits, TTL inicial de 30 días, rotación en cada uso y detección de reutilización por familia. Se almacenará únicamente su SHA-256: al tener 256 bits aleatorios permite búsqueda indexada sin hacer viable fuerza bruta.
- El refresh token viajará solo en cookie `HttpOnly`, `Secure` en producción, `SameSite=Strict` y `Path=/api/v1/auth`. `refresh` y `logout` validarán además `Origin` contra la allowlist.
- El access token vivirá solo en memoria en el frontend. Recargar la página provocará un refresh silencioso; no se usará `localStorage` ni `sessionStorage` para credenciales.
- Redis tendrá funciones reales desde Fase 1: rate limiting distribuido, backoff de login, denylist de `jti` hasta su expiración, registro temporal de los `jti` activos por usuario y tickets WebSocket de un solo uso.
- **Decisión explícita de disponibilidad/seguridad:** Redis es una dependencia dura del camino caliente de autenticación. En **cada request autenticada**, el middleware valida primero firma/claims del JWT y consulta después en Redis el `jti`. Un `jti` revocado devuelve 401; timeout, error o desconexión de Redis devuelve 503 y el handler no se ejecuta. No se limita esta comprobación a login/refresh/logout. Se acepta conscientemente la pérdida de disponibilidad porque la prioridad es no aceptar un token cuya revocación no pueda comprobarse. `/health/ready` también permanecerá fallido mientras Redis no sea fiable.
- La conexión WebSocket del navegador no pondrá credenciales en la URL. Un request HTTP autenticado emitirá un ticket aleatorio de un solo uso, con TTL de 30 segundos en Redis; el ticket se enviará en `Sec-WebSocket-Protocol` durante el handshake.
- Perfiles y mensajes serán texto plano sanitizado. No habrá `dangerouslySetInnerHTML` con datos de usuario.
- El almacenamiento local será el adaptador de desarrollo; S3 con AWS SDK v2 será el adaptador de producción. La base guardará claves internas de objeto, no URLs aportadas por el cliente.
- La Fase 6 (LLM real) queda desactivada hasta una confirmación expresa.

## 2. Estructura exacta del monorepo

```text
llmatch-v2/
├── .github/
│   └── workflows/
│       └── ci.yml
├── api/
│   └── openapi.yaml
├── backend/
│   ├── cmd/
│   │   ├── api/main.go
│   │   └── migrate/main.go
│   ├── internal/
│   │   ├── domain/
│   │   │   ├── user/{entity.go,errors.go,repository.go}
│   │   │   ├── session/{entity.go,errors.go,repository.go}
│   │   │   ├── account/{token.go,consent.go,errors.go,repository.go}
│   │   │   ├── profile/{entity.go,errors.go,repository.go,storage.go}
│   │   │   ├── matching/{entity.go,errors.go,repository.go}
│   │   │   ├── messaging/{entity.go,errors.go,repository.go}
│   │   │   └── story/{entity.go,errors.go,repository.go}
│   │   ├── application/
│   │   │   ├── auth/{service.go,ports.go}
│   │   │   ├── account/{recovery.go,verification.go,privacy.go,ports.go}
│   │   │   ├── profile/{service.go,ports.go}
│   │   │   ├── matching/{service.go,ports.go}
│   │   │   ├── messaging/{service.go,ports.go}
│   │   │   └── stories/{service.go,ports.go}
│   │   ├── adapters/
│   │   │   ├── http/
│   │   │   │   ├── router.go
│   │   │   │   ├── response.go
│   │   │   │   ├── auth/{handler.go,dto.go}
│   │   │   │   ├── account/{handler.go,dto.go}
│   │   │   │   ├── profile/{handler.go,dto.go}
│   │   │   │   ├── matching/{handler.go,dto.go}
│   │   │   │   ├── messaging/{handler.go,dto.go}
│   │   │   │   └── stories/{handler.go,dto.go}
│   │   │   ├── postgres/
│   │   │   │   ├── pool.go
│   │   │   │   ├── repositories/
│   │   │   │   ├── queries/{users.sql,sessions.sql,account_tokens.sql,privacy.sql,profiles.sql,matching.sql,messages.sql,stories.sql}
│   │   │   │   └── sqlc/                 # generado; solo vive aquí el código ligado a pgx
│   │   │   ├── redis/{client.go,rate_limiter.go,token_denylist.go,ws_tickets.go}
│   │   │   ├── email/{sender.go,smtp.go}
│   │   │   ├── storage/{storage.go,local.go,s3.go}
│   │   │   └── websocket/{handler.go,hub.go,client.go,events.go}
│   │   └── platform/
│   │       ├── config/{config.go,validate.go}
│   │       ├── logger/logger.go
│   │       ├── crypto/{argon2id.go,jwt.go,tokens.go}
│   │       └── middleware/{auth.go,request_id.go,logging.go,recovery.go,security.go,cors.go}
│   ├── migrations/
│   │   ├── 000001_extensions.up.sql
│   │   ├── 000001_extensions.down.sql
│   │   ├── 000002_auth.up.sql
│   │   ├── 000002_auth.down.sql
│   │   ├── 000003_profiles.up.sql
│   │   ├── 000003_profiles.down.sql
│   │   ├── 000004_matching.up.sql
│   │   ├── 000004_matching.down.sql
│   │   ├── 000005_messaging.up.sql
│   │   ├── 000005_messaging.down.sql
│   │   ├── 000006_stories.up.sql
│   │   ├── 000006_stories.down.sql
│   │   ├── 000007_privacy.up.sql
│   │   ├── 000007_privacy.down.sql
│   │   ├── 000008_account_recovery.up.sql
│   │   └── 000008_account_recovery.down.sql
│   ├── test/
│   │   ├── integration/{postgres_test.go,auth_test.go}
│   │   └── fixtures/
│   ├── .golangci.yml
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   └── sqlc.yaml
├── frontend/
│   ├── public/
│   ├── src/
│   │   ├── app/{App.tsx,router.tsx,providers.tsx,layout.tsx}
│   │   ├── features/
│   │   │   ├── auth/{api,components,hooks,schemas,types.ts}
│   │   │   ├── account/{api,components,hooks,schemas,types.ts}
│   │   │   ├── profile/{api,components,hooks,schemas,types.ts}
│   │   │   ├── swipe/{api,components,hooks,types.ts}
│   │   │   ├── matches/{api,components,hooks,types.ts}
│   │   │   ├── chat/{api,components,hooks,types.ts}
│   │   │   └── stories/{api,components,hooks,types.ts}
│   │   ├── shared/
│   │   │   ├── components/ui/
│   │   │   ├── hooks/
│   │   │   ├── lib/{api-client.ts,query-client.ts,env.ts,errors.ts,utils.ts}
│   │   │   ├── state/auth-store.ts
│   │   │   └── animations/
│   │   ├── styles/index.css
│   │   ├── test/{setup.ts,mocks.ts}
│   │   └── main.tsx
│   ├── Dockerfile
│   ├── components.json
│   ├── eslint.config.js
│   ├── index.html
│   ├── package.json
│   ├── pnpm-lock.yaml
│   ├── tsconfig.json
│   ├── vite.config.ts
│   └── vitest.config.ts
├── reverse-proxy/
│   ├── nginx.conf
│   ├── conf.d/{dev.conf,prod.conf}
│   └── Dockerfile
├── scripts/
│   ├── generate-dev-keys.ps1
│   └── generate-dev-keys.sh
├── secrets/
│   └── .gitkeep                    # los secretos generados están ignorados
├── .dockerignore
├── .env.example
├── .gitignore
├── docker-compose.yml
├── docker-compose.prod.yml
├── Makefile
├── PLAN.md
└── README.md
```

Regla comprobable: los paquetes de `domain` no importarán `chi`, `pgx`, Redis, JWT ni tipos HTTP; `application` solo dependerá de `domain` y de la biblioteca estándar. Los adaptadores traducirán DTO ↔ dominio y filas `sqlc` ↔ dominio.

## 3. Esquema de base de datos inicial

La extensión y cada grupo funcional se desplegarán en la fase correspondiente; este es el diseño objetivo inicial completo. Se usarán `pgcrypto`, `citext` y `postgis`, `gen_random_uuid()`, claves foráneas explícitas y timestamps `timestamptz`.

### `users` — Fase 1

- Columnas: `id uuid PK`, `email citext NOT NULL`, `password_hash text NOT NULL`, `display_name varchar(100)`, `birth_date date`, `gender text`, `status text NOT NULL DEFAULT 'active'`, `email_verified_at timestamptz NULL`, `password_changed_at timestamptz NOT NULL`, `created_at`, `updated_at`.
- Checks: `status IN ('active','suspended','deleted')`. La edad mínima se valida en el caso de uso usando `birth_date`; nunca se acepta una edad calculada por el cliente.
- Índices: `users_email_uq UNIQUE(email)`, `users_status_idx(status)` y `users_discover_idx(gender,birth_date) WHERE status = 'active'`.

### `refresh_tokens` — Fase 1

- Columnas: `id uuid PK`, `user_id FK users`, `family_id uuid`, `token_hash bytea`, `replaced_by uuid NULL FK refresh_tokens`, `device_label text NULL`, `user_agent text NULL`, `ip inet NULL`, `expires_at`, `last_used_at NULL`, `revoked_at NULL`, `revoke_reason text NULL`, `created_at`.
- Índices: `refresh_tokens_hash_uq UNIQUE(token_hash)`, `refresh_tokens_user_active_idx(user_id, revoked_at, expires_at)` y `refresh_tokens_family_idx(family_id)`.
- Regla: la rotación se ejecutará en una transacción con bloqueo de la fila. Reutilizar un token ya rotado revocará toda su familia.

### `email_verification_tokens` y `password_reset_tokens` — Fase 8

- Ambas tablas: `id uuid PK`, `user_id FK users`, `token_hash bytea`, `expires_at`, `used_at NULL`, `revoked_at NULL`, `created_at`.
- Índices por tabla: `*_token_hash_uq UNIQUE(token_hash)` y `*_user_active_idx(user_id,used_at,revoked_at,expires_at)`.
- Reglas: tokens opacos aleatorios de 256 bits, solo se persiste SHA-256, un único uso y revocación de tokens anteriores al volver a solicitar. TTL de 15 minutos para reset y 24 horas para verificación. Nunca se registran en logs, métricas ni trazas.

### `profiles` — Fase 2

- Columnas: `user_id uuid PK/FK users`, `bio varchar(500)`, `interests text[]`, `city varchar(120)`, `location geography(Point,4326) NULL`, `questionnaire jsonb NOT NULL DEFAULT '{}'`, `onboarding_completed bool`, `created_at`, `updated_at`.
- Reglas: coordenadas válidas y contenido validado/sanitizado por dominio y DTO. `display_name`, `birth_date` y `gender` viven en `users` porque son datos obligatorios del registro; no se duplican aquí.
- Índices: `profiles_location_gix USING GIST(location)`, `profiles_onboarding_idx(user_id) WHERE onboarding_completed` y `profiles_interests_gin USING GIN(interests)`.

### `user_preferences` — Fase 2

- Columnas: `user_id uuid PK/FK users`, `min_age smallint`, `max_age smallint`, `max_distance_km smallint`, `genders text[]`, `created_at`, `updated_at`.
- Checks: `18 <= min_age <= max_age <= 100` y `1 <= max_distance_km <= 500`.
- Índice: la PK cubre la lectura por usuario; no se añade un índice sin una consulta que lo justifique.
- **RGPD art. 9:** `genders` se tratará como dato de categoría especial porque revela de facto orientación/preferencia sexual. Exige consentimiento explícito, granular y versionado antes de persistirse; queda excluido de logs, analítica, trazas y datasets de prueba. Retirar el consentimiento borra el valor y desactiva discovery hasta que el usuario configure una preferencia válida con nuevo consentimiento.

### `privacy_consents` — Fase 2

- Columnas: `id uuid PK`, `user_id FK users`, `purpose text`, `policy_version text`, `granted_at`, `withdrawn_at NULL`, `source text`, `created_at`.
- Índices: `privacy_consents_user_purpose_idx(user_id,purpose,granted_at DESC)` y restricción que impide más de un consentimiento vigente por usuario/propósito.
- Propósito inicial: `matching_gender_preferences`. El registro de consentimiento no almacena el valor sensible; solo evidencia la acción, finalidad y versión informada. Se crea junto a `user_preferences`, no se pospone hasta Fase 7; esa fase audita y completa los derechos de acceso, portabilidad y borrado.

### `data_export_requests` y `account_deletion_jobs` — Fase 7

- `data_export_requests`: `id uuid PK`, `user_id FK users`, `status`, `storage_key NULL`, `requested_at`, `completed_at NULL`, `expires_at NULL`, `failure_code NULL`; índice `data_export_user_idx(user_id,requested_at DESC)` y checks de estado.
- `account_deletion_jobs`: `id uuid PK`, `user_id uuid NULL`, `status`, `requested_at`, `started_at NULL`, `completed_at NULL`, `failure_code NULL`; índice `account_deletion_status_idx(status,requested_at)`. Al concluir, se elimina la vinculación con el usuario y solo queda evidencia operacional no identificable con retención corta.
- Los artefactos de exportación se cifran, caducan y son eliminados automáticamente; nunca se incluyen en backups de larga retención.

### `photos` — Fase 2

- Columnas: `id uuid PK`, `user_id FK users`, `storage_key text`, `mime_type text`, `byte_size bigint`, `width int`, `height int`, `position smallint`, `is_primary bool`, `created_at`, `deleted_at NULL`.
- Checks: tamaño positivo, dimensiones positivas, posición entre 0 y 5 y MIME permitido.
- Índices: `photos_user_position_uq UNIQUE(user_id,position) WHERE deleted_at IS NULL`, `photos_user_primary_uq UNIQUE(user_id) WHERE is_primary AND deleted_at IS NULL` y `photos_storage_key_uq UNIQUE(storage_key)`.

### `swipes` — Fase 3

- Columnas: `id uuid PK`, `actor_id FK users`, `target_id FK users`, `action text`, `created_at`.
- Checks: `actor_id <> target_id` y `action IN ('like','dislike','superlike')`.
- Índices: `swipes_pair_uq UNIQUE(actor_id,target_id)`, `swipes_actor_daily_idx(actor_id,created_at DESC)` y `swipes_target_like_idx(target_id,action,created_at DESC)`.

### `matches` — Fase 3

- Columnas: `id uuid PK`, `user_low_id FK users`, `user_high_id FK users`, `matched_at`, `unmatched_at NULL`, `unmatched_by NULL FK users`.
- Reglas: `user_low_id < user_high_id`; ambos valores se ordenan antes de persistir. El match se crea en la misma transacción que confirma el like mutuo y la unicidad lo hace idempotente.
- Índices: `matches_pair_uq UNIQUE(user_low_id,user_high_id)`, `matches_low_active_idx(user_low_id,matched_at DESC) WHERE unmatched_at IS NULL` y `matches_high_active_idx(user_high_id,matched_at DESC) WHERE unmatched_at IS NULL`.

### `messages` — Fase 4

- Columnas: `id uuid PK`, `match_id FK matches`, `sender_id FK users`, `client_nonce uuid`, `type text`, `content varchar(2000) NULL`, `storage_key text NULL`, `read_at NULL`, `created_at`, `deleted_at NULL`.
- Checks: `type IN ('text','image','gif')` y coherencia entre `type`, `content` y `storage_key`.
- Índices: `messages_sender_nonce_uq UNIQUE(sender_id,client_nonce)` para idempotencia, `messages_match_cursor_idx(match_id,created_at DESC,id DESC) WHERE deleted_at IS NULL` y `messages_unread_idx(match_id,read_at) WHERE read_at IS NULL AND deleted_at IS NULL`.
- No se guarda `receiver_id`: se deriva de los dos participantes del match y así no puede contradecirlo. La paginación será por cursor `(created_at,id)`, no por `OFFSET`.

### `stories` — Fase 5

- Columnas: `id uuid PK`, `user_id FK users`, `storage_key text`, `media_type text`, `mime_type text`, `byte_size bigint`, `duration_seconds smallint`, `caption varchar(200) NULL`, `expires_at`, `created_at`, `deleted_at NULL`.
- Checks: `media_type IN ('image','video')`, límites de tamaño/duración y `expires_at > created_at`.
- Índices: `stories_feed_idx(expires_at DESC,created_at DESC) WHERE deleted_at IS NULL`, `stories_user_active_idx(user_id,expires_at DESC) WHERE deleted_at IS NULL` y `stories_storage_key_uq UNIQUE(storage_key)`.

### `story_views` — Fase 5

- Columnas: `story_id FK stories`, `viewer_id FK users`, `viewed_at`; PK compuesta `(story_id,viewer_id)`.
- Índice adicional: `story_views_viewer_idx(viewer_id,viewed_at DESC)`. El contador se calcula con `COUNT(*)` o se cachea; no se mantendrá un contador duplicado susceptible de desincronizarse.

### Seguridad de usuario: `blocks` y `reports` — Fases 3 y 7

- `blocks`: `blocker_id`, `blocked_id`, `created_at`, PK `(blocker_id,blocked_id)`, check de usuarios distintos e índice inverso `blocks_blocked_idx(blocked_id,blocker_id)`. Bloquear excluye discovery y corta interacción/mensajería en ambas direcciones.
- `reports`: `id`, `reporter_id`, `reported_id`, `reason`, `description`, `status`, `created_at`, `resolved_at`; checks de usuarios distintos y estados permitidos; índices `reports_status_idx(status,created_at)` y `reports_reporter_target_idx(reporter_id,reported_id,created_at DESC)`.

La aplicación usará un rol de base de datos con privilegios mínimos y sin DDL. Un proceso de migración separado usará otro rol. `DATABASE_URL` y `MIGRATIONS_DATABASE_URL` serán obligatorias y no tendrán fallback.

## 4. Contratos y criterios técnicos por fase

### Fase 0 — Fundación

- Crear el monorepo, el wiring mínimo hexagonal, Vite/React, proxy y Dockerfiles multi-stage.
- `docker-compose.yml`: proxy como único servicio con puerto publicado; backend, frontend, PostGIS y Redis solo en redes internas. En desarrollo el proxy publicará `127.0.0.1:8080`; en producción se mapearán 80/443 al proxy no-root.
- Contenedores con usuario no-root explícito, `no-new-privileges`, healthchecks y filesystem read-only cuando sea compatible. Volúmenes separados para Postgres, Redis y media local.
- Config tipada y validada: en producción exige URLs de DB/Redis, claves RSA válidas, orígenes CORS concretos y secretos con requisitos mínimos. Las interpolaciones de Compose usarán forma obligatoria `${VAR:?mensaje}`, nunca `:-default` para secretos.
- Endpoints `GET /health/live`, `GET /health/ready` y alias `GET /health`; readiness comprobará Postgres y Redis con timeout.
- CI inicial: formato/lint, `go test -race`, TypeScript/ESLint/Vitest, validación de `sqlc` y build de ambos proyectos. Gitleaks se activa desde el primer commit.
- Tests mínimos: config rechaza producción sin claves/secretos, health responde correctamente y smoke test del frontend.
- Terminado: builds reproducibles, CI verde y `docker compose up --build` saludable sin publicar Postgres/Redis.

### Fase 1 — Auth

- Endpoints: `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `POST /auth/logout-all`, `GET /auth/me`.
- Registro transaccional con email, contraseña, nombre visible, fecha de nacimiento y género; email normalizado/case-insensitive, política de contraseña, edad mínima y respuesta genérica donde sea necesario para evitar enumeración.
- Login con rate limit Redis por IP + email normalizado (5 intentos/15 min), backoff progresivo, comparación Argon2id y logs sin credenciales.
- El middleware de **todas** las rutas autenticadas ejecutará esta secuencia: extraer Bearer → validar RS256, `iss`, `aud`, `exp` y claims → consultar `denylist:jti:<jti>` en Redis con timeout estricto → adjuntar la identidad al contexto. Token revocado: 401. Redis caído, timeout o respuesta inválida: 503 `AUTH_DEPENDENCY_UNAVAILABLE`, sin llegar al caso de uso. Un circuit breaker podrá reducir latencia durante una caída, pero siempre seguirá fail-closed.
- Cada emisión de access token registra su `jti` y expiración en un conjunto temporal por usuario. `logout` añade el `jti` actual al denylist; `logout-all` revoca todas las familias de refresh y, mediante una operación Lua en Redis, añade todos los `jti` todavía activos del usuario. Las claves tienen TTL hasta `exp`, Redis de producción usa persistencia AOF y una instancia que no haya recuperado el estado de revocación no pasa readiness.
- Los endpoints públicos que dependen de controles Redis tampoco degradan a modo inseguro: login/register devuelven 503 si no pueden aplicar rate limit/backoff, y refresh/logout devuelven 503 si no pueden comprobar o materializar revocaciones.
- Rotación/revocación real de refresh y denylist inmediata del access `jti`. Cambio de contraseña, recuperación y verificación quedan expresamente diferidos a la Fase 8; `email_verified_at` permanecerá nulo y no se anunciará email verificado antes de esa fase.
- Frontend de login/registro, Zod, access token en Zustand solo en memoria, cola de un único refresh ante 401 para evitar carreras y logout seguro.
- Tests obligatorios indicados en el prompt más casos de carrera de rotación, cookie segura, claim/algoritmo JWT incorrecto, rate limiting distribuido y una prueba de middleware que demuestra que Redis caído produce 503 y nunca ejecuta el handler protegido.

### Fase 2 — Perfil y fotos

- CRUD de perfil/preferencias y endpoints de crear, listar, reordenar, marcar principal y borrar fotos.
- El onboarding muestra una finalidad separada para `user_preferences.genders` y persiste el consentimiento explícito en `privacy_consents` antes de guardar la preferencia. Sin consentimiento no se guarda el dato ni se habilita discovery; retirarlo borra el valor de forma transaccional.
- Máximo 6 fotos. Imágenes JPEG/PNG/WebP, máximo 10 MiB; MIME detectado desde contenido, decodificación real para validar formato/dimensiones y UUID como clave. El original del usuario nunca forma parte de la ruta.
- Strategy local/S3, compensación si DB o storage falla, y borrado lógico + limpieza asíncrona segura.
- Frontend de onboarding/perfil con preview local, progreso, estados de error y animaciones.
- Tests unitarios, integración Postgres y pruebas de consentimiento obligatorio/retirada, magic bytes falsos, exceso de tamaño, path traversal y consistencia foto principal.

### Fase 3 — Discover, swipe y matches

- **Estado: completada y validada el 6 de agosto de 2026.** No inicia trabajo de la Fase 4.
- Filtros duros: estado activo, onboarding completo, edad, compatibilidad de género mutua, distancia PostGIS, no visto, no bloqueado y no emparejado.
- Ranking determinista y auditable implementado con pesos configurables: intereses compartidos `0.35`, afinidad del cuestionario `0.30`, distancia `0.20` y actividad reciente `0.15`. Los empates se resuelven de forma estable y el cursor opaco conserva la instantánea temporal del ranking.
- Swipe idempotente; like/superlike mutuo crea un único match en la misma transacción, con el par de UUID ordenado y bloqueo transaccional para carreras concurrentes.
- Límite predeterminado de 100 swipes por día UTC. Redis reserva capacidad mediante una operación atómica y PostgreSQL reconcilia el contador persistido; si Redis no está disponible, el swipe falla con 503 sin saltarse el límite. Al agotarse, la API responde 429 con `Retry-After` hasta el siguiente día UTC.
- APIs implementadas: discovery y matches paginados por cursor, swipe, unmatch, block, report y lectura autenticada de fotos visibles. Un bloqueo oculta a ambas personas en discovery, invalida el acceso a sus fotos desde matching y deshace cualquier match activo entre ellas.
- Frontend implementado con cartas drag/spring, modal de match persistente durante la invalidación, listado de matches y acciones de deshacer, bloquear y reportar. Las fotos se descargan con Bearer token y sus URLs de objeto se liberan sin romper el remount de React `StrictMode`.
- Validación completada con unitarios de aplicación, integración contra Postgres/PostGIS y Redis reales, carreras de likes, ranking con fixtures, cursores, exclusión por distancia, fotos protegidas, bloqueos bidireccionales, límite diario, autorización e idempotencia de unmatch y sanitización de reportes. La interfaz se verificó también en escritorio y viewport móvil.

### Fase 4 — Mensajería en tiempo real

- HTTP: historial por cursor, envío idempotente, marcar leído y crear ticket WebSocket.
- WebSocket: ticket Redis de un solo uso, autorización por pertenencia a match, límites de tamaño/frecuencia, ping/pong, cierre limpio y backpressure por cliente.
- Persistir antes de publicar; Redis Pub/Sub permitirá varias réplicas del backend. La reconexión obtiene mensajes posteriores al último cursor confirmado.
- Frontend con reconexión exponencial con jitter, estados de entrega y deduplicación por `client_nonce`; typing será efímero y no se persistirá.
- Tests de ticket reutilizado/caducado, acceso a match ajeno, orden/deduplicación y dos instancias comunicadas por Redis.

### Fase 5 — Historias

- Crear, listar feed, marcar vista y consultar vistas propias. Expiración lógica por `expires_at`; un job idempotente elimina media caducada.
- Imágenes con reglas de Fase 2; vídeo MP4/WebM hasta 50 MiB y 30 segundos, validado por contenido y metadatos, no por extensión.
- Feed restringido a matches activos y respetando bloqueos.
- Visor frontend con progreso animado, pausa/reanudación, teclado y preferencias de movimiento reducido.
- Tests de expiración, permisos, vista idempotente, límites de media y job repetible.

### Fase 6 — LLM opcional

- **Gate obligatorio de producto y proveedor.** No se implementará sin aprobación posterior.
- Si se aprueba, el proveedor estará detrás de un puerto de dominio; la API key solo en backend, límites Redis por usuario, timeouts, presupuesto, moderación y política de privacidad/retención explícita.

### Fase 7 — Endurecimiento, privacidad y RGPD

- Nginx con CSP sin `unsafe-inline`, HSTS solo en producción HTTPS, `frame-ancestors 'none'`, `nosniff`, `Referrer-Policy` y `Permissions-Policy` mínima.
- Compose de producción con 80/443 únicamente en el proxy, TLS desde secretos/mounts, imágenes no-root, read-only, capabilities eliminadas y límites de recursos.
- CI completa: `golangci-lint`, `go test -race -cover`, Vitest, `gosec`, Gitleaks, auditoría pnpm, Trivy de filesystem e imágenes y builds.
- Checklist OWASP Top 10 con evidencia enlazada a tests/comandos. ZAP baseline contra un entorno efímero. El resultado documentará hallazgos y límites; no afirmará “seguro para producción” sin evidencia.
- `user_preferences.genders` se clasifica y trata como **dato de categoría especial a efectos del RGPD art. 9**. Antes de guardarlo se obtiene consentimiento explícito, separado, informado, versionado y revocable. Su finalidad queda limitada al matching; no se copia a logs, analítica, trazas, entrenamiento, LLM ni entornos de prueba. La retirada del consentimiento elimina el valor sensible.
- Portabilidad/acceso: `POST /api/v1/account/export` inicia una exportación autenticada y con rate limit; estado y descarga requieren reautenticación. El resultado será JSON legible por máquina más media propia en ZIP, con enlace firmado de TTL corto. Incluirá cuenta, perfil, preferencias y consentimientos, fotos/historias propias, swipes, matches y mensajes correspondientes al usuario, minimizando PII de terceros.
- Borrado real: `DELETE /api/v1/account` exige reautenticación y confirmación explícita. Revoca refresh tokens y todos los access `jti`, borra claves de storage, tokens, preferencias —incluido `genders`—, perfil y PII. El contenido compartido imprescindible para derechos de otros usuarios se redacta y sus referencias se sustituyen por un identificador irreversible sin tabla de correspondencia; cualquier conservación de reportes exige base legal y plazo documentados.
- `users.status = 'deleted'` solo podrá ser un estado transitorio que bloquee el acceso durante el job: no es el resultado final. La eliminación/anonimización en base operativa y object storage tendrá SLA máximo de 24 horas; los backups cifrados expiran en un máximo de 30 días y no podrán restaurar la cuenta borrada como activa.
- Tests RGPD: no se persiste `genders` sin consentimiento vigente, retirarlo limpia el dato, la exportación contiene solo el ámbito permitido y el job de borrado es idempotente y deja cero PII/sensibles rastreables en almacenamiento operativo.
- Runbook de backups/restauración, rotación de claves, migraciones, rollback y respuesta a incidentes.

### Fase 8 — Verificación de email y recuperación de contraseña

- Endpoints: `POST /auth/email-verification/request`, `POST /auth/email-verification/confirm`, `POST /auth/password-reset/request` y `POST /auth/password-reset/confirm`.
- Las solicitudes responden de forma indistinguible exista o no el email y tienen rate limiting Redis por IP+email. El adaptador de correo recibe plantillas sin secretos; tokens y enlaces completos quedan excluidos de logs y telemetría.
- Verificación: token opaco de un solo uso, TTL 24 horas y reenvío que revoca los anteriores. La confirmación bloquea la fila del token, rellena `users.email_verified_at` y consume el token en una sola transacción. Tras esta fase, una cuenta no verificada puede iniciar sesión y gestionar/verificar su cuenta, pero no usar discovery, swipe, chat ni publicar media.
- Recuperación: token opaco de un solo uso con TTL 15 minutos. La confirmación adquiere un lock de autenticación por usuario en Redis para impedir login/refresh concurrente, valida el token, añade mediante Lua todos sus access `jti` activos al denylist y después actualiza el hash Argon2id, `password_changed_at`, consume el token y revoca todas las familias de refresh en una transacción. Si Redis no puede completar la invalidación, el cambio no se aplica y se devuelve 503; una invalidación Redis seguida de fallo DB puede cerrar sesiones de más, pero nunca deja una sesión antigua válida.
- Frontend: pantallas de solicitar/confirmar verificación y reset, estados genéricos contra enumeración y validación Zod de la nueva contraseña.
- Tests: token usado, revocado, caducado o alterado; solicitudes sin enumeración; carrera de dos confirmaciones; verificación idempotente; y prueba end-to-end de que, tras reset, fallan inmediatamente todos los access y refresh tokens anteriores.

## 5. Estrategia de pruebas y commits

- Unitarios junto al paquete (`*_test.go`, `*.test.tsx`) y dobles escritos contra puertos del dominio.
- Integración en `backend/test/integration` con `testcontainers-go` para PostGIS y Redis reales; handlers con `httptest`.
- Frontend con Vitest + Testing Library. Playwright se añadirá para login, swipe→match y chat cuando existan esos flujos.
- Cada migración tendrá `up`/`down` y prueba de migrar desde cero. `sqlc generate` no podrá dejar diffs sin commitear.
- Umbral inicial de cobertura: 80% en `application` y código crítico de auth; la cobertura global será informativa y nunca sustituirá los casos de seguridad.
- Commits atómicos por capacidad: scaffolding, infraestructura, configuración/health, CI y luego dominio→aplicación→adaptadores→UI/tests por cada fase. No se mezclará una fase nueva con arreglos pendientes de la anterior.

## 6. Decisiones menores asumidas

- Solo email/contraseña en v2 inicial; los botones sociales decorativos de v1 no se trasladan.
- Usuarios de 18 años o más.
- API y eventos internos en inglés; interfaz de usuario inicialmente en español y preparada para i18n.
- No se reutilizan las imágenes demo del ZIP por derechos/procedencia no acreditados; tests usarán fixtures generados.
- Observabilidad inicial con logs JSON, `request_id` y métricas básicas; trazas distribuidas quedan fuera salvo necesidad demostrada.
- Desarrollo local usa HTTP en loopback; cookies `Secure` y TLS son obligatorias en producción. No se relajará `SameSite=Strict`.
- Verificación de email y recuperación de contraseña sí forman parte del alcance obligatorio, en la Fase 8; no quedan como funcionalidad implícita ni futura sin asignar.

## 7. Puerta de aprobación

Tras aprobar este documento se iniciará únicamente la **Fase 0**. Al terminar se entregará un resumen breve, evidencia de pruebas y el listado de decisiones nuevas, y se esperará revisión antes de avanzar a Fase 1. La Fase 6 requerirá una segunda aprobación específica aunque las fases anteriores estén aceptadas; las Fases 7 y 8 son obligatorias para considerar terminado el alcance actual.
