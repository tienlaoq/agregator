# GitHub CI/CD

This repository now includes:

- `ci.yml` — Go + frontend lint/test/build on PR and push.
- `cd-k8s-prod.yml` — build/push all backend images to Yandex Container Registry and deploy to Kubernetes via Helm on `main`.

## Required GitHub Secrets

Set these in **Settings > Secrets and variables > Actions**:

- `YCR_REGISTRY` — e.g. `cr.yandex`
- `YCR_REPOSITORY_PREFIX` — e.g. `crp123456789/agregator`
- `YCR_USERNAME`
- `YCR_PASSWORD`
- `KUBECONFIG_B64` — base64 encoded kubeconfig for production cluster

## Helm chart

Chart path: `deploy/helm/agregator`.

By default each service reads env vars from Kubernetes secrets:

- `auth-service-env`
- `user-service-env`
- `venue-service-env`
- `booking-service-env`
- `review-service-env`
- `payment-service-env`
- `master-service-env`
- `chat-service-env`
- `api-gateway-env`

Create these secrets before first deploy.

### Optional ingress

Enable in `deploy/helm/agregator/values.yaml`:

- `ingress.enabled: true`
- `ingress.host`
- `ingress.tlsSecretName`

## Local dry-run

```bash
helm lint deploy/helm/agregator
helm template agregator deploy/helm/agregator
```