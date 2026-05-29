# Multi-stage: сборка внутри образа (не зависит от bin/linux на хосте).
FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/booking-service ./services/booking-service
WORKDIR /src/services/booking-service
RUN go mod download \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/booking-service ./cmd/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/booking-service /service
COPY services/booking-service/migrations /migrations
CMD ["/service"]
