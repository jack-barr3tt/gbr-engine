# Multi-stage Dockerfile for gbr-engine
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /gbr-engine ./

FROM scratch
COPY --from=builder /gbr-engine /gbr-engine
EXPOSE 8080
ENTRYPOINT ["/gbr-engine"]
