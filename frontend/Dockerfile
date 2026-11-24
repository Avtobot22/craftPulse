# syntax=docker/dockerfile:1

FROM node:26.3.0-alpine@sha256:9c0e1e52125d6b67d505cf75b4880fcf1290ccea5c480849910e1d57b2cf72b5 AS build
WORKDIR /src
COPY package.json package-lock.json ./
COPY tools/api-types-generator/package.json ./tools/api-types-generator/package.json
RUN npm ci --no-audit --no-fund
COPY . ./
RUN npm run build

FROM nginx:1.30.8-alpine@sha256:6437a63b5c529f0e06162b3f593c0154cff6fa2ea0f94fd7c27928cb399e022d
RUN apk upgrade --no-cache
ARG COMMIT_SHA=unknown
LABEL org.opencontainers.image.title="craftpulse-frontend" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      org.opencontainers.image.source="https://github.com/Avtobot22/craftPulse"
COPY --from=build /src/dist/ /usr/share/nginx/html/
EXPOSE 80
