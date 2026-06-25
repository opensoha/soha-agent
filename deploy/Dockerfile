FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS agent-build
ARG TARGETOS=linux
ARG TARGETARCH
ARG GOPROXY=https://proxy.golang.org,direct
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
ENV GOPROXY=$GOPROXY
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./soha-agent/
COPY --from=contracts go.mod go.sum ./soha-contracts/
RUN for attempt in 1 2 3; do \
      printf 'go 1.25.0\n\nuse (\n\t./soha-agent\n\t./soha-contracts\n)\n\nreplace github.com/opensoha/soha-contracts v0.1.1 => ./soha-contracts\n' > go.work; \
      (cd soha-agent && go mod download) && exit 0; \
      echo "go mod download failed on attempt ${attempt}, retrying..." >&2; \
      sleep 2; \
    done; \
    exit 1
COPY --from=contracts . ./soha-contracts
COPY . ./soha-agent
RUN if [ -z "$TARGETARCH" ]; then TARGETARCH="$(go env GOARCH)"; fi; \
    cd soha-agent && GOWORK=off CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X github.com/opensoha/soha-agent/internal/agent/buildinfo.Version=${VERSION} -X github.com/opensoha/soha-agent/internal/agent/buildinfo.Commit=${COMMIT} -X github.com/opensoha/soha-agent/internal/agent/buildinfo.Date=${DATE}" -o /out/soha-agent ./cmd/agent

FROM alpine:3.20
RUN addgroup -S -g 10001 soha && adduser -S -D -H -u 10001 -G soha soha && apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=agent-build /out/soha-agent /app/soha-agent
COPY configs/agent.config.yaml /app/configs/agent.config.yaml
RUN mkdir -p /app/configs /var/lib/soha-agent /var/log/soha-agent \
    && chown -R 10001:10001 /var/lib/soha-agent /var/log/soha-agent
ENV SOHA_AGENT_CONFIG_FILE=/app/configs/agent.config.yaml
USER 10001:10001
EXPOSE 18080
ENTRYPOINT ["/app/soha-agent"]
