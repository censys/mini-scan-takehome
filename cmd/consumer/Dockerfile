FROM golang:1.24 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 go build -o consumer ./cmd/consumer

FROM alpine:3.21
RUN apk add --no-cache sqlite
RUN mkdir -p /data
WORKDIR /app
COPY --from=builder /src/consumer .
CMD ["/app/consumer"]
