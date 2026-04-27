#!/bin/sh
# Seed a test user in the Choresy app
# Creates test@choresy.local / "correct horse battery"
set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
EMAIL="test@choresy.local"
PASSWORD="correct horse battery"
APP_BASE_URL="${APP_BASE_URL:-http://localhost:8080}"

echo "=== Seed: Getting CSRF token ==="
CSRF_TOKEN=$(curl -sf -c - "${APP_BASE_URL}/api/auth/login" 2>/dev/null | grep choresy_csrf | awk '{print $NF}')
echo "CSRF Token: ${CSRF_TOKEN}"

echo "=== Seed: Registering user ==="
REGISTER_RESPONSE=$(curl -sf -X POST "${BASE_URL}/api/auth/register" \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: ${CSRF_TOKEN}" \
  -b "choresy_csrf=${CSRF_TOKEN}" \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}" 2>&1 || true)

if echo "${REGISTER_RESPONSE}" | grep -q '"user"'; then
  echo "User registered successfully."
elif echo "${REGISTER_RESPONSE}" | grep -q 'already exists'; then
  echo "User already exists (skipping)."
else
  echo "Registration response: ${REGISTER_RESPONSE}"
fi

echo "=== Seed: Complete ==="
