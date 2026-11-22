FROM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
WORKDIR /src
COPY go.mod semver.txt ./
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/ web/
ARG VERSION
RUN set -eu; \
    version="$(head -n 1 semver.txt)"; \
    if ! printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then \
      echo "ERROR: semver.txt must start with an X.Y.Z version" >&2; \
      exit 1; \
    fi; \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION:-$version}" -o /out/labdash ./cmd/labdash

FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS certs
RUN apk add --no-cache ca-certificates tzdata

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/labdash /labdash
ENV LABDASH_PORT=8080 LABDASH_CONFIG=/data/config.json
VOLUME ["/data"]
EXPOSE 8080
# Runs as root by default because the Docker socket is usually root:docker.
# Override with `user: "1000:<docker-gid>"` in Compose if your socket allows it.
ENTRYPOINT ["/labdash"]
