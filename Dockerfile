FROM golang:1.24-alpine AS builder

ARG VERSION=dev

WORKDIR /app

RUN go install github.com/swaggo/swag/cmd/swag@latest

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN swag init -g cmd/server/main.go -o docs --parseDependency 2>/dev/null; \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=${VERSION}" -o /drugs ./cmd/server

FROM alpine:3.21

RUN apk --no-cache add ca-certificates
COPY --from=builder /drugs /drugs
COPY config.yaml /config.yaml

ENV CONFIG_PATH=/config.yaml
ENV LISTEN_ADDR=:8080

EXPOSE 8080

ENTRYPOINT ["/drugs"]
