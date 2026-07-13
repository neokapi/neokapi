package billing

import "strings"

// Stripe identifier prefixes. Stripe's IDs are prefixed by kind, and the prefix
// is stable across live and test mode (`sk_live_…` / `sk_test_…`).
const (
	secretKeyPrefix      = "sk_" // standard secret key
	restrictedKeyPrefix  = "rk_" // restricted key (scoped secret key)
	webhookSecretPrefix  = "whsec_"
	pricePrefix          = "price_"
	placeholderSSMSecret = "CHANGEME"
)

// IsStripeSecretKey reports whether v looks like a Stripe secret (or restricted)
// key.
//
// "Non-empty" is not the same as "configured". Terraform creates the Stripe SSM
// parameters as literal "CHANGEME" placeholders (epic 002, modules/secrets), and
// STRIPE_SECRET_KEY being set is what the platform reads as *this is a billed
// deployment*: it enables the checkout surface, constructs the Stripe client,
// turns on worker credit metering, and makes a missing BOWRAIN_SECRETS_KEY a hard
// startup failure. A placeholder passing that test gives the worst of both worlds
// — the product advertises billing while every Stripe call 401s. Callers treat a
// value that fails this check as absent.
func IsStripeSecretKey(v string) bool {
	return strings.HasPrefix(v, secretKeyPrefix) || strings.HasPrefix(v, restrictedKeyPrefix)
}

// IsStripeWebhookSecret reports whether v looks like a Stripe webhook signing
// secret. A placeholder here would fail every signature verification with a 400,
// which Stripe retries — better to report the endpoint as unconfigured.
func IsStripeWebhookSecret(v string) bool {
	return strings.HasPrefix(v, webhookSecretPrefix)
}

// IsStripePriceID reports whether v looks like a Stripe price ID. A placeholder
// price makes a plan simply not purchasable (GET /billing/plans reports it), which
// is an honest degradation; sending it to Stripe would 400 at checkout instead.
func IsStripePriceID(v string) bool {
	return strings.HasPrefix(v, pricePrefix)
}

// IsPlaceholder reports whether v is the placeholder value Terraform seeds the
// Stripe SSM parameters with. Callers use it to distinguish "an operator has not
// provisioned Stripe yet" (expected before epic 005's provisioning step, worth a
// single informational log) from "an operator set something wrong" (worth a
// warning).
func IsPlaceholder(v string) bool {
	return v == placeholderSSMSecret
}
