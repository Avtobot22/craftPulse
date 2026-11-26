# syntax=docker/dockerfile:1

FROM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS build
ENV CGO_ENABLED=0 GOFLAGS=-trimpath GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
ARG ADAPTER
ARG COMMIT_SHA=unknown
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -buildvcs=false \
      -ldflags="-s -w -X github.com/Avtobot22/craftPulse/backend/pkg/health._commitSHA=${COMMIT_SHA}" \
      -o /out/adapter "./cmd/${ADAPTER}"

FROM alpine:3.23.3@sha256:59855d3dceb3ae53991193bd03301e082b2a7faa56a514b03527ae0ec2ce3a95 AS adapter-runtime
RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates git openssh-client && \
    addgroup -S -g 65532 nonroot && \
    adduser -S -D -u 65532 -G nonroot nonroot && \
    mkdir -p /var/cache/craftpulse-git && chown 65532:65532 /var/cache/craftpulse-git
ENV GIT_CACHE_DIR=/var/cache/craftpulse-git
USER 65532:65532
EXPOSE 8443

FROM adapter-runtime AS runtime
ARG ADAPTER
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-${ADAPTER}" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      org.opencontainers.image.source="https://github.com/Avtobot22/craftPulse"
COPY --from=build /out/adapter /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
