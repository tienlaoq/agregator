# Multi-stage: компиляция внутри образа, чтобы не подхватывать устаревший bin/linux с другим proto.
FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/venue-service ./services/venue-service
WORKDIR /src/services/venue-service
RUN go mod download \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/venue-service ./cmd/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/venue-service /service
COPY services/venue-service/migrations /migrations
CMD ["/service"]
