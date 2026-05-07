FROM golang:1.25.4-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bastyle-blacklist ./cmd/main.go

FROM debian:bookworm-slim

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl ffmpeg \
	&& rm -rf /var/lib/apt/lists/*

RUN useradd --system --home-dir /var/lib/bastyle --create-home --shell /usr/sbin/nologin bastyle \
	&& mkdir -p /etc/bastyle /var/lib/bastyle \
	&& chown -R bastyle:bastyle /var/lib/bastyle

COPY --from=builder /out/bastyle-blacklist /usr/local/bin/bastyle-blacklist

USER bastyle
WORKDIR /var/lib/bastyle

ENTRYPOINT ["/usr/local/bin/bastyle-blacklist"]
CMD ["-config", "/etc/bastyle/config.yaml"]
