# Multi-stage: сборка внутри образа (не зависит от bin/linux на хосте).
FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/user-service ./services/user-service
WORKDIR /src/services/user-service
RUN go mod download \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/user-service ./cmd/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/user-service /service
COPY services/user-service/migrations /migrations
CMD ["/service"]
