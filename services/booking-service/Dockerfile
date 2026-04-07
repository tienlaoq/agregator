FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY bin/linux/booking-service /service
COPY services/booking-service/migrations /migrations
CMD ["/service"]
