FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY bin/linux/master-service /service
COPY services/master-service/migrations /migrations
CMD ["/service"]
