# GitHub CI/CD

This repository now includes:

- `ci.yml` — Go + frontend lint/test/build on PR and push.
- `cd-k8s-prod.yml` — build/push all images to Yandex Container Registry on every push to `main`; **deploy (helm upgrade) runs only on manual `workflow_dispatch`** ("Run workflow"), never automatically on push.

## Deploying (manual approval gate)

Pushing to `main` builds and pushes images but does **not** deploy. To release:

1. Actions → **CD Kubernetes Production** → **Run workflow** (select `main`).
2. This rebuilds images for that commit and runs `helm upgrade --install`.

Native environment required-reviewers (Settings → Environments → Production) is
not available on the current plan (Free, private repo); the manual-dispatch gate
is the dependency-free equivalent. If the plan is upgraded, add required
reviewers to the `Production` environment — the deploy job already targets it.

## Required GitHub Secrets

Set these in **Settings > Secrets and variables > Actions**:

- `YCR_REGISTRY` � e.g. `cr.yandex`
- `YCR_REPOSITORY_PREFIX` � e.g. `crp123456789/agregator`
- `YCR_USERNAME`
- `YCR_PASSWORD`
- `KUBECONFIG_B64` � base64 encoded kubeconfig for production cluster

## Helm chart

Chart path: `deploy/helm/agregator`.

Each service reads non-secret config from `values.yaml` (`global.env` + per-service
`env`) and sensitive values from a per-service `*-env` Kubernetes secret
(`auth-service-env`, …, plus `migrator-env` and `pg-backup-env`).

Generate and apply all of them with the helper (never commit secrets):

```bash
deploy/helm/secrets/generate-secrets.sh -n agregator   # see SECRETS.md
```

Databases & migrations: `deploy/helm/MIGRATIONS.md`. Backups: `deploy/BACKUPS.md`.

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