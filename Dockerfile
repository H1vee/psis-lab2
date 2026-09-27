ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder

ARG APP_VERSION=1.0.0

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${APP_VERSION}" \
    -o /out/teapot .

FROM scratch AS runtime

ARG APP_VERSION=1.0.0

LABEL org.opencontainers.image.title="teapot" \
      org.opencontainers.image.description="Minimal HTTP service returning tea brewing parameters" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.authors="Artem Ledynskyi"

COPY --from=builder /out/teapot /teapot

ENV TEAPOT_ADDR=":8080"

USER 10001:10001

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=2s --retries=3 \
    CMD ["/teapot", "-healthcheck"]

ENTRYPOINT ["/teapot"]
CMD []
