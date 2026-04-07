FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY bin/linux/venue-service /service
COPY services/venue-service/migrations /migrations
CMD ["/service"]
