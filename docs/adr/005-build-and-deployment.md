# ADR-005: Build, embedding and container deployment

Status: accepted · Date: 2026-10-17

## Decision
- **Frontend build:** Vite produces static assets (hashed filenames, precompressed gzip/brotli).
- **Embedding:** assets are embedded in the Go binary with `go:embed`. The server serves hashed files with long cache headers and `index.html` with `no-cache`. Result: one self-contained binary.
- **Container:** multi-stage Dockerfile (node build, then Go build with `CGO_ENABLED=0`, then `gcr.io/distroless/static:nonroot`). Non-root, read-only root filesystem, no shell. Multi-arch (amd64 + arm64) via buildx. Tagged with version and git SHA.
- **Kubernetes:** example manifests in `deploy/k8s/`: Deployment (1 replica), Service, ConfigMap for the YAML, Secret for tokens via env, Ingress with WebSocket-friendly timeouts, liveness `/healthz`, readiness `/readyz`, resource requests/limits, and a termination grace period suited to graceful WebSocket shutdown.
- **Config reload:** the server watches the mounted config directory (handles ConfigMap symlink swaps, with debounce) and pushes `config` to clients. Invalid config keeps the last good one (FR-8).
- **Local development:** Development uses two terminals (Go server, Vite dev server proxying `/ws`); the commands are in the README. A Makefile is optional.
- **CI (later):** lint and test (Go and frontend), build the image, push to a registry.

## Alternatives considered
- Separate frontend (nginx) and backend containers: more moving parts for no benefit.
- Helm chart from the start: plain manifests first; a chart can follow if wanted.

## Consequences
- Single deployable unit and trivial rollbacks. Clients detect new versions via the `welcome` message and reload (NFR-14).
