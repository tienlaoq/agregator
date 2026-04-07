FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY bin/linux/api-gateway /service
CMD ["/service"]
