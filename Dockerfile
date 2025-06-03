# syntax=docker/dockerfile:1.3
# Dockerfile reference: https://docs.docker.com/engine/reference/builder/

# Stage 1: Build web assets
FROM --platform=${BUILDPLATFORM} node:lts-alpine AS bundler
COPY web web
RUN yarn --cwd ./web/source install && \
    yarn --cwd ./web/source ts-patch install && \
    yarn --cwd ./web/source build && \
    rm -rf ./web/source

# Stage 2: Build Go binary
FROM --platform=${BUILDPLATFORM} golang:1.21 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download -x && go mod verify
COPY . .
RUN go build -o gotosocial -v

# Stage 3: Create executor container
FROM --platform=${TARGETPLATFORM} alpine:3.21 AS executor
USER 1000:1000
WORKDIR "/gotosocial/storage"
WORKDIR "/gotosocial/.cache"
WORKDIR "/gotosocial"
COPY --from=builder --chown=1000:1000 /app/gotosocial /gotosocial/gotosocial
COPY --from=bundler --chown=1000:1000 web /gotosocial/web
COPY --from=bundler --chown=1000:1000 /web/assets/swagger.yaml /gotosocial/web/assets/swagger.yaml
VOLUME [ "/gotosocial/storage", "/gotosocial/.cache" ]
EXPOSE 8080
ENTRYPOINT [ "/gotosocial/gotosocial", "server", "start" ]
