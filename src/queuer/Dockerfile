FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org
RUN go mod download
COPY src/ ./src/
WORKDIR /src/src/queuer
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /queuer ./

FROM scratch
COPY --from=builder /queuer /queuer
EXPOSE 8080
ENTRYPOINT ["/queuer"]
