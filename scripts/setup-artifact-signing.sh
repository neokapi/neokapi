#!/usr/bin/env bash
#
# Set up Azure Artifact Signing (formerly Trusted Signing) for neokapi Windows
# code signing, and wire GitHub Actions OIDC. Runs in TWO phases around the one
# step that cannot be scripted: portal Identity Validation.
#
#   ./setup-artifact-signing.sh 1
#       → registers the RP, creates the resource group + signing account
#       → then you complete Identity Validation in the portal (see printed steps)
#
#   IDENTITY_VALIDATION_ID=<id-from-portal> ./setup-artifact-signing.sh 2
#       → certificate profile + GitHub OIDC app + federated credential + RBAC
#       → prints the `gh secret set` commands to finish wiring CI
#
# ─── PUBLISHER IDENTITY ───────────────────────────────────────────────────────
# The Windows "verified publisher" name is sourced from the Azure *billing
# account's* legal entity — NOT the subscription name. Every billing account in
# this tenant is "Skissefabrikken AS", so any subscription here yields a
# Skissefabrikken AS publisher, matching the macOS Developer ID. Point
# SUBSCRIPTION_ID at whichever subscription should host the signing resource;
# the validated identity is the same either way.
# ──────────────────────────────────────────────────────────────────────────────
set -euo pipefail

#### config — edit these ######################################################
SUBSCRIPTION_ID="${SUBSCRIPTION_ID:-7adbf563-3201-43a1-91a9-aaa7ed0bcfdc}"  # subscription to host the signing account (under the Skissefabrikken AS billing account)
LOCATION="${LOCATION:-westeurope}"
RG="${RG:-rg-codesigning}"
ACCOUNT="${ACCOUNT:-skissefabrikken-signing}"   # 3-24 chars, must be globally unique
PROFILE="${PROFILE:-skissefabrikken-prod}"
APP_NAME="${APP_NAME:-gh-neokapi-signing}"
GH_REPO="${GH_REPO:-neokapi/neokapi}"
GH_ENVIRONMENT="${GH_ENVIRONMENT:-release}"     # OIDC subject ties to this GH Actions environment
###############################################################################

PHASE="${1:-1}"
command -v az >/dev/null || { echo "az CLI not found"; exit 1; }

az account set --subscription "$SUBSCRIPTION_ID"
echo ">> Subscription : $(az account show --query name -o tsv) ($SUBSCRIPTION_ID)"
echo ">> Tenant       : $(az account show --query tenantId -o tsv)"
echo ">> Signed in as : $(az ad signed-in-user show --query userPrincipalName -o tsv 2>/dev/null || echo '(service principal)')"
read -r -p ">> Confirm this subscription rolls up to the Skissefabrikken AS billing account? [y/N] " ok
[ "$ok" = "y" ] || [ "$ok" = "Y" ] || { echo "Aborting — set SUBSCRIPTION_ID / log in to the right tenant first."; exit 1; }

phase1() {
  echo ">> Registering Microsoft.CodeSigning provider (free, idempotent)..."
  az provider register --namespace Microsoft.CodeSigning --wait
  az extension show -n artifact-signing >/dev/null 2>&1 || az extension add -n artifact-signing

  echo ">> Resource group + signing account in $LOCATION (Basic SKU — \$9.99/mo)..."
  az group create -n "$RG" -l "$LOCATION" -o none
  az artifact-signing create -n "$ACCOUNT" -g "$RG" -l "$LOCATION" --sku Basic -o none

  # Grant the current user the role needed to create the identity validation
  # in the portal. Distinct from the signer role (assigned to CI in phase 2).
  # Requires Owner / User Access Administrator on the account scope; RBAC can
  # take a few minutes to propagate.
  ACCOUNT_ID="$(az artifact-signing show -n "$ACCOUNT" -g "$RG" --query id -o tsv)"
  MY_OID="$(az ad signed-in-user show --query id -o tsv 2>/dev/null || true)"
  if [ -n "$MY_OID" ]; then
    echo ">> Granting you 'Artifact Signing Identity Verifier' on the account..."
    az role assignment create \
      --assignee-object-id "$MY_OID" --assignee-principal-type User \
      --role "Artifact Signing Identity Verifier" --scope "$ACCOUNT_ID" -o none \
      && echo "   done (allow a few minutes for it to take effect)" \
      || echo "   ⚠ could not self-assign — ask an Owner/User Access Administrator to grant you 'Artifact Signing Identity Verifier' on $ACCOUNT"
  fi

  cat <<EOM

✅ Account '$ACCOUNT' created.

NEXT — portal only (cannot be scripted; prerequisite for the certificate profile):
  1. portal.azure.com → Artifact Signing → account '$ACCOUNT'
  2. Identity validations → + Add → type **Public Trust**
       • Submit as the legal entity **Skissefabrikken AS** (org details are
         auto-sourced from this subscription's billing account).
       • A third-party validator reviews it — allow a few business days.
  3. When status = Completed, copy the Identity Validation ID, then run:

       IDENTITY_VALIDATION_ID=<id> $0 2
EOM
}

phase2() {
  : "${IDENTITY_VALIDATION_ID:?set IDENTITY_VALIDATION_ID (from the completed portal identity validation)}"
  az extension show -n artifact-signing >/dev/null 2>&1 || az extension add -n artifact-signing

  echo ">> Creating PublicTrust certificate profile '$PROFILE'..."
  az artifact-signing certificate-profile create \
    --account "$ACCOUNT" -g "$RG" -n "$PROFILE" \
    --profile-type PublicTrust \
    --identity-validation-id "$IDENTITY_VALIDATION_ID" -o none

  ACCOUNT_ID="$(az artifact-signing show -n "$ACCOUNT" -g "$RG" --query id -o tsv)"
  ENDPOINT="$(az artifact-signing show -n "$ACCOUNT" -g "$RG" --query accountUri -o tsv 2>/dev/null)"
  [ -n "$ENDPOINT" ] || ENDPOINT="https://weu.codesigning.azure.net/"

  echo ">> GitHub OIDC app registration + federated credential..."
  APP_ID="$(az ad app create --display-name "$APP_NAME" --query appId -o tsv)"
  az ad sp create --id "$APP_ID" -o none 2>/dev/null || true
  SP_OID="$(az ad sp show --id "$APP_ID" --query id -o tsv)"
  az ad app federated-credential create --id "$APP_ID" --parameters "{
    \"name\": \"neokapi-${GH_ENVIRONMENT}\",
    \"issuer\": \"https://token.actions.githubusercontent.com\",
    \"subject\": \"repo:${GH_REPO}:environment:${GH_ENVIRONMENT}\",
    \"audiences\": [\"api://AzureADTokenExchange\"]
  }" -o none

  echo ">> Assigning the signer role on the account..."
  ROLE="$(az role definition list --query "[?contains(roleName,'Certificate Profile Signer')].roleName | [0]" -o tsv)"
  ROLE="${ROLE:-Artifact Signing Certificate Profile Signer}"
  az role assignment create \
    --assignee-object-id "$SP_OID" --assignee-principal-type ServicePrincipal \
    --role "$ROLE" --scope "$ACCOUNT_ID" -o none

  TENANT_ID="$(az account show --query tenantId -o tsv)"
  cat <<EOM

✅ DONE. Set these GitHub repo secrets (run from anywhere with gh authed):

  gh secret set AZURE_CLIENT_ID         --repo ${GH_REPO} --body "${APP_ID}"
  gh secret set AZURE_TENANT_ID         --repo ${GH_REPO} --body "${TENANT_ID}"
  gh secret set AZURE_SUBSCRIPTION_ID   --repo ${GH_REPO} --body "${SUBSCRIPTION_ID}"
  gh secret set AZURE_CODESIGN_ENDPOINT --repo ${GH_REPO} --body "${ENDPOINT}"
  gh secret set AZURE_CODESIGN_ACCOUNT  --repo ${GH_REPO} --body "${ACCOUNT}"
  gh secret set AZURE_CODESIGN_PROFILE  --repo ${GH_REPO} --body "${PROFILE}"

Role assigned: '${ROLE}'
Signing role used the '${GH_ENVIRONMENT}' GitHub environment as the OIDC subject —
gate the Windows signing job(s) with:  environment: ${GH_ENVIRONMENT}
EOM
}

case "$PHASE" in
  1) phase1 ;;
  2) phase2 ;;
  *) echo "usage: $0 [1|2]"; exit 1 ;;
esac
