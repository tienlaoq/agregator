#!/bin/sh
set -e
cd /src/gen/go && go mod tidy
cd /src && mkdir -p bin/linux
for svc in auth-service user-service venue-service booking-service review-service payment-service api-gateway; do
  echo "=== $svc ==="
  (cd "/src/services/$svc" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "/src/bin/linux/$svc" ./cmd/)
done
ls -la /src/bin/linux
