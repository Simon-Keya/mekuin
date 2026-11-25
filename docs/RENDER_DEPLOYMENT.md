# MEKU-In — Render deployment guide

## Services

| Service | Type | Context | Port |
|---------|------|---------|------|
| meku-in-api | Docker web | `services/api` | 8080 |
| meku-in-web | Docker web | `apps/web` | 3000 |
| meku-in-db | Managed Postgres | — | 5432 |

Migrations run **once** with the migration-owner role. API runtime uses restricted `mekuin_app` only.

## Backend env

- `DATABASE_URL` (mekuin_app role)
- `JWT_SECRET`, `AUTH_ENCRYPTION_KEY` (≥32 chars)
- `SESSION_COOKIE_SECURE=true` in production
- `APP_BASE_URL` (public web origin)
- Optional: SMTP_*, GOOGLE_CLIENT_ID, DARAJA_*

## Frontend env (build-time)

- `NEXT_PUBLIC_API_BASE_URL` — must be set as Docker build-arg
- `NEXT_PUBLIC_GOOGLE_CLIENT_ID` — optional

```bash
docker build --build-arg NEXT_PUBLIC_API_BASE_URL=https://api.example.com -t meku-in-web ./apps/web
```

## Sequence

1. Provision Postgres and apply `db/migrations` in order  
2. Deploy API with runtime role URL  
3. Confirm API health  
4. Build web with API public URL and deploy  

See `.env.example` for placeholder names only.
