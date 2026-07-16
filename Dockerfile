FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o tavily-router .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/tavily-router .
COPY config.example.yaml .
EXPOSE 8787
ENTRYPOINT ["./tavily-router"]
CMD ["-config", "config.yaml"]
