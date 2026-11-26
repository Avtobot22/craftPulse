# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS cli-source
ENV CGO_ENABLED=0 GOFLAGS="-trimpath -mod=readonly" GOTOOLCHAIN=local
WORKDIR /src
COPY . ./
ARG TARGETOS
ARG TARGETARCH

# The pinned DB can reuse an unstamped downloader across application revisions.
FROM cli-source AS db-downloader
RUN --mount=type=cache,id=ci-cli-modules,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=ci-cli-build,target=/root/.cache/go-build,sharing=locked \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -buildvcs=false -ldflags="-s -w" \
      -o /out/craftpulse-ci ./cmd/craftpulse-ci

FROM cli-source AS build
ARG COMMIT_SHA=unknown
RUN --mount=type=cache,id=ci-cli-modules,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=ci-cli-build,target=/root/.cache/go-build,sharing=locked \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -buildvcs=false \
      -ldflags="-s -w -X github.com/Avtobot22/craftPulse/backend/pkg/health._commitSHA=${COMMIT_SHA}" \
      -o /out/craftpulse-ci ./cmd/craftpulse-ci

# A GitLab job image needs a shell for the job script, so the runtime is Alpine, not distroless. Profile
# images build on this one and add their tools.
FROM alpine:3.23.3@sha256:59855d3dceb3ae53991193bd03301e082b2a7faa56a514b03527ae0ec2ce3a95 AS ci-base
RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates && \
    addgroup -S -g 65532 nonroot && \
    adduser -S -D -u 65532 -G nonroot nonroot
LABEL org.opencontainers.image.source="https://github.com/Avtobot22/craftPulse"

FROM ci-base AS ci-runtime
COPY --link --from=build /out/craftpulse-ci /usr/local/bin/craftpulse-ci
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-ci" \
      org.opencontainers.image.revision="${COMMIT_SHA}"
USER 65532:65532

# Gitleaks is linked into craftpulse-ci at the version in go.mod. Git reads checked-out objects;
# raw blobs and scanner findings stay in memory. GitLab still needs the base shell.
FROM ci-base AS gitleaks-default
RUN apk add --no-cache git
ENV CRAFTPULSE_SECRETS=1
COPY pkg/secrets/profiles/gitleaks-default/ /opt/craftpulse/profiles/gitleaks-default/
COPY --link --from=build /out/craftpulse-ci /usr/local/bin/craftpulse-ci
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-ci-gitleaks-default" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      io.craftpulse.profile="gitleaks-default"
USER 65532:65532

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS golangci-lint
ENV CGO_ENABLED=0 GOFLAGS=-trimpath GOTOOLCHAIN=local
ARG GOLANGCI_LINT_VERSION=2.15.0
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v${GOLANGCI_LINT_VERSION} && \
    mkdir -p /out && cp "$(find /go/bin -name golangci-lint -type f)" /out/golangci-lint

# golangci-lint type-checks the packages with the Go toolchain of the target platform.
FROM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS go-toolchain

FROM scratch AS jscpd-amd64
ADD --checksum=sha256:56215dd2eeb7ce52afb9874e4ba45e3ed2dd67a1b0c98d7d5639b025ba32ae8a \
    https://registry.npmjs.org/jscpd-linux-x64-musl/-/jscpd-linux-x64-musl-5.4.2.tgz /jscpd.tgz

FROM scratch AS jscpd-arm64
ADD --checksum=sha256:75bc3fceae4465aa867250558311e0ef55ea76ac3450b85dc25e553af7d090bb \
    https://registry.npmjs.org/jscpd-linux-arm64-musl/-/jscpd-linux-arm64-musl-5.4.2.tgz /jscpd.tgz

FROM jscpd-${TARGETARCH} AS jscpd

# Code profile of Go: golangci-lint, Lizard and jscpd at the versions of profile.json. The job runs
# "craftpulse-ci code" and "craftpulse-ci upload"; the analysis needs no network when the module
# dependencies are in the checkout or the module cache.
FROM ci-base AS go-default
RUN apk add --no-cache python3
COPY --from=go-toolchain /usr/local/go /usr/local/go
COPY --from=golangci-lint /out/golangci-lint /usr/local/bin/golangci-lint
COPY internal/ci/profiles/go-default/ /opt/craftpulse/profiles/go-default/
RUN --mount=from=jscpd,target=/mnt/jscpd \
    tar -xzf /mnt/jscpd/jscpd.tgz -C /tmp package/bin/jscpd && \
    install -m 0755 /tmp/package/bin/jscpd /usr/local/bin/jscpd && rm -rf /tmp/package && \
    python3 -m venv /opt/lizard && \
    /opt/lizard/bin/pip install --no-cache-dir --no-deps --require-hashes --only-binary=:all: \
      -r /opt/craftpulse/profiles/go-default/lizard-requirements.txt && \
    ln -s /opt/lizard/bin/lizard /usr/local/bin/lizard
ENV PATH=/usr/local/go/bin:/opt/lizard/bin:$PATH GOTOOLCHAIN=local CGO_ENABLED=0 \
    GOPATH=/tmp/go GOCACHE=/tmp/go-cache GOLANGCI_LINT_CACHE=/tmp/golangci-lint-cache \
    CRAFTPULSE_CODE_PROFILE=/opt/craftpulse/profiles/go-default
COPY --link --from=build /out/craftpulse-ci /usr/local/bin/craftpulse-ci
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-ci-go-default" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      io.craftpulse.profile="go-default"
USER 65532:65532

# Code profile of Python: Ruff, Lizard and jscpd at the versions of profile.json; Ruff and Lizard come from
# requirements.txt by hash.
FROM ci-base AS python-default
RUN apk add --no-cache python3
COPY internal/ci/profiles/python-default/ /opt/craftpulse/profiles/python-default/
RUN --mount=from=jscpd,target=/mnt/jscpd \
    tar -xzf /mnt/jscpd/jscpd.tgz -C /tmp package/bin/jscpd && \
    install -m 0755 /tmp/package/bin/jscpd /usr/local/bin/jscpd && rm -rf /tmp/package && \
    python3 -m venv /opt/code-tools && \
    /opt/code-tools/bin/pip install --no-cache-dir --no-deps --require-hashes --only-binary=:all: \
      -r /opt/craftpulse/profiles/python-default/requirements.txt && \
    ln -s /opt/code-tools/bin/lizard /usr/local/bin/lizard && ln -s /opt/code-tools/bin/ruff /usr/local/bin/ruff
ENV PATH=/opt/code-tools/bin:$PATH CRAFTPULSE_CODE_PROFILE=/opt/craftpulse/profiles/python-default HOME=/tmp
COPY --link --from=build /out/craftpulse-ci /usr/local/bin/craftpulse-ci
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-ci-python-default" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      io.craftpulse.profile="python-default"
USER 65532:65532

# ESLint, typescript-eslint and their dependencies come from package-lock.json; the packages are plain
# JavaScript, so the build stage needs no Node version of its own.
FROM node:26.3.0-alpine@sha256:9c0e1e52125d6b67d505cf75b4880fcf1290ccea5c480849910e1d57b2cf72b5 AS eslint
WORKDIR /opt/craftpulse/profiles/js-default
COPY internal/ci/profiles/js-default/package.json internal/ci/profiles/js-default/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --ignore-scripts --no-audit --no-fund

# Code profile of JavaScript and TypeScript: ESLint with typescript-eslint, Lizard and jscpd at the
# versions of profile.json. ESLint loads only the configuration and plugins of the profile.
FROM ci-base AS js-default
RUN apk add --no-cache nodejs python3
COPY internal/ci/profiles/js-default/ /opt/craftpulse/profiles/js-default/
COPY --from=eslint /opt/craftpulse/profiles/js-default/node_modules/ /opt/craftpulse/profiles/js-default/node_modules/
RUN --mount=from=jscpd,target=/mnt/jscpd \
    tar -xzf /mnt/jscpd/jscpd.tgz -C /tmp package/bin/jscpd && \
    install -m 0755 /tmp/package/bin/jscpd /usr/local/bin/jscpd && rm -rf /tmp/package && \
    python3 -m venv /opt/code-tools && \
    /opt/code-tools/bin/pip install --no-cache-dir --no-deps --require-hashes --only-binary=:all: \
      -r /opt/craftpulse/profiles/js-default/lizard-requirements.txt && \
    ln -s /opt/code-tools/bin/lizard /usr/local/bin/lizard && \
    ln -s /opt/craftpulse/profiles/js-default/node_modules/eslint/bin/eslint.js /usr/local/bin/eslint
ENV PATH=/opt/code-tools/bin:$PATH CRAFTPULSE_CODE_PROFILE=/opt/craftpulse/profiles/js-default HOME=/tmp
COPY --link --from=build /out/craftpulse-ci /usr/local/bin/craftpulse-ci
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-ci-js-default" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      io.craftpulse.profile="js-default"
USER 65532:65532

FROM scratch AS opengrep-amd64
ADD --checksum=sha256:aa03c3bfff105dc122cbbe6e239dc39a5cae486f1837bba3af7a16bca173f8d6 \
    https://github.com/opengrep/opengrep/releases/download/v1.33.1/opengrep_musllinux_x86 /opengrep

FROM scratch AS opengrep-arm64
ADD --checksum=sha256:17c6917b53d8f8af794d8effb13c1a068018c602435d33fefc105aaa23d96f4b \
    https://github.com/opengrep/opengrep/releases/download/v1.33.1/opengrep_musllinux_aarch64 /opengrep

FROM opengrep-${TARGETARCH} AS opengrep

# SAST profile: Opengrep at the version of profile.json with the rule set of the profile. The job runs
# "craftpulse-ci sast" and "craftpulse-ci upload".
FROM ci-base AS opengrep-default
COPY --from=opengrep --chmod=0755 /opengrep /usr/local/bin/opengrep
COPY internal/ci/profiles/opengrep-default/ /opt/craftpulse/profiles/opengrep-default/
ENV CRAFTPULSE_SAST_PROFILE=/opt/craftpulse/profiles/opengrep-default HOME=/tmp
COPY --link --from=build /out/craftpulse-ci /usr/local/bin/craftpulse-ci
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-ci-opengrep-default" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      io.craftpulse.profile="opengrep-default"
USER 65532:65532

FROM scratch AS trivy-amd64
ADD --checksum=sha256:bc0fc726e017b8927a62b2d3531d7a84ee01769daf5c99ed866dced54089fdbf \
    https://github.com/aquasecurity/trivy/releases/download/v0.76.2/trivy_0.76.2_Linux-64bit.tar.gz /trivy.tgz

FROM scratch AS trivy-arm64
ADD --checksum=sha256:ab9d51f86e6a5fb936f605faff6b593ac51bc378fff4d3e8b9d58358b9c367c3 \
    https://github.com/aquasecurity/trivy/releases/download/v0.76.2/trivy_0.76.2_Linux-ARM64.tar.gz /trivy.tgz

FROM trivy-${TARGETARCH} AS trivy-archive

FROM ci-base AS trivy-tool
RUN apk add --no-cache git
RUN --mount=from=trivy-archive,target=/mnt/trivy tar -xzf /mnt/trivy/trivy.tgz -C /usr/local/bin trivy

# The vulnerability database snapshot of the image, pinned by the digest of its OCI artifact.
FROM trivy-tool AS trivy-db
ARG TRIVY_DB_REPOSITORY=ghcr.io/aquasecurity/trivy-db@sha256:5622a5d226f954dec2919250bc79cd1725e339a1c453587c3a6aa3d6daab4a78
RUN --mount=type=bind,from=db-downloader,source=/out/craftpulse-ci,target=/usr/local/bin/craftpulse-ci \
    craftpulse-ci trivy-db --cache-dir /opt/craftpulse/trivy --repository "$TRIVY_DB_REPOSITORY"

# SCA and IaC profile: Trivy at the version of profile.json with the database snapshot of the image; the job
# runs "craftpulse-ci trivy" and "craftpulse-ci upload" without a network. The job user owns the database, so
# "craftpulse-ci trivy-db" can replace it from an internal mirror.
FROM trivy-tool AS trivy-default
COPY internal/ci/profiles/trivy-default/ /opt/craftpulse/profiles/trivy-default/
COPY --link --from=trivy-db --chown=65532:65532 /opt/craftpulse/trivy/ /opt/craftpulse/trivy/
ENV CRAFTPULSE_TRIVY_PROFILE=/opt/craftpulse/profiles/trivy-default CRAFTPULSE_TRIVY_CACHE=/opt/craftpulse/trivy HOME=/tmp
COPY --link --from=build /out/craftpulse-ci /usr/local/bin/craftpulse-ci
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-ci-trivy-default" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      io.craftpulse.profile="trivy-default"
USER 65532:65532

FROM node:26.3.0-alpine@sha256:9c0e1e52125d6b67d505cf75b4880fcf1290ccea5c480849910e1d57b2cf72b5 AS spectral
WORKDIR /opt/craftpulse/profiles/git-markdown-openapi
COPY internal/ci/profiles/git-markdown-openapi/package.json internal/ci/profiles/git-markdown-openapi/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --ignore-scripts --no-audit --no-fund

# Git documentation profile: Markdown links, anchors and the inventory by craftpulse-ci with goldmark, OpenAPI
# by Spectral at the version of profile.json with the ruleset of the profile. The job runs "craftpulse-ci docs"
# and "craftpulse-ci upload".
FROM ci-base AS git-markdown-openapi
RUN apk add --no-cache nodejs
COPY internal/ci/profiles/git-markdown-openapi/ /opt/craftpulse/profiles/git-markdown-openapi/
COPY --from=spectral /opt/craftpulse/profiles/git-markdown-openapi/node_modules/ /opt/craftpulse/profiles/git-markdown-openapi/node_modules/
RUN ln -s /opt/craftpulse/profiles/git-markdown-openapi/node_modules/@stoplight/spectral-cli/dist/index.js /usr/local/bin/spectral
ENV CRAFTPULSE_DOCS_PROFILE=/opt/craftpulse/profiles/git-markdown-openapi HOME=/tmp
COPY --link --from=build /out/craftpulse-ci /usr/local/bin/craftpulse-ci
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-ci-git-markdown-openapi" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      io.craftpulse.profile="git-markdown-openapi"
USER 65532:65532

# Keep the default target the base CLI, for existing callers.
FROM ci-runtime AS ci
