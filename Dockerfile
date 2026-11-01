# syntax=docker/dockerfile:1

# Build stages run on the builder's native platform ($BUILDPLATFORM) and
# cross-compile, so nothing is emulated for the arm64 image.

# --- types --------------------------------------------------------------
# The *.gen.ts files are not committed: generate them from the Go models on
# every build (tygo writes to ../frontend/src/**/*.gen.ts).
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS types
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN mkdir -p /src/frontend && go tool tygo generate

# --- frontend -----------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:26-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
COPY --from=types /src/frontend/src/ ./src/
# Vite writes into ../backend/internal/web/dist (embedded by the Go build).
RUN npm run build

# --- backend ------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /src/backend/internal/web/dist ./internal/web/dist
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/lumina ./cmd/lumina

# --- prep (native platform, files only) ---------------------------------
# Everything that needs a RUN happens here, so the runtime stage below has no
# RUN step and never executes under emulation.
FROM --platform=$BUILDPLATFORM alpine:3.24 AS prep
COPY scripts/docker-entrypoint.sh /docker-entrypoint.sh
# Strip CRs in case of a Windows checkout; they would break the shebang.
RUN sed -i 's/\r$//' /docker-entrypoint.sh && chmod 755 /docker-entrypoint.sh \
 && mkdir /config

# --- runtime ------------------------------------------------------------
# Alpine instead of distroless: the entrypoint is a small shell script.
FROM alpine:3.24
COPY --from=backend /out/lumina /lumina
# Default config; the entrypoint copies it to /config/config.yaml if missing
# (a mounted /config hides anything baked into it).
COPY config/config.example.yaml /usr/share/lumina/config.yaml
COPY --from=prep /docker-entrypoint.sh /docker-entrypoint.sh
COPY --from=prep --chown=65532:65532 /config /config
EXPOSE 8080
# Numeric uid/gid: no passwd entry needed, so no RUN (adduser) in this stage.
USER 65532:65532
ENTRYPOINT ["/docker-entrypoint.sh"]
