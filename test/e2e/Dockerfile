# Minimal test-only image for the `--since` end-to-end harness (never
# shipped). Expects the linux/<arch> `dash0` binary to already be
# cross-compiled by the host into this directory before `docker build` runs
# -- see the `test-e2e-image` Makefile target -- since the harness's
# go.mod replace directive (a local sibling checkout of dash0-api-client-go)
# would otherwise need to be part of the Docker build context too.
FROM alpine:3.20
RUN apk add --no-cache git
COPY dash0 /usr/local/bin/dash0
RUN chmod +x /usr/local/bin/dash0
RUN mkdir -p /work
ENTRYPOINT ["tail", "-f", "/dev/null"]
