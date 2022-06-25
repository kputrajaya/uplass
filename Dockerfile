# Build
FROM golang:1.18-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /app


# Run
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app /
EXPOSE 8080
ENTRYPOINT ["/app"]
