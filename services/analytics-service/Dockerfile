# Multi-stage: компиляция внутри образа, чтобы не подхватывать устаревший bin/linux.
FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY pkg ./pkg
COPY services/analytics-service ./services/analytics-service
WORKDIR /src/services/analytics-service
RUN go mod download \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/analytics-service ./cmd/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/analytics-service /service
COPY services/analytics-service/migrations /migrations
CMD ["/service"]
