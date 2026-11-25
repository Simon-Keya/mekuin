# Authentication implementation increment

## Implemented

- Registration creates the owner workspace but does not issue an authenticated session. A six-digit email verification code is issued and must be verified before password login.
- Email verification codes and password reset codes are stored as HMAC-SHA256 digests keyed by `JWT_SECRET`, expire after 15 minutes, are single-use, and have a maximum of ten attempts. A 60-second per-user/per-purpose resend cooldown is enforced.
- Login rejects accounts whose `email_verified_at` is null. Existing accounts with a null verification timestamp must complete the one-time email verification after deployment; they can request a code through `/api/v1/auth/email/resend` and use `/verify-email`.
- Forgot-password uses a non-enumerating 202 response; reset changes the bcrypt password and revokes every existing session.
- Optional TOTP MFA setup, enable, login challenge and disable flows are implemented. MFA secrets are encrypted with AES-256-GCM using a key derived from `AUTH_ENCRYPTION_KEY`; the encryption key is never stored in PostgreSQL.
- Google Identity Services ID-token sign-in is implemented for an already registered account. The API verifies the token with Google's tokeninfo endpoint, checks the configured audience, issuer and verified email, and then uses the existing account's MFA policy. Google does not create a first workspace/account in this increment.
- Login, verification, recovery, Google sign-in and MFA routes are included in the OpenAPI contract.

## Required deployment configuration

- `JWT_SECRET`: at least 32 characters of high-entropy random material. Used to HMAC one-time codes.
- `AUTH_ENCRYPTION_KEY`: at least 32 characters of high-entropy random material. Used for authenticated encryption of TOTP secrets. Keep stable and back it up securely; losing it makes existing MFA secrets unrecoverable.
- `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME` (or `SMTP_USER`), `SMTP_PASSWORD`, `SMTP_FROM`: mail delivery for verification and password recovery.
- `GOOGLE_CLIENT_ID`: OAuth client ID matching `NEXT_PUBLIC_GOOGLE_CLIENT_ID` in the web deployment. Configure the corresponding authorized JavaScript origins in Google Cloud.
- `NEXT_PUBLIC_API_BASE_URL`: API origin used by the browser.

Do not put secrets in `NEXT_PUBLIC_*` variables. Use the deployment secret manager. Rotate secrets through a controlled procedure; changing `AUTH_ENCRYPTION_KEY` without re-encrypting MFA secrets invalidates MFA for existing users.

## Integration limitations still outstanding

- Daraja dispatch still uses process-level deployment credentials; business-specific encrypted credential storage and tenant-aware dispatch are not implemented. Do not claim per-business Daraja isolation.
- eTIMS remains configuration/status only; KRA invoice transmission is not implemented.
- Card payments remain manual-reference only; no acquiring gateway is integrated.
- Google sign-in is for existing registered users. New-user Google workspace provisioning/linking is not implemented.

## Acceptance still required

Run migration 050 on PostgreSQL 17 and execute `db/tests/auth_flows_smoke.sql`. CI now starts Mailpit and exercises the unverified-login rejection plus email-code verification in the authenticated API smoke scripts. Run Go 1.24.13 tests, race tests and vet. Run Next lint/build and authenticated browser acceptance for registration verification, resend cooldown, verified login, Google login, MFA setup/enable/login/disable, password recovery, session revocation and suspended-account rejection. SMTP and Google flows require real sandbox/deployment credentials to verify end-to-end.
