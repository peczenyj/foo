ARG GO_VERSION=1.27
# The build stage runs on the builder's own platform and cross-compiles for
# the target one (TARGETOS/TARGETARCH, set by BuildKit): a multi-platform
# build needs no emulation.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build

WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG VERSION=dev
ARG REVISION=unknown
ARG NAME=foo
ARG TARGETOS
ARG TARGETARCH

# Static, reproducible binary: no cgo, no local paths, no debug symbols.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.revision=${REVISION}" \
      -o /out/${NAME} \
      ./cmd/server

# ---- runtime stage ----------------------------------------------------------
# distroless/static: no shell, no package manager, runs as uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED
# The project name and repository: make docker passes NAME and SOURCE from the
# Makefile; these defaults serve a plain docker build.
ARG NAME=foo
ARG SOURCE=https://github.com/peczenyj/foo
LABEL org.opencontainers.image.title="${NAME}" \
      org.opencontainers.image.description="Example Go HTTP service: GET /foo replies bar" \
      org.opencontainers.image.source="${SOURCE}" \
      org.opencontainers.image.documentation="${SOURCE}/wiki" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}"

# A fixed path: an exec-form ENTRYPOINT cannot expand ARGs, and distroless
# has no shell to do it.
COPY --from=build /out/${NAME} /server

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/server"]
