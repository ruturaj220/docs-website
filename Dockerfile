FROM golang:1.26-trixie AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/docs-platform ./cmd/server

# mkdocs is a Python tool with no Go equivalent, so the runtime image needs
# Python regardless of the server being written in Go.
FROM python:3.12-slim

RUN apt-get update && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY --from=build /out/docs-platform /usr/local/bin/docs-platform
COPY mkdocs-base.yml ./mkdocs-base.yml
# The shared landing page used for services that don't ship their own index.md.
COPY docs ./docs

ENV DATA_DIR=/data \
    LISTEN_ADDR=:8080 \
    MKDOCS_BASE_CONFIG=/app/mkdocs-base.yml \
    COMMON_INDEX=/app/docs/index.md

# Pre-create DATA_DIR owned by the non-root UID the chart runs as, so the
# server works whether /data is a mounted PVC, an emptyDir, or nothing at all.
RUN useradd --uid 65532 --user-group --no-create-home --shell /usr/sbin/nologin nonroot \
    && mkdir -p /data \
    && chown -R 65532:65532 /data

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["docs-platform"]
