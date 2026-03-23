FROM golang:1.24-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
COPY vendor ./vendor
COPY . .

RUN go build -mod=vendor -o server .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/server .
COPY --from=builder /app/schema.sql .
EXPOSE 8080
CMD ["./server"]