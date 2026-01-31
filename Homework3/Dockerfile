# Build stage
FROM golang:1.23 AS builder

WORKDIR /app

# Copy go module files first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build a static Linux binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server main.go

# Run stage
FROM alpine:3.20

WORKDIR /app

# Copy the built binary from builder
COPY --from=builder /app/server .

# Expose the port the app listens on
EXPOSE 8080

# Start the server
CMD ["./server"]
