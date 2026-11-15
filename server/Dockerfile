FROM oven/bun:1.4.2-debian AS client-build
WORKDIR /src
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
COPY tsconfig.json biome.json ./
COPY client ./client
RUN bun run build:client

FROM golang:1.27.3-bookworm AS server-build
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -buildid=" -o /out/portal ./cmd/api

FROM alpine:3.22.2
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 portal
WORKDIR /app
COPY --from=server-build /out/portal /app/portal
COPY --from=client-build /src/dist /app/static
COPY infra/mihomo/hackathon.yaml /app/infra/mihomo/hackathon.yaml
COPY AGENTS.md /app/AGENTS.md
USER 10001:10001
ENV PORTAL_LISTEN=0.0.0.0:4000 STATIC_DIR=/app/static MIHOMO_CONFIG_PATH=/app/infra/mihomo/hackathon.yaml AGENTS_PATH=/app/AGENTS.md
EXPOSE 4000
HEALTHCHECK --interval=15s --timeout=3s --retries=5 --start-period=20s CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:4000/health"]
ENTRYPOINT ["/app/portal"]
