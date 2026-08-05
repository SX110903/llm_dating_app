# syntax=docker/dockerfile:1.7
FROM golang:1.25.3-alpine AS build

WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build --chown=nonroot:nonroot /out/api /app/api
COPY --from=build --chown=nonroot:nonroot /out/healthcheck /app/healthcheck
COPY --from=build --chown=nonroot:nonroot /out/migrate /app/migrate
COPY --from=build --chown=nonroot:nonroot /src/migrations /app/migrations

USER nonroot:nonroot
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=5 CMD ["/app/healthcheck"]
ENTRYPOINT ["/app/api"]
