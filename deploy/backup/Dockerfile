# Postgres logical-backup image: pg_dump (matching server major 16) + mc client
# for S3-compatible upload (Yandex Object Storage / MinIO).
# Build context is deploy/backup (see .github/workflows/cd-k8s-prod.yml).
FROM postgres:16-alpine

# mc (MinIO client) speaks the S3 API and already underpins this platform's
# media storage. Pinned to a dated release for reproducible builds.
ARG MC_RELEASE=RELEASE.2025-04-08T15-39-49Z
RUN apk add --no-cache ca-certificates wget \
	&& wget -q "https://dl.min.io/client/mc/release/linux-amd64/archive/mc.${MC_RELEASE}" -O /usr/local/bin/mc \
	&& chmod +x /usr/local/bin/mc

COPY backup.sh /usr/local/bin/backup.sh
RUN chmod +x /usr/local/bin/backup.sh

# Run as the unprivileged postgres user that ships with the base image.
USER postgres
ENTRYPOINT ["/usr/local/bin/backup.sh"]
