FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY bin/linux/user-service /service
COPY services/user-service/migrations /migrations
CMD ["/service"]
