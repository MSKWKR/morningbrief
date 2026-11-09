FROM golang:1.27.3-alpine3.24 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/morningbrief ./cmd/morningbrief

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata

ARG SUPERCRONIC_VERSION=v0.2.51
ARG SUPERCRONIC_SHA256=cc232108374e8e2b537045f2d5eaa1229f619db314c3ba6a14d38b35dc0aa123
ADD https://github.com/aptible/supercronic/releases/download/${SUPERCRONIC_VERSION}/supercronic-linux-amd64 /usr/local/bin/supercronic
RUN echo "${SUPERCRONIC_SHA256}  /usr/local/bin/supercronic" | sha256sum -c - \
    && chmod +x /usr/local/bin/supercronic

ENV TZ=Asia/Taipei
WORKDIR /app
COPY --from=build /app/morningbrief /app/morningbrief
COPY deploy/crontab /app/crontab

ENTRYPOINT ["/usr/local/bin/supercronic", "/app/crontab"]
