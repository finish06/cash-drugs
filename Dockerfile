FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /drugs ./cmd/server

FROM alpine:3.21

RUN apk --no-cache add ca-certificates
COPY --from=builder /drugs /drugs
COPY config.yaml /config.yaml

ENV CONFIG_PATH=/config.yaml
ENV LISTEN_ADDR=:8080

EXPOSE 8080

ENTRYPOINT ["/drugs"]
