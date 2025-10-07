FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org
RUN go mod download
COPY src/common/ ./src/common/
COPY src/trust-consumer/ ./src/trust-consumer
WORKDIR /src/src/trust-consumer
RUN CGO_ENABLED=0 GOOS=linux go build -o /trust-consumer ./

FROM scratch
COPY --from=builder /trust-consumer /trust-consumer
EXPOSE 8080
ENTRYPOINT ["/trust-consumer"]
