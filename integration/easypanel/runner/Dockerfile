# Runner image: builds easypanel-migrate and carries the docker CLI so the E2E
# test can drive host-side detection (docker ps / docker exec) against the two
# fake panel containers over the mounted docker socket.
FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/easypanel-migrate ./cmd/easypanel-migrate

FROM debian:bookworm-slim
# Install a recent docker CLI from Docker's official repo; distro docker.io
# ships an API-1.41 client that is too old for modern daemons.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl gnupg \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc \
    && chmod a+r /etc/apt/keyrings/docker.asc \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian bookworm stable" > /etc/apt/sources.list.d/docker.list \
    && apt-get update && apt-get install -y --no-install-recommends docker-ce-cli \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/easypanel-migrate /usr/local/bin/easypanel-migrate
