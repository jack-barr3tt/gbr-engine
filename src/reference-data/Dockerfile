FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org
RUN go mod download
COPY src/common/ ./src/common/
COPY src/reference-data/ ./src/reference-data
WORKDIR /src/src/reference-data
RUN CGO_ENABLED=0 GOOS=linux go build -o /reference-data ./

FROM gcr.io/distroless/static:latest
COPY --from=builder --chmod=0755 /reference-data /reference-data
EXPOSE 8080
ENTRYPOINT ["/reference-data"]
