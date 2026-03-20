#!/usr/bin/env bash
#
# stripe-seed.sh — Idempotent Stripe product/price/meter setup for Bowrain.
#
# Prerequisites:
#   - Stripe CLI installed and authenticated (stripe login)
#   - STRIPE_SECRET_KEY set (or use stripe CLI's default key)
#
# Usage:
#   ./scripts/stripe-seed.sh
#
# This creates the products, prices, and meters needed for Bowrain billing.
# Safe to run multiple times — checks for existing resources by metadata.

set -euo pipefail

echo "=== Bowrain Stripe Seed ==="

# Helper: find or create a product by metadata key.
find_or_create_product() {
  local name="$1"
  local metadata_key="$2"
  local metadata_value="$3"

  # Search for existing product.
  existing=$(stripe products search --query "metadata['$metadata_key']:'$metadata_value'" --limit 1 2>/dev/null | jq -r '.data[0].id // empty')
  if [ -n "$existing" ]; then
    echo "  Product '$name' already exists: $existing"
    echo "$existing"
    return
  fi

  id=$(stripe products create \
    --name "$name" \
    -d "metadata[$metadata_key]=$metadata_value" \
    2>/dev/null | jq -r '.id')
  echo "  Created product '$name': $id"
  echo "$id"
}

# Helper: find or create a price for a product.
find_or_create_price() {
  local product_id="$1"
  local amount="$2"
  local interval="$3"
  local metadata_key="$4"
  local metadata_value="$5"

  existing=$(stripe prices search --query "product:'$product_id' AND metadata['$metadata_key']:'$metadata_value'" --limit 1 2>/dev/null | jq -r '.data[0].id // empty')
  if [ -n "$existing" ]; then
    echo "  Price already exists: $existing"
    echo "$existing"
    return
  fi

  if [ "$interval" = "one_time" ]; then
    id=$(stripe prices create \
      --product "$product_id" \
      -d "unit_amount=$amount" \
      -d "currency=usd" \
      -d "metadata[$metadata_key]=$metadata_value" \
      2>/dev/null | jq -r '.id')
  else
    id=$(stripe prices create \
      --product "$product_id" \
      -d "unit_amount=$amount" \
      -d "currency=usd" \
      -d "recurring[interval]=$interval" \
      -d "metadata[$metadata_key]=$metadata_value" \
      2>/dev/null | jq -r '.id')
  fi
  echo "  Created price: $id"
  echo "$id"
}

echo ""
echo "--- Products ---"
PRO_PRODUCT=$(find_or_create_product "Bowrain Pro" "bowrain_type" "pro")
TEAM_PRODUCT=$(find_or_create_product "Bowrain Team (per seat)" "bowrain_type" "team")
CREDITS_PRODUCT=$(find_or_create_product "Bowrain Credit Pack" "bowrain_type" "credits")

echo ""
echo "--- Prices ---"
echo "Pro monthly ($25/mo):"
PRO_PRICE=$(find_or_create_price "$PRO_PRODUCT" 2500 month "bowrain_plan" "pro")

echo "Team monthly ($15/seat/mo):"
TEAM_PRICE=$(find_or_create_price "$TEAM_PRODUCT" 1500 month "bowrain_plan" "team")

echo "Credit pack ($5, one-time):"
CREDIT_PRICE=$(find_or_create_price "$CREDITS_PRODUCT" 500 one_time "bowrain_type" "credit_pack")

echo ""
echo "--- Environment Variables ---"
echo "Add these to your .env or deployment config:"
echo ""
echo "STRIPE_PRO_PRICE_ID=$PRO_PRICE"
echo "STRIPE_TEAM_PRICE_ID=$TEAM_PRICE"
echo "STRIPE_CREDIT_PRICE_ID=$CREDIT_PRICE"
echo ""
echo "=== Done ==="
