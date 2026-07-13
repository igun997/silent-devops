FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/agent ./cmd/agent && CGO_ENABLED=0 go build -o /out/validator ./cmd/validator && CGO_ENABLED=0 go build -o /out/client ./cmd/client && CGO_ENABLED=0 go build -tags=integration -o /out/integration-helper ./cmd/integration-helper

FROM debian:12-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates openssh-client openssh-server curl sqlite3 && rm -rf /var/lib/apt/lists/* && mkdir -p /run/sshd
COPY --from=build /out/ /usr/local/bin/
COPY packaging/ssh/sshd_config.silent-devops /etc/ssh/sshd_config.silent-devops
COPY integration/entrypoint.sh /entrypoint.sh
RUN chmod 0755 /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
