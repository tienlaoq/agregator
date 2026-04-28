# Multi-stage: сборка внутри образа (не зависит от bin/linux на хосте).
FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/auth-service ./services/auth-service
WORKDIR /src/services/auth-service
RUN go mod download \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/auth-service ./cmd/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/auth-service /service
COPY services/auth-service/migrations /migrations
CMD ["/service"]
