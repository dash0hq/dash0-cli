FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY go-cli .
RUN CGO_ENABLED=0 GOOS=linux go build -o dash0 ./cmd/dash0

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/dash0 .

ENTRYPOINT ["./dash0"]
