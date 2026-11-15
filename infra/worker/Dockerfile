FROM golang:1.27.3-alpine AS build

WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker-agent ./cmd/worker-agent \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/git-askpass ./cmd/git-askpass

FROM alpine:3.22.2
RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose git openssh-client \
    && adduser -D -u 10001 worker
COPY --from=build /out/worker-agent /usr/local/bin/worker-agent
COPY --from=build /out/git-askpass /usr/local/bin/hackathon-git-askpass
RUN chmod 0555 /usr/local/bin/worker-agent /usr/local/bin/hackathon-git-askpass
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/worker-agent"]
