FROM golang:1.26.5-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kaftar ./cmd

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 kaftar
WORKDIR /app
COPY --from=builder /out/kaftar /usr/local/bin/kaftar
USER kaftar
ENV HTTP_PORT=8080
EXPOSE 8080
ENTRYPOINT ["kaftar"]
