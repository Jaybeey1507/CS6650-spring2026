FROM golang:1.23 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o reducer main.go

FROM gcr.io/distroless/base-debian12
COPY --from=build /app/reducer /reducer
EXPOSE 8080
ENTRYPOINT ["/reducer"]
