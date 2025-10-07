FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org
RUN go mod download
COPY src/common/ ./src/common/
COPY src/data-fetcher/ ./src/data-fetcher
WORKDIR /src/src/data-fetcher
RUN CGO_ENABLED=0 GOOS=linux go build -o /data-fetcher ./

FROM gcr.io/distroless/static:latest
COPY --from=builder /data-fetcher /data-fetcher
EXPOSE 8080
ENTRYPOINT ["/data-fetcher"]
