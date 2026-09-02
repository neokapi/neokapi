// Command stripe-provision creates (or confirms) the Stripe objects the Bowrain
// billing code expects: the Pro and Team subscription prices, the credit-pack
// price, the AI-token meter, and the webhook endpoint.
//
// It is idempotent. Every object it owns is addressed by a stable key — prices by
// `lookup_key`, products and the meter by `event_name`/metadata, the webhook by
// URL — so running it twice creates nothing the second time. That matters because
// the alternative is a Stripe account slowly accumulating near-duplicate prices
// nobody can tell apart, and a price is not deletable.
//
// It is an operator tool, not part of any shipped image: run it once per Stripe
// account (test mode first, then live).
//
//	export STRIPE_SECRET_KEY=sk_test_...
//	go run ./bowrain/cmd/stripe-provision -webhook-url https://app.bowrain.cloud/api/webhooks/stripe -env prod
//
// It prints the resulting IDs, and — with -env — the exact `aws ssm put-parameter`
// commands that fill the placeholder parameters epic 002 created. It never writes
// to AWS itself: putting a live secret into an account is a step an operator
// should take deliberately, with the command in front of them.
//
// The contracts it encodes are the ones the runtime reads, and they are not
// cosmetic:
//
//   - Each subscription price carries metadata `bowrain_plan=pro|team`. It is what
//     billing.planFromSubscription reads to decide which plan a subscription is.
//     Without it, plan detection falls back to guessing from the quantity.
//   - The meter's event_name is `ai_token_usage`, matching billing.UsageHooks
//     exactly. A meter created under any other name silently drops every event
//     (meter events are fire-and-forget).
//   - The webhook subscribes to exactly the events the handler dispatches. A
//     missing customer.subscription.created means a new Team subscription is never
//     reconciled from its price metadata.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/stripe/stripe-go/v82"
)

// The lookup keys are this tool's idempotency handles: they are unique per Stripe
// account, so "create if absent" is a single list call, and they survive a price
// being renamed in the dashboard.
const (
	proLookupKey        = "bowrain_pro_monthly"
	teamLookupKey       = "bowrain_team_monthly_seat"
	creditPackLookupKey = "bowrain_credit_pack"

	// Dollar amounts live in Stripe (DECISIONS L4) — which means this tool is the
	// one place in the repo that states them, because it is the thing that puts
	// them into Stripe. Pro $25/mo, Team $20/seat/mo, credit pack $5.
	proMonthlyCents      = 2500
	teamPerSeatCents     = 2000
	creditPackPriceCents = 500

	meterEventName = "ai_token_usage"
)

// webhookEvents is exactly what billing.WebhookHandler.dispatch handles. Keeping
// the endpoint subscribed to only these avoids paying for deliveries we ignore,
// and keeping it subscribed to ALL of them is load-bearing.
var webhookEvents = []string{
	"checkout.session.completed",
	"customer.subscription.created",
	"customer.subscription.updated",
	"customer.subscription.deleted",
	"invoice.paid",
	"invoice.payment_failed",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	webhookURL := flag.String("webhook-url", "", "public URL of the Bowrain webhook endpoint (e.g. https://app.bowrain.cloud/api/webhooks/stripe); omit to skip webhook setup")
	env := flag.String("env", "", "environment name (e.g. prod): prints the aws ssm put-parameter commands for /bowrain/<env>/…")
	flag.Parse()

	key := os.Getenv("STRIPE_SECRET_KEY")
	if !billing.IsStripeSecretKey(key) {
		return errors.New("STRIPE_SECRET_KEY must be set to a Stripe secret key (sk_… or rk_…)")
	}
	mode := "TEST"
	if strings.HasPrefix(key, "sk_live_") || strings.HasPrefix(key, "rk_live_") {
		mode = "LIVE"
	}
	fmt.Printf("Provisioning Bowrain billing in %s mode\n\n", mode)

	ctx := context.Background()
	sc := stripe.NewClient(key)

	proProduct, err := ensureProduct(ctx, sc, "Bowrain Pro", "pro")
	if err != nil {
		return err
	}
	teamProduct, err := ensureProduct(ctx, sc, "Bowrain Team", "team")
	if err != nil {
		return err
	}
	packProduct, err := ensureProduct(ctx, sc, "Bowrain Credit Pack", "credit_pack")
	if err != nil {
		return err
	}

	// The `bowrain_plan` metadata on the PRICE (not the product) is what the
	// webhook reads — Stripe puts the price on the subscription item, not the
	// product.
	proPrice, err := ensurePrice(ctx, sc, priceSpec{
		lookupKey: proLookupKey,
		product:   proProduct,
		cents:     proMonthlyCents,
		recurring: true,
		metadata:  map[string]string{"bowrain_plan": string(billing.PlanPro)},
	})
	if err != nil {
		return err
	}
	teamPrice, err := ensurePrice(ctx, sc, priceSpec{
		lookupKey: teamLookupKey,
		product:   teamProduct,
		cents:     teamPerSeatCents,
		recurring: true,
		metadata:  map[string]string{"bowrain_plan": string(billing.PlanTeam)},
	})
	if err != nil {
		return err
	}
	packPrice, err := ensurePrice(ctx, sc, priceSpec{
		lookupKey: creditPackLookupKey,
		product:   packProduct,
		cents:     creditPackPriceCents,
		recurring: false,
		// The pack's credit size is the server's constant (billing.CreditPackCredits);
		// stamping it here keeps the Stripe object self-describing for whoever reads
		// the dashboard later, and makes a mismatch visible.
		metadata: map[string]string{"bowrain_credits": strconv.Itoa(billing.CreditPackCredits)},
	})
	if err != nil {
		return err
	}

	if err := ensureMeter(ctx, sc); err != nil {
		return err
	}

	var webhookSecret string
	if *webhookURL != "" {
		webhookSecret, err = ensureWebhook(ctx, sc, *webhookURL)
		if err != nil {
			return err
		}
	}

	fmt.Println("\n--- Configuration ---")
	fmt.Printf("STRIPE_PRO_PRICE_ID=%s\n", proPrice)
	fmt.Printf("STRIPE_TEAM_PRICE_ID=%s\n", teamPrice)
	fmt.Printf("STRIPE_CREDIT_PRICE_ID=%s\n", packPrice)
	if webhookSecret != "" {
		fmt.Printf("STRIPE_WEBHOOK_SECRET=%s\n", webhookSecret)
	} else if *webhookURL == "" {
		fmt.Println("STRIPE_WEBHOOK_SECRET=  (no -webhook-url given; for local testing use `stripe listen`, which prints its own secret)")
	} else {
		fmt.Println("STRIPE_WEBHOOK_SECRET=  (endpoint already existed: Stripe only reveals the signing secret at creation; read it from the dashboard, or delete and re-create the endpoint)")
	}

	if *env != "" {
		printSSM(*env, proPrice, teamPrice, packPrice, webhookSecret)
	}

	fmt.Println("\nRemaining dashboard steps (not API-configurable):")
	fmt.Println("  - Billing → Automatic collection: enable smart retries, and cancel the")
	fmt.Println("    subscription after the final retry. That is what turns a failed payment")
	fmt.Println("    into customer.subscription.deleted, which is the only thing that")
	fmt.Println("    downgrades a past_due workspace (AD-018).")
	fmt.Println("  - Customer portal: allow customers to update payment methods, cancel, and")
	fmt.Println("    change the Team seat quantity (the quantity IS the seat count).")
	return nil
}

// ensureProduct finds the product tagged with the given bowrain_type, or creates
// it. Products are matched on metadata rather than name so an operator can rename
// them in the dashboard without this tool creating a duplicate.
func ensureProduct(ctx context.Context, sc *stripe.Client, name, kind string) (string, error) {
	for p, err := range sc.V1Products.List(ctx, &stripe.ProductListParams{
		Limit: stripe.Int64(100),
	}) {
		if err != nil {
			return "", fmt.Errorf("list products: %w", err)
		}
		if p.Metadata["bowrain_type"] == kind {
			fmt.Printf("product  %-20s exists  %s\n", kind, p.ID)
			return p.ID, nil
		}
	}

	p, err := sc.V1Products.Create(ctx, &stripe.ProductCreateParams{
		Name:     stripe.String(name),
		Metadata: map[string]string{"bowrain_type": kind},
	})
	if err != nil {
		return "", fmt.Errorf("create product %s: %w", kind, err)
	}
	fmt.Printf("product  %-20s created %s\n", kind, p.ID)
	return p.ID, nil
}

type priceSpec struct {
	lookupKey string
	product   string
	cents     int64
	recurring bool
	metadata  map[string]string
}

// ensurePrice finds the price with the spec's lookup_key, or creates it.
//
// An existing price is returned as-is and never mutated: Stripe prices are
// immutable in amount, and silently creating a second one because the amount
// changed would leave existing subscribers on the old price with no signal. If
// the amount ever needs to change, that is a deliberate migration — create the
// new price, move subscribers, and retire the old lookup_key.
func ensurePrice(ctx context.Context, sc *stripe.Client, spec priceSpec) (string, error) {
	for p, err := range sc.V1Prices.List(ctx, &stripe.PriceListParams{
		LookupKeys: []*string{stripe.String(spec.lookupKey)},
		Limit:      stripe.Int64(1),
	}) {
		if err != nil {
			return "", fmt.Errorf("list prices for %s: %w", spec.lookupKey, err)
		}
		fmt.Printf("price    %-20s exists  %s (%s)\n", spec.lookupKey, p.ID, formatAmount(p.UnitAmount))
		if got := p.Metadata["bowrain_plan"]; got != spec.metadata["bowrain_plan"] {
			fmt.Printf("  WARNING: price metadata bowrain_plan=%q, expected %q, so plan detection will fall back to guessing\n",
				got, spec.metadata["bowrain_plan"])
		}
		if p.UnitAmount != spec.cents {
			fmt.Printf("  WARNING: price is %s, expected %s. Stripe prices are immutable; migrate deliberately if this is wrong\n",
				formatAmount(p.UnitAmount), formatAmount(spec.cents))
		}
		return p.ID, nil
	}

	params := &stripe.PriceCreateParams{
		Product:    stripe.String(spec.product),
		LookupKey:  stripe.String(spec.lookupKey),
		Currency:   stripe.String(string(stripe.CurrencyUSD)),
		UnitAmount: &spec.cents,
		Metadata:   spec.metadata,
	}
	if spec.recurring {
		// `licensed` (the default) bills the quantity set on the subscription — for
		// Team that quantity is the seat count. `metered` would bill reported usage
		// instead, which is not how seats work.
		params.Recurring = &stripe.PriceCreateRecurringParams{
			Interval:  stripe.String(string(stripe.PriceRecurringIntervalMonth)),
			UsageType: stripe.String(string(stripe.PriceRecurringUsageTypeLicensed)),
		}
	}

	p, err := sc.V1Prices.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("create price %s: %w", spec.lookupKey, err)
	}
	fmt.Printf("price    %-20s created %s (%s)\n", spec.lookupKey, p.ID, formatAmount(spec.cents))
	return p.ID, nil
}

// ensureMeter creates the AI-token meter if it does not exist.
//
// The meter must be named `ai_token_usage` because that is the event name
// billing.UsageHooks reports under. It aggregates the `value` payload key by sum
// and maps events to customers by `stripe_customer_id` — both are the payload keys
// StripeClient.ReportMeterEvent actually sends. The meter prices nothing (credits
// are the billing unit); it exists so token consumption is visible in Stripe
// alongside the revenue.
func ensureMeter(ctx context.Context, sc *stripe.Client) error {
	for m, err := range sc.V1BillingMeters.List(ctx, &stripe.BillingMeterListParams{
		Status: stripe.String("active"),
		Limit:  stripe.Int64(100),
	}) {
		if err != nil {
			return fmt.Errorf("list meters: %w", err)
		}
		if m.EventName == meterEventName {
			fmt.Printf("meter    %-20s exists  %s\n", meterEventName, m.ID)
			return nil
		}
	}

	m, err := sc.V1BillingMeters.Create(ctx, &stripe.BillingMeterCreateParams{
		DisplayName: stripe.String("AI token usage"),
		EventName:   stripe.String(meterEventName),
		DefaultAggregation: &stripe.BillingMeterCreateDefaultAggregationParams{
			Formula: stripe.String("sum"),
		},
		ValueSettings: &stripe.BillingMeterCreateValueSettingsParams{
			EventPayloadKey: stripe.String("value"),
		},
		CustomerMapping: &stripe.BillingMeterCreateCustomerMappingParams{
			EventPayloadKey: stripe.String("stripe_customer_id"),
			Type:            stripe.String("by_id"),
		},
	})
	if err != nil {
		return fmt.Errorf("create meter: %w", err)
	}
	fmt.Printf("meter    %-20s created %s\n", meterEventName, m.ID)
	return nil
}

// ensureWebhook creates the webhook endpoint if one does not already point at the
// URL, and returns its signing secret. Stripe reveals the secret only at creation,
// so an existing endpoint yields an empty secret and the operator is told where to
// find it.
//
// An existing endpoint whose event list has drifted from what the handler
// dispatches is repaired in place: a missing event type is a silent failure (the
// event simply never arrives), which is exactly the kind of thing nobody notices
// until a customer's plan is wrong.
func ensureWebhook(ctx context.Context, sc *stripe.Client, url string) (string, error) {
	for w, err := range sc.V1WebhookEndpoints.List(ctx, &stripe.WebhookEndpointListParams{
		Limit: stripe.Int64(100),
	}) {
		if err != nil {
			return "", fmt.Errorf("list webhook endpoints: %w", err)
		}
		if w.URL != url {
			continue
		}
		fmt.Printf("webhook  %-20s exists  %s\n", "endpoint", w.ID)
		if missing := missingEvents(w.EnabledEvents); len(missing) > 0 {
			fmt.Printf("  repairing: subscribing to missing events %v\n", missing)
			if _, err := sc.V1WebhookEndpoints.Update(ctx, w.ID, &stripe.WebhookEndpointUpdateParams{
				EnabledEvents: stripe.StringSlice(webhookEvents),
			}); err != nil {
				return "", fmt.Errorf("update webhook endpoint: %w", err)
			}
		}
		return "", nil
	}

	w, err := sc.V1WebhookEndpoints.Create(ctx, &stripe.WebhookEndpointCreateParams{
		URL:           stripe.String(url),
		EnabledEvents: stripe.StringSlice(webhookEvents),
		Description:   stripe.String("Bowrain billing"),
	})
	if err != nil {
		return "", fmt.Errorf("create webhook endpoint: %w", err)
	}
	fmt.Printf("webhook  %-20s created %s\n", "endpoint", w.ID)
	return w.Secret, nil
}

// missingEvents returns the handled event types the endpoint is not subscribed to.
func missingEvents(enabled []string) []string {
	have := make(map[string]bool, len(enabled))
	for _, e := range enabled {
		have[e] = true
	}
	var missing []string
	for _, want := range webhookEvents {
		if !have[want] && !have["*"] {
			missing = append(missing, want)
		}
	}
	return missing
}

// printSSM emits the commands that fill the placeholder parameters Terraform
// created (epic 002 seeds them as "CHANGEME"; the server treats a placeholder as
// "not configured", so billing stays off until these are run).
func printSSM(env, proPrice, teamPrice, packPrice, webhookSecret string) {
	fmt.Printf("\n--- Fill the SSM parameters for %s ---\n", env)
	put := func(name, value string) {
		fmt.Printf("aws ssm put-parameter --overwrite --type SecureString --name /bowrain/%s/%s --value %s\n", env, name, value)
	}
	put("stripe-secret-key", "$STRIPE_SECRET_KEY")
	put("stripe-pro-price-id", proPrice)
	put("stripe-team-price-id", teamPrice)
	put("stripe-credit-price-id", packPrice)
	if webhookSecret != "" {
		put("stripe-webhook-secret", webhookSecret)
	} else {
		put("stripe-webhook-secret", "<signing secret from the Stripe dashboard>")
	}
	fmt.Println("\nThen redeploy the server and worker so the tasks pick up the new values.")
}

func formatAmount(cents int64) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}
