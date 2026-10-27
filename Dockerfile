# syntax=docker/dockerfile:1

# --- frontend -----------------------------------------------------------
FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
# Vite writes into ../backend/internal/web/dist (embedded by the Go build).
RUN npm run build

# --- backend ------------------------------------------------------------
FROM golang:1.24-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /src/backend/internal/web/dist ./internal/web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/lumina ./cmd/lumina

# --- runtime ------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /out/lumina /lumina
COPY config/config.example.yaml /config/config.yaml
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/lumina"]
