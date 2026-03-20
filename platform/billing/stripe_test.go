package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewStripeClient(t *testing.T) {
	client := NewStripeClient("sk_test_fake")
	assert.NotNil(t, client)
	assert.NotNil(t, client.sc)
}

func TestStripeInvoice_JSONFields(t *testing.T) {
	inv := StripeInvoice{
		ID:         "inv_123",
		Number:     "INV-001",
		Status:     "paid",
		AmountDue:  2500,
		AmountPaid: 2500,
		Currency:   "usd",
		Created:    1711000000,
		InvoicePDF: "https://pay.stripe.com/invoice/pdf",
		HostedURL:  "https://pay.stripe.com/invoice/hosted",
	}

	assert.Equal(t, "inv_123", inv.ID)
	assert.Equal(t, "paid", inv.Status)
	assert.Equal(t, int64(2500), inv.AmountPaid)
	assert.Equal(t, "usd", inv.Currency)
	assert.NotEmpty(t, inv.InvoicePDF)
	assert.NotEmpty(t, inv.HostedURL)
}
