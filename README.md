# LLMatch v2

Reescritura segura de LLMatch como monorepo Go + React. La implementación avanza por las fases aprobadas en [PLAN.md](PLAN.md); las **Fases 0 a 4** están implementadas y validadas. La implementación de la Fase 9 está terminada y su cierre espera únicamente la prueba de humo obligatoria en un teléfono físico.

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

## Mensajería en tiempo real (Fase 4)

- Endpoints autenticados `GET /conversations`, `GET/POST /matches/{matchID}/messages`, `POST /matches/{matchID}/messages/read` y `POST /messaging/tickets`, más el handshake `GET /ws`. Contrato completo en [api/openapi.yaml](api/openapi.yaml).
- **Autorización por consulta, no por código.** Cada operación resuelve la pertenencia con una única sentencia que exige match activo, membresía del usuario y ausencia de bloqueo en ambas direcciones. Conversación inexistente, ajena, deshecha y bloqueada devuelven el mismo 404, de modo que sondear no permite distinguirlas.
- **Envío idempotente.** El par `(sender_id, client_nonce)` tiene índice único y la inserción usa `ON CONFLICT DO NOTHING`: reenviar tras un timeout devuelve 200 con el mensaje ya almacenado en lugar de duplicarlo. La carrera de dos envíos simultáneos con el mismo nonce está cubierta por pruebas de integración.
- **Sin claves de objeto del cliente.** El cuerpo de envío no admite `storage_key` y solo acepta `type: text`. Aceptar una clave propuesta por el cliente permitiría apuntar un mensaje a un objeto ajeno, así que los tipos con media esperan a que el backend genere la clave él mismo.
- **Historial por cursor** sobre `(created_at, id)`, nunca `OFFSET`. Los cursores son opacos, versionados y se rechazan si vienen alterados.
- **Handshake con ticket de un solo uso.** `POST /messaging/tickets` emite un ticket opaco de vida corta del que Redis solo guarda el hash; el consumo es atómico vía Lua, así que un ticket no sirve dos veces. Viaja como segunda entrada de `Sec-WebSocket-Protocol`, nunca en la URL, donde acabaría en logs, referrers e historial.
- **Fail-closed.** Sin Redis no se emiten ni se consumen tickets: la respuesta es 503 y el handshake nunca se acepta a ciegas.
- **Persistir antes de publicar.** El mensaje es duradero antes del fan-out por Pub/Sub, así que perder la publicación solo cuesta entrega en vivo; el cliente la recupera por HTTP desde su último cursor. La entrega entre instancias distintas está cubierta por pruebas de integración.
- **Contrapresión por desconexión.** Cada cliente tiene una cola acotada; si se llena, se cierra esa conexión en lugar de bloquear el hub. Un consumidor lento nunca degrada al resto.
- El socket solo acepta tramas `typing` y `ping`. Enviar mensajes va por HTTP a propósito, para mantener persistencia e idempotencia en un único sitio.
- La interfaz añade la pestaña Mensajes: lista de conversaciones con no leídos, chat con historial paginado, envío optimista con reintento que conserva el nonce, indicador de escritura y reconexión con backoff y jitter que pide un ticket nuevo en cada intento.

Variables opcionales con valores predeterminados seguros para esta fase: `MESSAGING_TICKET_TTL=30s`, `MESSAGING_RATE_LIMIT=60`, `MESSAGING_RATE_WINDOW=1m`, `MESSAGING_SOCKET_QUEUE_SIZE=64`, `MESSAGING_SOCKET_READ_LIMIT_BYTES=32768`, `MESSAGING_SOCKET_WRITE_TIMEOUT=10s`, `MESSAGING_SOCKET_PING_INTERVAL=25s`, `MESSAGING_SOCKET_READ_TIMEOUT=5m` y `MESSAGING_CLIENT_EVENTS_PER_MINUTE=120`. El arranque falla si el intervalo de ping no es menor que el timeout de lectura.

## Experiencia responsive, fotos y animación (Fase 9)

- Todos los controles interactivos tienen un área mínima de 44×44 px y un foco visible. La navegación mantiene nombres accesibles cuando las etiquetas se ocultan visualmente, y se puede recorrer y activar con teclado.
- Las vistas Descubrir, Matches, Mensajes y Perfil se verificaron a 320, 375, 768 y 1280 px. En los cuatro anchos se renderizaron sin desbordamiento horizontal ni controles visibles menores de 44×44 px; por debajo de 640 px, la navegación ocupa una segunda fila para conservar una cabecera utilizable.
- Las acciones de foto ya no dependen de `hover`: un botón permanente abre un menú táctil y de teclado para marcar principal, mover o eliminar. Las tarjetas también se reordenan por arrastre y conservan los botones de mover como alternativa accesible.
- La subida acepta clic o arrastre, abre un recorte cuadrado antes de transferir, comprime en el navegador manteniendo JPEG, PNG o WebP, muestra el progreso real de XHR y conserva el archivo preparado para reintentar un fallo. La validación de contenido del servidor permanece sin cambios y sigue siendo la autoridad.
- La carta de discovery añade profundidad e inclinación 3D durante el gesto mediante perspectiva y transformaciones existentes. No se añadió WebGL ni una dependencia de renderizado; con `prefers-reduced-motion`, las animaciones y transformaciones nuevas se desactivan.
- El presupuesto completo y el método reproducible están en [docs/phase9-performance-budget.md](docs/phase9-performance-budget.md). El build final queda en 545,97 kB de JavaScript inicial, 41,80 kB de CSS inicial y 1,77 kB gzip para el chunk diferido de preparación de fotos; la medición móvil obtiene 59,51 fps, p95 de 16,80 ms y FCP de 1.396 ms, dentro de todos los límites fijados antes del 3D.
- La suite frontend termina con 54 tests, incluyendo navegación por teclado, objetivos táctiles, arrastre y reordenación de fotos, reintento y movimiento reducido. El smoke local recorrió las cuatro vistas y completó una subida, recorte y borrado real de una foto de prueba. El único criterio aún pendiente es repetir el smoke en un teléfono físico; una emulación de viewport no lo sustituye.

Historias, LLM, RGPD, verificación de email y recuperación de contraseña todavía no están implementadas. No se inicia otra fase sin su aprobación correspondiente.
