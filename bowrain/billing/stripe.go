package billing

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"time"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/invoice"
)

// StripeClient wraps the Stripe SDK for billing operations.
type StripeClient struct {
	sc *stripe.Client
}

// NewStripeClient creates a StripeClient with the given secret key.
func NewStripeClient(secretKey string) *StripeClient {
	stripe.Key = secretKey
	return &StripeClient{
		sc: stripe.NewClient(secretKey),
	}
}

// CreateCustomer creates a Stripe customer for a workspace.
func (c *StripeClient) CreateCustomer(_ context.Context, workspaceID, email, name string) (string, error) {
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
		Name:  stripe.String(name),
	}
	params.AddMetadata("workspace_id", workspaceID)

	cust, err := customer.New(params)
	if err != nil {
		return "", fmt.Errorf("create stripe customer: %w", err)
	}
	return cust.ID, nil
}

// CheckoutOptions configures optional checkout behavior.
type CheckoutOptions struct {
	Metadata  map[string]string
	TrialDays int64 // 0 = no trial
}

// CreateCheckoutSession creates a Stripe Checkout session for subscribing to a plan.
// metadata is attached to the session (e.g. workspace_id for webhook routing).
func (c *StripeClient) CreateCheckoutSession(_ context.Context, customerID, priceID, successURL, cancelURL string, metadata ...map[string]string) (string, error) {
	opts := CheckoutOptions{}
	if len(metadata) > 0 {
		opts.Metadata = metadata[0]
	}
	return c.CreateCheckoutSessionWithOptions(customerID, priceID, successURL, cancelURL, opts)
}

// CreateCheckoutSessionWithOptions creates a Stripe Checkout session with full options.
func (c *StripeClient) CreateCheckoutSessionWithOptions(customerID, priceID, successURL, cancelURL string, opts CheckoutOptions) (string, error) {
	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customerID),
		Mode:     stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
	}
	for k, v := range opts.Metadata {
		params.AddMetadata(k, v)
	}
	// Also attach metadata to the subscription so webhooks can route events.
	params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{}
	for k, v := range opts.Metadata {
		params.SubscriptionData.AddMetadata(k, v)
	}
	if opts.TrialDays > 0 {
		params.SubscriptionData.TrialPeriodDays = new(opts.TrialDays)
	}

	sess, err := checkoutsession.New(params)
	if err != nil {
		return "", fmt.Errorf("create checkout session: %w", err)
	}
	return sess.URL, nil
}

// CreatePortalSession creates a Stripe Customer Portal session for managing subscriptions.
func (c *StripeClient) CreatePortalSession(_ context.Context, customerID, returnURL string) (string, error) {
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(returnURL),
	}

	sess, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("create portal session: %w", err)
	}
	return sess.URL, nil
}

// ReportMeterEvent sends a meter event to Stripe for usage-based billing.
// This is called asynchronously and errors are logged, not returned.
// An idempotency key is derived from the dimensions to prevent duplicate billing.
func (c *StripeClient) ReportMeterEvent(_ context.Context, customerID, eventName string, value int64, dimensions map[string]string) {
	params := &stripe.V2BillingMeterEventCreateParams{
		EventName: stripe.String(eventName),
		Payload: map[string]string{
			"value":              strconv.FormatInt(value, 10),
			"stripe_customer_id": customerID,
		},
	}
	maps.Copy(params.Payload, dimensions)

	// Idempotency key prevents duplicate meter events on retries.
	// Uses workspace_id + operation_type + timestamp bucket (per-second).
	wsID := dimensions["workspace_id"]
	op := dimensions["operation_type"]
	if wsID != "" {
		params.Identifier = stripe.String(fmt.Sprintf("%s-%s-%s-%d", wsID, eventName, op, time.Now().Unix()))
	}

	if _, err := c.sc.V2BillingMeterEvents.Create(context.Background(), params); err != nil {
		slog.Info("stripe meter event error", "error", err)
	}
}

// CreatePaymentCheckout creates a one-time payment checkout session (e.g. credit packs).
func (c *StripeClient) CreatePaymentCheckout(_ context.Context, customerID, priceID, successURL, cancelURL string, metadata map[string]string) (string, error) {
	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customerID),
		Mode:     stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
	}
	for k, v := range metadata {
		params.AddMetadata(k, v)
	}

	sess, err := checkoutsession.New(params)
	if err != nil {
		return "", fmt.Errorf("create payment checkout: %w", err)
	}
	return sess.URL, nil
}

// StripeInvoice represents a simplified invoice for API responses.
type StripeInvoice struct {
	ID         string `json:"id"`
	Number     string `json:"number"`
	Status     string `json:"status"`
	AmountDue  int64  `json:"amount_due"`
	AmountPaid int64  `json:"amount_paid"`
	Currency   string `json:"currency"`
	Created    int64  `json:"created"`
	InvoicePDF string `json:"invoice_pdf"`
	HostedURL  string `json:"hosted_url"`
}

// GetInvoices returns recent invoices for a customer.
func (c *StripeClient) GetInvoices(_ context.Context, customerID string, limit int) ([]StripeInvoice, error) {
	params := &stripe.InvoiceListParams{
		Customer: stripe.String(customerID),
	}
	params.Filters.AddFilter("limit", "", strconv.Itoa(limit))

	iter := invoice.List(params)
	var invoices []StripeInvoice
	for iter.Next() {
		inv := iter.Invoice()
		si := StripeInvoice{
			ID:         inv.ID,
			Number:     inv.Number,
			Status:     string(inv.Status),
			AmountDue:  inv.AmountDue,
			AmountPaid: inv.AmountPaid,
			Currency:   string(inv.Currency),
			Created:    inv.Created,
			InvoicePDF: inv.InvoicePDF,
			HostedURL:  inv.HostedInvoiceURL,
		}
		invoices = append(invoices, si)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	return invoices, nil
}
