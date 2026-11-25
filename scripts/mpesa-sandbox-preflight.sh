#!/usr/bin/env sh
set -eu

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
pass() { printf 'OK: %s\n' "$1"; }

[ "${DARAJA_ENV:-}" = "sandbox" ] || fail "DARAJA_ENV must be explicitly set to sandbox"
for name in DARAJA_CONSUMER_KEY DARAJA_CONSUMER_SECRET DARAJA_SHORTCODE DARAJA_PASSKEY DARAJA_CALLBACK_URL; do
  eval "value=\${$name:-}"
  [ -n "$value" ] || fail "$name is not configured"
done
case "$DARAJA_CALLBACK_URL" in
  https://*) ;;
  *) fail "DARAJA_CALLBACK_URL must use HTTPS" ;;
esac
case "$DARAJA_CALLBACK_URL" in
  *'/daraja/stk/callback') ;;
  *) fail "DARAJA_CALLBACK_URL must end in /daraja/stk/callback" ;;
esac
case "$DARAJA_CALLBACK_URL" in
  *localhost*|*127.0.0.1*|*0.0.0.0*|*' '* ) fail "DARAJA_CALLBACK_URL must be a public HTTPS URL" ;;
esac
if [ -n "${DARAJA_BASE_URL:-}" ]; then
  case "$DARAJA_BASE_URL" in
    https://sandbox.safaricom.co.ke|https://sandbox.safaricom.co.ke/ ) ;;
    *) fail "DARAJA_BASE_URL override is not the standard Safaricom sandbox host" ;;
  esac
fi
pass "sandbox mode explicitly selected"
pass "required Daraja secrets are present (values not displayed)"
pass "callback URL has HTTPS and expected callback path"
pass "provider endpoint is the standard sandbox host"
printf '\nPreflight only validates configuration shape. It does not authenticate with Safaricom, prove callback reachability, or execute a payment.\n'
