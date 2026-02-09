FROM golang:1.23 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o mapper main.go

FROM gcr.io/distroless/base-debian12
COPY --from=build /app/mapper /mapper
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/mapper"]
