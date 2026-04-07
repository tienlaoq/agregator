FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY bin/linux/review-service /service
COPY services/review-service/migrations /migrations
CMD ["/service"]
