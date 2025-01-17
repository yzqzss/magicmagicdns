FROM golang:latest AS builder
COPY . /src

WORKDIR /src

RUN CGO_ENABLED=0 go build -ldflags="-extldflags=-static -s -w" -o magicmagicdns

FROM alpine:latest
RUN apk update && apk add ca-certificates iptables ip6tables && rm -rf /var/cache/apk/*

# Copy Tailscale binaries from the tailscale image on Docker Hub.
COPY --from=docker.io/tailscale/tailscale:stable /usr/local/bin/tailscaled /app/tailscaled
COPY --from=docker.io/tailscale/tailscale:stable /usr/local/bin/tailscale /app/tailscale
RUN mkdir -p /var/run/tailscale /var/cache/tailscale /var/lib/tailscale


COPY --from=builder /src/magicmagicdns /app/magicmagicdns
COPY start.sh /app/start.sh

RUN chmod +x /app/start.sh
RUN chmod +x /app/magicmagicdns
# Run on container startup.
CMD ["/app/start.sh"]