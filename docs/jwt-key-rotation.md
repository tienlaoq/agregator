# JWT Key Rotation Runbook

Applies to: ES256 (ECDSA P-256) access tokens.  
Affects: `auth-service` (signs), `api-gateway` (verifies).

---

## When to rotate

- Suspected or confirmed private key compromise
- Scheduled rotation (recommended: every 12 months)
- Team member with key access leaves the organisation

---

## Rotation procedure

### 1. Generate a new key pair

```bash
# Private key (PKCS8, unencrypted)
openssl ecparam -name prime256v1 -genkey -noout | \
    openssl pkcs8 -topk8 -nocrypt -out ec_private_new.pem

# Public key (PKIX)
openssl ec -in ec_private_new.pem -pubout -out ec_public_new.pem
```

Store both files in your secrets manager (Vault, AWS Secrets Manager, k8s Secret, etc.).  
**Never commit PEM files to git.**

---

### 2. Deploy api-gateway with BOTH public keys (zero-downtime window)

During rotation there will be tokens signed with the old key still in circulation
(valid for up to `JWT_ACCESS_TTL`, default 15 min).  
To avoid 401s for logged-in users, temporarily accept both the old and new public key.

**Option A — rolling restart with overlap (simplest)**

If `JWT_ACCESS_TTL` is short (≤ 15 min) you can skip this step: just wait one TTL
after deploying auth-service before deploying api-gateway.

**Option B — dual-key verification (zero downtime)**

Add a `JWT_EC_PUBLIC_KEY_PREV` env var and update `middleware/auth.go` to try
the new key first, then fall back to the old one. Remove the fallback after
one full `JWT_ACCESS_TTL` has elapsed post-cutover.

---

### 3. Deploy auth-service with the new private key

Update `JWT_EC_PRIVATE_KEY` (or `JWT_EC_PRIVATE_KEY_FILE`) in `deploy/.env` /
your secrets manager, then restart auth-service:

```bash
# docker-compose
docker compose up -d --no-deps auth-service

# k8s
kubectl rollout restart deployment/auth-service
```

From this point, all newly issued tokens are signed with the new key.

---

### 4. Deploy api-gateway with only the new public key

Once at least one full `JWT_ACCESS_TTL` has passed since step 3
(all old tokens have expired), update `JWT_EC_PUBLIC_KEY` to the new public key
and restart api-gateway:

```bash
docker compose up -d --no-deps api-gateway
# or
kubectl rollout restart deployment/api-gateway
```

---

### 5. Invalidate all refresh tokens (on compromise only)

If the rotation is triggered by a key compromise, old access tokens may still be
valid until they expire. To force all users to re-authenticate immediately:

```sql
-- Run against auth-service Postgres
TRUNCATE TABLE refresh_tokens;
```

This revokes all refresh tokens. Users will be logged out and must log in again.
Access tokens already in circulation will remain valid until `JWT_ACCESS_TTL`
expires — this window is unavoidable without a token blocklist.

---

### 6. Verify

```bash
# Should return 200 with a fresh token pair
curl -X POST https://api.example.com/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"test@example.com","password":"..."}'

# Decode the access_token header — alg must be ES256, kid (if added) must match new key
echo "<access_token>" | cut -d. -f1 | base64 -d 2>/dev/null | jq .
```

---

### 7. Clean up

- Remove the old PEM files from disk and secrets manager
- If you used Option B (dual-key), remove the fallback code
- Update this document with the rotation date

---

## Emergency: immediate lockout

If you need to stop all JWT authentication instantly (e.g. active breach):

1. Set `JWT_EC_PUBLIC_KEY` to an empty or invalid value and restart api-gateway.  
   All token verification will fail → 401 for every authenticated request.
2. Proceed with the full rotation procedure above.
3. Restore a valid public key when the incident is resolved.

---

## Key inventory

| Environment | Secret name            | Location          | Last rotated |
|-------------|------------------------|-------------------|--------------|
| production  | `jwt_ec_private`       | secrets manager   | —            |
| production  | `jwt_ec_public`        | secrets manager   | —            |
| staging     | `JWT_EC_PRIVATE_KEY`   | `deploy/.env`     | —            |
| staging     | `JWT_EC_PUBLIC_KEY`    | `deploy/.env`     | —            |

Update the "Last rotated" column after each rotation.
