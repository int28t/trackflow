FROM golang:1.25.1-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/api-gateway ./cmd/server

FROM alpine:3.21

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /out/api-gateway /app/api-gateway

USER app

EXPOSE 8081

ENTRYPOINT ["/app/api-gateway"]
