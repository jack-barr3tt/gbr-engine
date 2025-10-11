FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org
RUN go mod download
COPY src/common/ ./src/common/
COPY src/vstp-consumer/ ./src/vstp-consumer
WORKDIR /src/src/vstp-consumer
RUN CGO_ENABLED=0 GOOS=linux go build -o /vstp-consumer ./

FROM scratch
COPY --from=builder /vstp-consumer /vstp-consumer
EXPOSE 8080
ENTRYPOINT ["/vstp-consumer"]