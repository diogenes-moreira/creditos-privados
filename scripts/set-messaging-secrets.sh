#!/bin/bash
# Loads the SendGrid and Twilio credentials into Secret Manager.
#
# Keys are typed without echo and go straight to Secret Manager; nothing is
# printed or written to disk. Leaving a value empty keeps the current one.
# The service picks the new versions up on its next revision.
#
# Usage: ./scripts/set-messaging-secrets.sh
set -euo pipefail

PROJECT="${PROJECT:-fedegan-prestia-30x}"
ACCOUNT="${ACCOUNT:-b@cuppogan.com}"

put() { # name value
  printf '%s' "$2" | gcloud secrets versions add "$1" --data-file=- \
    --project="$PROJECT" --account="$ACCOUNT" >/dev/null
  echo "  $1: actualizado"
}

ask() { # name prompt hidden(0/1) [default]
  local value
  if [ "$3" = 1 ]; then
    read -rs -p "$2: " value </dev/tty; echo
  else
    read -r -p "$2${4:+ [$4]}: " value </dev/tty
    value="${value:-${4:-}}"
  fi
  if [ -z "$value" ]; then echo "  $1: sin cambios"; else put "$1" "$value"; fi
}

echo "Proyecto: $PROJECT  |  cuenta: $ACCOUNT"
echo
echo "== SendGrid (código de acceso por correo)"
ask SENDGRID_API_KEY    "API key de SendGrid (permiso Mail Send)" 1
ask SENDGRID_FROM_EMAIL "Remitente verificado" 0 "hola@cuppogan.com"
echo
echo "== Twilio (código de acceso por WhatsApp)"
ask TWILIO_ACCOUNT_SID           "Account SID (empieza con AC)" 1
ask TWILIO_AUTH_TOKEN            "Auth Token" 1
ask TWILIO_WHATSAPP_FROM         "Número remitente de WhatsApp, con código de país (+57...)" 0
ask TWILIO_WHATSAPP_TEMPLATE_SID "SID de la plantilla de autenticación (empieza con HX)" 0
echo
echo "Listo. Avisale a Claude para que despliegue y pruebe el envío."
