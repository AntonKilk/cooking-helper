# ---- Build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Restore dependencies first for layer caching.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO off → fully static binary. Keep it off in CH-3 by using a pure-Go SQLite
# driver (modernc.org/sqlite); switching to a CGO driver requires changing this.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/server

# ---- Run stage ----
FROM alpine:3.20
RUN apk add --no-cache wget ca-certificates \
    && adduser -D -u 10001 app \
    && mkdir -p /data \
    && chown app:app /data
COPY --from=build /out/server /usr/local/bin/server

USER app
ENV PORT=8080
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/server"]
