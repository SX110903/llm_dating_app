.PHONY: setup dev down logs backend-test frontend-test verify sqlc

setup:
	pwsh -NoProfile -File scripts/setup-dev.ps1

dev:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f

backend-test:
	cd backend && go test -race ./...

frontend-test:
	cd frontend && pnpm lint && pnpm test && pnpm build

sqlc:
	cd backend && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

verify: backend-test frontend-test sqlc
	docker compose config --quiet

