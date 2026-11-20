FROM alpine:3.23.3 AS runtime
RUN apk add --no-cache curl
USER 65532:65532
HEALTHCHECK CMD ["curl", "-f", "http://localhost:8080/health"]
CMD ["sleep", "infinity"]
