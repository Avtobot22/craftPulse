# syntax=docker/dockerfile:1

FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS build

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
