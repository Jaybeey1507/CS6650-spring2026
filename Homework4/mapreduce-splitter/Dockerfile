FROM golang:1.21 AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o splitter main.go

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /app/splitter /splitter
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/splitter"]
