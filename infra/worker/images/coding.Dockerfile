FROM golang:1.27.3-bookworm AS go-toolchain
FROM golang:1.27.3-bookworm AS check-build

WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/check-runner ./cmd/check-runner

FROM oven/bun:1.4.2-debian

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 worker \
    && useradd --uid 10001 --gid 10001 --create-home worker
COPY --from=go-toolchain /usr/local/go /usr/local/go
COPY --from=check-build /out/check-runner /opt/check-runner
ENV PATH=/usr/local/go/bin:$PATH GOPATH=/opt/gopath GOMODCACHE=/opt/gopath/pkg/mod
WORKDIR /opt/dependencies
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
WORKDIR /opt/go-cache
COPY server/go.mod server/go.sum ./
RUN go mod download \
    && chmod -R a=rX /opt/dependencies /opt/gopath /usr/local/go /opt/check-runner
USER 10001:10001
WORKDIR /work/repository
CMD ["/opt/check-runner"]
