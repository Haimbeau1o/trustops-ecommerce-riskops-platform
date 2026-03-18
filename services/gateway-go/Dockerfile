FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gateway ./cmd/server

FROM alpine:3.22

WORKDIR /app
COPY --from=builder /out/gateway /app/gateway

EXPOSE 8080
ENTRYPOINT ["/app/gateway"]
