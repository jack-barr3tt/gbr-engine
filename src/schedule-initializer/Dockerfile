FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY src/ src/
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o schedule-initializer src/schedule-initializer/main.go
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=builder /app/schedule-initializer .
CMD ["./schedule-initializer"]