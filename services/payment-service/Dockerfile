FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY bin/linux/payment-service /service
COPY services/payment-service/migrations /migrations
CMD ["/service"]
