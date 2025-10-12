FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
COPY src/common/ ./src/common/
COPY src/http-api/ ./src/http-api/
COPY spec/ ./spec/
WORKDIR /app/src/http-api
RUN go mod download
RUN go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
RUN go generate
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/src/http-api/main .
EXPOSE 3000
CMD ["./main"]