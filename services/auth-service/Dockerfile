FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY bin/linux/auth-service /service
COPY services/auth-service/migrations /migrations
CMD ["/service"]
