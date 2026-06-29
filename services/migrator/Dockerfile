# Multi-stage: compile inside the image (no dependency on host bin/linux).
# Build context is the repo root (see .github/workflows/cd-k8s-prod.yml).
FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY pkg ./pkg
COPY services/migrator ./services/migrator
WORKDIR /src/services/migrator
RUN go mod download \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/migrator ./cmd/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/migrator /migrator

# Bundle every service's migrations under /migrations/<service>/ so the binary
# can apply them by service name (see services/migrator/cmd/main.go plan).
COPY services/auth-service/migrations /migrations/auth-service
COPY services/user-service/migrations /migrations/user-service
COPY services/venue-service/migrations /migrations/venue-service
COPY services/crm-service/migrations /migrations/crm-service
COPY services/booking-service/migrations /migrations/booking-service
COPY services/review-service/migrations /migrations/review-service
COPY services/payment-service/migrations /migrations/payment-service
COPY services/master-service/migrations /migrations/master-service
COPY services/chat-service/migrations /migrations/chat-service
COPY services/notification-service/migrations /migrations/notification-service
COPY services/api-gateway/migrations /migrations/api-gateway
COPY services/analytics-service/migrations /migrations/analytics-service

ENV MIGRATIONS_ROOT=/migrations
ENTRYPOINT ["/migrator"]
