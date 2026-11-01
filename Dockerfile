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
# Alpine instead of distroless: the entrypoint is a small shell script.
FROM alpine:3.21
RUN addgroup -S -g 65532 nonroot \
 && adduser -S -u 65532 -G nonroot nonroot \
 && mkdir /config && chown nonroot:nonroot /config
COPY --from=backend /out/lumina /lumina
# Default config; the entrypoint copies it to /config/config.yaml if missing
# (a mounted /config hides anything baked into it).
COPY config/config.example.yaml /usr/share/lumina/config.yaml
COPY scripts/docker-entrypoint.sh /docker-entrypoint.sh
# Strip CRs in case of a Windows checkout; they would break the shebang.
RUN sed -i 's/\r$//' /docker-entrypoint.sh && chmod 755 /docker-entrypoint.sh
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/docker-entrypoint.sh"]
