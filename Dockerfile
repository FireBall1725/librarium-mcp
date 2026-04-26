FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
# VERSION is injected at build time by the release workflow. Defaults to the
# value baked into internal/version/version.go when building locally.
ARG VERSION
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w ${VERSION:+-X 'github.com/fireball1725/librarium-mcp/internal/version.Version=${VERSION}'}" \
    -o /librarium-mcp ./cmd/mcp
# Pre-create the persistent data dir so the auto-generated MCP token has
# a mount point with the right perms even when nothing is bind-mounted.
RUN mkdir -p /data

FROM gcr.io/distroless/static-debian12
COPY --from=builder /librarium-mcp /librarium-mcp
COPY --from=builder /data /data
EXPOSE 8090
ENTRYPOINT ["/librarium-mcp"]
