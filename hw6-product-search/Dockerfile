# ---- build stage ----
FROM golang:1.23-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server .

# ---- run stage ----
FROM alpine:3.19
WORKDIR /app

COPY --from=build /app/server /app/server

EXPOSE 8080
ENTRYPOINT ["/app/server"]