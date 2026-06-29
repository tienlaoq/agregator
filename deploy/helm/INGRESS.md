# Ingress / edge & TLS (Kubernetes)

The chart exposes two host-based ingresses (mirroring the Caddy subdomain split):

| Host (example)   | Backend                  |
|------------------|--------------------------|
| `api.<domain>`   | `agregator-api-gateway:8080` |
| `<domain>`       | `agregator-frontend:3000`    |

Cloudflare is **not** used on the k8s path. For РФ, terminate TLS inside Russia —
either the Yandex Application Load Balancer (recommended, managed) or
nginx-ingress with an RF-issued certificate.

## Option A — Yandex ALB Ingress (default in values.yaml)

RF-native managed L7 load balancer + TLS from Yandex Certificate Manager. The
chart is pre-wired; fill the `CHANGE_ME` values under `ingress` / `ingressFrontend`:

1. **Install the ALB Ingress Controller** in the cluster (YC Marketplace / Helm).
2. **Certificate Manager**: issue a managed certificate for your domain(s) and
   note each certificate ID. Reference it as the TLS secret name:
   `tlsSecretName: yc-certmgr-cert-id-<certificate-id>` (the controller resolves
   this special name — no real k8s TLS secret needed).
3. **Annotations** (already in `values.yaml`):
   - `ingress.alb.yc.io/subnets` — comma-separated subnet IDs for the balancer
   - `ingress.alb.yc.io/security-groups` — SG allowing 80/443 in, health checks
   - `ingress.alb.yc.io/external-ipv4-address: auto` — provision a public IP
   - `ingress.alb.yc.io/group-name: agregator` — **same on both** so they share
     one ALB (one public IP, two host rules)
4. **DNS**: point `A` records for both hosts at the ALB's external IPv4 address
   (`kubectl get ingress` shows it once provisioned).

## Option B — nginx-ingress

```yaml
ingress:
  enabled: true
  className: nginx
  host: api.example.ru
  tlsSecretName: api-tls          # real TLS secret (e.g. cert-manager)
  annotations: {}                 # drop the alb.yc.io annotations
ingressFrontend:
  enabled: true
  className: nginx
  host: example.ru
  tlsSecretName: app-tls
  annotations: {}
```

Provide the TLS secrets yourself (cert-manager with an RF ACME CA, or import an
RF-issued cert as a `kubernetes.io/tls` secret).

## After the edge is up: trust X-Forwarded-For

The api-gateway ignores `X-Forwarded-For` unless the proxy CIDR is trusted, so
real client IPs (rate limiting, logs) need `TRUSTED_PROXY_CIDRS` set to the edge's
in-cluster source CIDR. Set it in `values.yaml` under `services.api-gateway.env`
once you know the ALB/ingress pod subnet, e.g.:

```yaml
services:
  api-gateway:
    env:
      TRUSTED_PROXY_CIDRS: "10.0.0.0/8"   # cluster/LB CIDR — narrow as possible
```

## Validate

```bash
helm template agregator deploy/helm/agregator | kubectl apply --dry-run=client -f -
kubectl get ingress -n agregator
```
