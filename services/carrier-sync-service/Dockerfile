FROM golang:1.25.1-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/carrier-sync-service ./cmd/server

FROM alpine:3.21

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /out/carrier-sync-service /app/carrier-sync-service

USER app

EXPOSE 8084

ENTRYPOINT ["/app/carrier-sync-service"]
