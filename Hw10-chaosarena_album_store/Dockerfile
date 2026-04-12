FROM golang:1.23-bookworm AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server main.go

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=builder /app/server /app/server
ENV PORT=8080
EXPOSE 8080
CMD ["/app/server"]
