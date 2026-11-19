# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS build
ENV CGO_ENABLED=0 GOFLAGS=-trimpath GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . ./
ARG COMMIT_SHA=unknown
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -buildvcs=false \
      -ldflags="-s -w -X github.com/Avtobot22/craftPulse/backend/pkg/health._commitSHA=${COMMIT_SHA}" \
      -o /out/craftpulse-ci ./cmd/craftpulse-ci

# A GitLab job image needs a shell for the job script, so the runtime is Alpine, not distroless. Profile
# images build on this one and add their tools.
FROM alpine:3.23.3@sha256:59855d3dceb3ae53991193bd03301e082b2a7faa56a514b03527ae0ec2ce3a95 AS ci-runtime
RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates && \
    addgroup -S -g 65532 nonroot && \
    adduser -S -D -u 65532 -G nonroot nonroot
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-ci" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      org.opencontainers.image.source="https://github.com/Avtobot22/craftPulse"
COPY --from=build /out/craftpulse-ci /usr/local/bin/craftpulse-ci
USER 65532:65532

# Gitleaks is linked into craftpulse-ci at the version in go.mod. Git reads checked-out objects;
# raw blobs and scanner findings stay in memory. GitLab still needs the base shell.
FROM ci-runtime AS gitleaks-default
USER 0
RUN apk add --no-cache git
ENV CRAFTPULSE_SECRETS=1
LABEL org.opencontainers.image.title="craftpulse-ci-gitleaks-default" \
      io.craftpulse.profile="gitleaks-default"
COPY internal/ci/profiles/gitleaks-default/ /opt/craftpulse/profiles/gitleaks-default/
USER 65532:65532

# Keep the default target the base CLI, for existing callers.
FROM ci-runtime AS ci
