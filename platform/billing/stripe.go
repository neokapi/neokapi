package billing

import (
	"context"
	"fmt"
	"log"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/invoice"
	"github.com/stripe/stripe-go/v82/v2/billing/meterevent"
)

// StripeClient wraps the Stripe SDK for billing operations.
type StripeClient struct {
	meterClient meterevent.Client
}

// NewStripeClient creates a StripeClient with the given secret key.
func NewStripeClient(secretKey string) *StripeClient {
	stripe.Key = secretKey
	backends := stripe.NewBackends(nil)
	return &StripeClient{
		meterClient: meterevent.Client{B: backends.API, Key: secretKey},
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

// CreateCheckoutSession creates a Stripe Checkout session for subscribing to a plan.
func (c *StripeClient) CreateCheckoutSession(_ context.Context, customerID, priceID, successURL, cancelURL string) (string, error) {
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
func (c *StripeClient) ReportMeterEvent(_ context.Context, customerID, eventName string, value int64, dimensions map[string]string) {
	params := &stripe.V2BillingMeterEventParams{
		EventName: stripe.String(eventName),
		Payload: map[string]string{
			"value":              fmt.Sprintf("%d", value),
			"stripe_customer_id": customerID,
		},
	}
	for k, v := range dimensions {
		params.Payload[k] = v
	}

	if _, err := c.meterClient.New(params); err != nil {
		log.Printf("stripe meter event error: %v", err)
	}
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
	params.Filters.AddFilter("limit", "", fmt.Sprintf("%d", limit))

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
