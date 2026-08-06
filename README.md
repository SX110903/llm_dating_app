# LLMatch v2

Reescritura segura de LLMatch como monorepo Go + React. La implementación avanza por las fases aprobadas en [PLAN.md](PLAN.md); actualmente están implementadas y validadas las **Fases 0 a 3**. El trabajo está detenido antes de la Fase 4.

## Requisitos

- Go 1.25.3+
- Node.js 24.14.0+ y pnpm 11.9.0+
- Docker Desktop con Docker Compose
- Windows PowerShell 5.1+ en Windows, u OpenSSL en Linux/macOS, para generar secretos locales

## Arranque seguro en desarrollo

No hay contraseñas ni claves predeterminadas. El script crea secretos aleatorios locales ignorados por Git:

```powershell
powershell -NoProfile -File scripts/setup-dev.ps1
docker compose up --build
```

En Linux/macOS:

```bash
./scripts/setup-dev.sh
docker compose up --build
```

La aplicación queda disponible en `http://localhost:8080`. Solo el reverse proxy publica un puerto; API, frontend, PostGIS y Redis permanecen en redes internas.

## Verificación local

`backend/test/integration` usa contenedores reales (Postgres, Redis) vía testcontainers-go, así que Docker debe estar disponible al ejecutar los tests del backend.

```powershell
cd backend
go test -race ./...

cd ../frontend
pnpm install --frozen-lockfile
pnpm audit --prod
pnpm peers check
pnpm lint
pnpm test
pnpm build

cd ..
docker compose config --quiet
```

Healthchecks:

- `GET /health/live`: vida del proceso.
- `GET /health/ready`: comprueba Postgres y Redis.
- `GET /health`: alias de readiness.

## Seguridad de la fundación

- El backend rechaza configuración incompleta y, en producción, URLs sin TLS, credenciales débiles, superusuario de Postgres, CORS comodín o claves RSA menores de 3072 bits.
- Todos los contenedores de aplicación y proxy usan usuario no-root, `no-new-privileges`, capabilities eliminadas y filesystem de solo lectura cuando es posible.
- Postgres y Redis no publican puertos al host.
- Los secretos se montan como Docker secrets y no participan en el build.
- CI fija acciones y dependencias, ejecuta lint, tests con race detector, generación `sqlc`, Gitleaks y builds.
- **La ejecución automática del CI está pausada temporalmente** por indisponibilidad de runners: el workflow conserva todas sus comprobaciones pero solo arranca a demanda (`workflow_dispatch`). Mientras dure la pausa, las validaciones de la sección [Verificación local](#verificación-local) deben ejecutarse en local antes de cada push. Para reanudarlo basta con devolver los disparadores `push` y `pull_request` a `.github/workflows/ci.yml`.
- Las imágenes base también están fijadas por digest. La configuración de producción se superpone con `docker-compose.prod.yml` y exige endpoints TLS y certificados reales declarados en `.env.example`.
- React Router sigue fuera del árbol: login, registro y la vista autenticada caben en una sola pantalla con estado local, así que se mantiene la decisión de Fase 0 hasta que una fase funcional necesite navegación real.

## Autenticación (Fase 1)

- Endpoints `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `POST /auth/logout-all` y `GET /auth/me` bajo `/api/v1`. Contrato completo en [api/openapi.yaml](api/openapi.yaml).
- Contraseñas con Argon2id (rehash transparente tras login) y JWT de acceso RS256 de 15 minutos con `sub`, `jti`, `iat`, `exp`, `iss` y `aud`.
- Refresh token opaco de 256 bits en cookie `HttpOnly`, `Secure` en producción, `SameSite=Strict` y `Path=/api/v1/auth`; rota en cada uso y revoca toda su familia si se detecta reutilización.
- El middleware de rutas autenticadas es fail-closed: si Redis no responde al comprobar el `jti`, la respuesta es `503 AUTH_DEPENDENCY_UNAVAILABLE` y el handler protegido nunca se ejecuta.
- Rate limiting distribuido en Redis por IP y por email normalizado, con backoff progresivo ante intentos repetidos en register y login.

## Perfil y fotos (Fase 2)

- Endpoints `GET/PUT /profile`, `GET/PUT /profile/preferences`, `GET/POST /profile/photos`, `GET /profile/photos/{id}/content`, `PUT /profile/photos/order`, `PUT /profile/photos/{id}/primary`, `DELETE /profile/photos/{id}` y `POST/GET/DELETE /account/consents[/{purpose}]`. Contrato completo en [api/openapi.yaml](api/openapi.yaml).
- `user_preferences.genders` es una categoría especial (RGPD art. 9): nunca se guarda sin un consentimiento activo y separado en `privacy_consents`; retirarlo borra el valor en la misma transacción que revoca el consentimiento.
- Fotos: máximo 6, JPEG/PNG/WebP hasta 10 MiB, MIME y dimensiones detectados decodificando el contenido real (nunca la extensión ni el nombre original), clave de almacenamiento basada en UUID. Storage local en desarrollo, S3 (AWS SDK v2) en producción, con compensación automática si falla la base de datos tras subir el blob.
- Borrado de foto: lógico y síncrono; la limpieza del blob es asíncrona y best-effort. Si se borra la foto principal, se promueve automáticamente otra restante.

## Discovery, swipes y matches (Fase 3)

- Endpoints autenticados `GET /discovery`, `POST /swipes`, `GET/DELETE /matches[/{matchID}]`, `POST /blocks`, `POST /reports` y `GET /matching/photos/{photoID}/content`. El contrato completo, incluidos cursores, errores y `Retry-After`, está en [api/openapi.yaml](api/openapi.yaml).
- Discovery aplica compatibilidad mutua de género y edad, distancia PostGIS, onboarding/consentimiento/foto, estado activo y exclusiones por swipe, match o bloqueo. El ranking determinista usa intereses `0.35`, cuestionario `0.30`, distancia `0.20` y actividad `0.15`; los pesos se pueden configurar por entorno.
- El like o superlike mutuo crea un único match dentro de la transacción del segundo swipe. Los pares se ordenan por UUID y las carreras concurrentes están cubiertas por pruebas de integración.
- El límite predeterminado es de 100 swipes por día UTC. Redis mantiene la reserva atómica reconciliada con PostgreSQL; una caída de Redis devuelve 503 y nunca desactiva el límite. Al agotarlo, la respuesta es 429 con los segundos restantes en `Retry-After`.
- Bloquear es idempotente, deshace el match activo y oculta perfiles y fotos en ambas direcciones. Los reportes admiten motivos cerrados y una descripción sanitizada de hasta 1000 caracteres.
- La interfaz incluye cartas con drag/spring, modal de match, listado y acciones de seguridad. Las fotos de discovery y matches se obtienen con el access token; no se exponen como objetos públicos.

Variables opcionales con valores predeterminados seguros para esta fase: `MATCHING_DAILY_SWIPE_LIMIT=100`, `MATCHING_WEIGHT_INTERESTS=0.35`, `MATCHING_WEIGHT_QUESTIONNAIRE=0.30`, `MATCHING_WEIGHT_DISTANCE=0.20` y `MATCHING_WEIGHT_ACTIVITY=0.15`.

Mensajería, historias, verificación de email y recuperación de contraseña todavía no están implementadas. La siguiente fase no se inicia sin confirmación.
