# syntax=docker/dockerfile:1

FROM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS build

ENV CGO_ENABLED=0 GOFLAGS=-trimpath GOTOOLCHAIN=local
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . ./

ARG COMMIT_SHA=unknown
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -buildvcs=false \
      -ldflags="-s -w -X github.com/Avtobot22/craftPulse/backend/pkg/health._commitSHA=${COMMIT_SHA}" \
      -o /out/ ./cmd/api ./cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-server" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      org.opencontainers.image.source="https://github.com/Avtobot22/craftPulse"

COPY --from=build /out/api /out/worker /usr/local/bin/

USER 65532:65532
EXPOSE 8080 9090
CMD ["/usr/local/bin/api"]
