# LLMatch v2

Reescritura segura de LLMatch como monorepo Go + React. La implementación avanza por las fases aprobadas en [PLAN.md](PLAN.md); actualmente solo está implementada la **Fase 0 — Fundación**.

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
- Las imágenes base también están fijadas por digest. La configuración de producción se superpone con `docker-compose.prod.yml` y exige endpoints TLS y certificados reales declarados en `.env.example`.
- React Router queda fuera de la Fase 0 porque una única pantalla no necesita navegación y las ramas publicadas auditadas contenían avisos de seguridad; se reevaluará cuando una fase funcional lo requiera.

La autenticación y el resto de funcionalidades no pertenecen a esta fase y no se anuncian como implementadas.
