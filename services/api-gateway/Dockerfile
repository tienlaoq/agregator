# Multi-stage: всегда свежая сборка с текущим gen/go (поле city в SearchVenuesRequest).
FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/api-gateway ./services/api-gateway
WORKDIR /src/services/api-gateway
RUN go mod download \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/api-gateway ./cmd/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/api-gateway /service
CMD ["/service"]
