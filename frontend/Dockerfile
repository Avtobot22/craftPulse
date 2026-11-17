# syntax=docker/dockerfile:1

FROM node:26.3.0-alpine@sha256:9c0e1e52125d6b67d505cf75b4880fcf1290ccea5c480849910e1d57b2cf72b5 AS build
WORKDIR /src
COPY package.json package-lock.json ./
COPY tools/api-types-generator/package.json ./tools/api-types-generator/package.json
RUN npm ci --no-audit --no-fund
COPY . ./
RUN npm run build

FROM nginx:1.30.6-alpine@sha256:901e593c95dc85f5a729f5765516321366ef4ac401821431d408897e95287d0a
LABEL org.opencontainers.image.title="craftpulse-frontend" \
      org.opencontainers.image.source="https://github.com/Avtobot22/craftPulse"
COPY --from=build /src/dist/ /usr/share/nginx/html/
EXPOSE 80
