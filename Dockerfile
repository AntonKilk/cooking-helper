# ---- Build stage ----
# go1.26.3 patches stdlib html/template / crypto / net advisories flagged by
# govulncheck (GO-2026-4865/4866/4870/4918/4946/4947/4971/4980/4982).
FROM golang:1.26.3-alpine AS build
WORKDIR /src

# Restore dependencies first for layer caching.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO off → fully static binary. Keep it off in CH-3 by using a pure-Go SQLite
# driver (modernc.org/sqlite); switching to a CGO driver requires changing this.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/server

# ---- Run stage ----
# `sqlite` (the CLI) is needed only for the daily `.backup` job invoked via
# `docker exec` (tech-design §7 / ops). The server itself uses the pure-Go
# modernc.org/sqlite driver, so this does NOT reintroduce CGO into the build.
FROM alpine:3.20
RUN apk add --no-cache wget ca-certificates sqlite \
    && adduser -D -u 10001 app \
    && mkdir -p /data \
    && chown app:app /data
COPY --from=build /out/server /usr/local/bin/server

USER app
ENV PORT=8080
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/server"]
