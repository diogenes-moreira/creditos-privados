#!/usr/bin/env bash
#
# Loads the Cloud Run runtime secrets into Secret Manager for the Prestia
# deployment, and grants the service account that runs Cloud Run permission to
# read them.
#
# Run it yourself: it prompts for the sensitive values so they never appear in
# an argument list, in your shell history, or in a log. Safe to re-run — an
# existing secret gets a new version rather than an error, and you can skip any
# prompt by leaving it blank.
#
#   chmod +x scripts/setup-secrets.sh
#   ./scripts/setup-secrets.sh
#
set -euo pipefail

PROJECT="${PROJECT:-fedegan-prestia-30x}"
REGION="${REGION:-us-central1}"
DB_INSTANCE_NAME="${DB_INSTANCE_NAME:-prestia-db}"
DB_NAME_VALUE="${DB_NAME_VALUE:-creditos}"
DB_USER_VALUE="${DB_USER_VALUE:-creditos}"

CONNECTION_NAME="${PROJECT}:${REGION}:${DB_INSTANCE_NAME}"

# Cloud Run runs as the Compute Engine default service account unless the
# service overrides it, so that is who needs read access to these secrets.
PROJECT_NUMBER="$(gcloud projects describe "$PROJECT" --format='value(projectNumber)')"
RUNTIME_SA="${RUNTIME_SA:-${PROJECT_NUMBER}-compute@developer.gserviceaccount.com}"

echo "Project:        $PROJECT"
echo "Cloud SQL:      $CONNECTION_NAME"
echo "Runtime SA:     $RUNTIME_SA"
echo

# put_secret <NAME> <VALUE>
# Creates the secret if missing, then adds the value as a new version. The
# value arrives on stdin so it is never visible in the process list.
put_secret() {
  local name="$1" value="$2"

  # Secret Manager rejects an empty payload, which would abort the whole script
  # under 'set -e' and leave later secrets uncreated. An intentionally blank
  # value is stored as a single space; the backend trims it back to empty.
  if [[ -z "$value" ]]; then
    value=" "
  fi

  if ! gcloud secrets describe "$name" --project="$PROJECT" >/dev/null 2>&1; then
    gcloud secrets create "$name" \
      --project="$PROJECT" \
      --replication-policy=automatic \
      --quiet >/dev/null
    echo "  created $name"
  fi

  printf '%s' "$value" | gcloud secrets versions add "$name" \
    --project="$PROJECT" \
    --data-file=- \
    --quiet >/dev/null
  echo "  set     $name"

  gcloud secrets add-iam-policy-binding "$name" \
    --project="$PROJECT" \
    --member="serviceAccount:${RUNTIME_SA}" \
    --role=roles/secretmanager.secretAccessor \
    --quiet >/dev/null
}

# put_secret_file <NAME> <PATH>
put_secret_file() {
  local name="$1" path="$2"

  if ! gcloud secrets describe "$name" --project="$PROJECT" >/dev/null 2>&1; then
    gcloud secrets create "$name" \
      --project="$PROJECT" \
      --replication-policy=automatic \
      --quiet >/dev/null
    echo "  created $name"
  fi

  gcloud secrets versions add "$name" \
    --project="$PROJECT" \
    --data-file="$path" \
    --quiet >/dev/null
  echo "  set     $name (from $path)"

  gcloud secrets add-iam-policy-binding "$name" \
    --project="$PROJECT" \
    --member="serviceAccount:${RUNTIME_SA}" \
    --role=roles/secretmanager.secretAccessor \
    --quiet >/dev/null
}

# ask_secret <PROMPT>  -> echoes the typed value, never displaying it
ask_secret() {
  local prompt="$1" value=""
  read -rs -p "$prompt" value </dev/tty
  echo >&2
  printf '%s' "$value"
}

# ---------------------------------------------------------------------------
# 1 · Database connection. These are not sensitive except the password.
# ---------------------------------------------------------------------------
echo "Database connection"

# The backend builds its DSN as "host=<DB_HOST> ... port=<DB_PORT>". Connecting
# through --add-cloudsql-instances exposes a unix socket, so DB_HOST is the
# socket directory and TLS is terminated by the Cloud SQL proxy (sslmode
# disable). DB_INSTANCE is read by config.go but not currently used by any
# connection path; it is set because the deploy mounts it as a required secret.
put_secret DB_HOST     "/cloudsql/${CONNECTION_NAME}"
put_secret DB_PORT     "5432"
put_secret DB_USER     "$DB_USER_VALUE"
put_secret DB_NAME     "$DB_NAME_VALUE"
put_secret DB_SSLMODE  "disable"
put_secret DB_INSTANCE "$CONNECTION_NAME"

echo
echo "Database password for user '${DB_USER_VALUE}'."
echo "Type it once: the script stores it in Secret Manager and, if you let it,"
echo "applies the same value to the Cloud SQL user, so the two cannot drift."
DB_PASS="$(ask_secret "  DB_PASSWORD (hidden): ")"

if [[ -n "$DB_PASS" ]]; then
  put_secret DB_PASSWORD "$DB_PASS"

  read -r -p "  Also set this password on the Cloud SQL user? [Y/n] " apply_db </dev/tty
  if [[ ! "${apply_db:-y}" =~ ^[Nn]$ ]]; then
    # gcloud 576 has no --prompt-for-password on 'users create' and no
    # --password-file anywhere, so the value goes through --password. It is
    # already in a shell variable rather than your history, but it is briefly
    # visible in the process list — fine on a personal machine, worth knowing
    # on a shared one.
    if gcloud sql users list --instance="$DB_INSTANCE_NAME" --project="$PROJECT" \
         --format='value(name)' 2>/dev/null | grep -qx "$DB_USER_VALUE"; then
      gcloud sql users set-password "$DB_USER_VALUE" \
        --instance="$DB_INSTANCE_NAME" \
        --project="$PROJECT" \
        --password="$DB_PASS" \
        --quiet >/dev/null
      echo "  updated Cloud SQL user '$DB_USER_VALUE'"
    else
      gcloud sql users create "$DB_USER_VALUE" \
        --instance="$DB_INSTANCE_NAME" \
        --project="$PROJECT" \
        --password="$DB_PASS" \
        --quiet >/dev/null
      echo "  created Cloud SQL user '$DB_USER_VALUE'"
    fi
  else
    echo "  left the Cloud SQL user untouched"
  fi
else
  echo "  skipped DB_PASSWORD"
fi
unset DB_PASS apply_db

# ---------------------------------------------------------------------------
# 2 · JWT signing key. Generated locally, never shown.
# ---------------------------------------------------------------------------
echo
echo "JWT signing key"
if gcloud secrets describe JWT_SECRET --project="$PROJECT" >/dev/null 2>&1; then
  read -r -p "  JWT_SECRET already exists. Rotate it? Existing sessions die. [y/N] " rotate </dev/tty
else
  rotate="y"
fi
if [[ "${rotate:-n}" =~ ^[Yy]$ ]]; then
  put_secret JWT_SECRET "$(openssl rand -base64 48)"
else
  echo "  kept existing JWT_SECRET"
fi

# ---------------------------------------------------------------------------
# 3 · Twilio, for the WhatsApp OTP.
# ---------------------------------------------------------------------------
echo
echo "Twilio (WhatsApp OTP). Leave blank to skip a value."
echo "Without credentials the OTP falls back to email link, then to console."

TW_SID="$(ask_secret "  TWILIO_ACCOUNT_SID (hidden): ")"
[[ -n "$TW_SID" ]] && put_secret TWILIO_ACCOUNT_SID "$TW_SID" || echo "  skipped TWILIO_ACCOUNT_SID"
unset TW_SID

TW_TOKEN="$(ask_secret "  TWILIO_AUTH_TOKEN (hidden): ")"
[[ -n "$TW_TOKEN" ]] && put_secret TWILIO_AUTH_TOKEN "$TW_TOKEN" || echo "  skipped TWILIO_AUTH_TOKEN"
unset TW_TOKEN

# The sender number and template id are not secret, but the deploy mounts them
# from Secret Manager alongside the rest of the runtime config.
read -r -p "  TWILIO_WHATSAPP_FROM (E.164, e.g. +14155238886): " TW_FROM </dev/tty
[[ -n "$TW_FROM" ]] && put_secret TWILIO_WHATSAPP_FROM "$TW_FROM" || echo "  skipped TWILIO_WHATSAPP_FROM"

read -r -p "  TWILIO_WHATSAPP_TEMPLATE_SID (HX..., blank if none yet): " TW_TPL </dev/tty
# An empty template is valid: Twilio then sends a freeform body, which only
# reaches users inside an open 24h window or in the sandbox.
put_secret TWILIO_WHATSAPP_TEMPLATE_SID "${TW_TPL:-}"

# ---------------------------------------------------------------------------
# 4 · Firebase service-account JSON, for Google sign-in and phone auth.
# ---------------------------------------------------------------------------
echo
echo "Firebase credentials (service-account JSON)."
echo "Only needed for Google sign-in and Firebase phone auth. The deploy mounts"
echo "it as a required secret, so it must exist even if empty."
read -r -p "  Path to JSON (blank to store an empty placeholder): " FB_PATH </dev/tty
if [[ -n "$FB_PATH" ]]; then
  if [[ ! -f "$FB_PATH" ]]; then
    echo "  ERROR: $FB_PATH not found" >&2
    exit 1
  fi
  put_secret_file FIREBASE_CREDENTIALS "$FB_PATH"
else
  put_secret FIREBASE_CREDENTIALS "{}"
  echo "  stored an empty placeholder — Google/phone sign-in will not work"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo
echo "Secrets in $PROJECT:"
gcloud secrets list --project="$PROJECT" --format="table(name.basename(), createTime.date('%Y-%m-%d %H:%M'))"

echo
echo "Done. ${RUNTIME_SA} can read all of the above."
echo "Next: deploy Cloud Run, or push to main if Workload Identity is set up."
