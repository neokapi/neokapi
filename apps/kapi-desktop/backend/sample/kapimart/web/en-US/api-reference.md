---
title: API Reference
sidebar_position: 4
---

# API Reference

The KapiMart REST API is organized around standard HTTP methods. All requests
and responses use JSON. Authentication is via Bearer token using your API key.

**Base URL:**
- Sandbox: `https://api.sandbox.kapimart.com/v2`
- Production: `https://api.kapimart.com/v2`

## Authentication

Include your API key in the `Authorization` header:

```
Authorization: Bearer km_live_your_api_key_here
```

## Products

### List Products

```
GET /products
```

| Parameter   | Type    | Description                        |
|-------------|---------|------------------------------------|
| `page`      | integer | Page number (default: 1)           |
| `per_page`  | integer | Items per page (default: 20)       |
| `category`  | string  | Filter by category slug            |
| `status`    | string  | Filter: `active`, `draft`, `archived` |

**Response:**

```json
{
  "data": [
    {
      "id": "prod_a1b2c3",
      "name": "Wireless Headphones",
      "price": 7999,
      "currency": "USD",
      "status": "active",
      "category": "electronics",
      "created_at": "2025-01-15T09:30:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total": 156
  }
}
```

### Create Product

```
POST /products
```

**Request body:**

```json
{
  "name": "Portable Speaker",
  "price": 4999,
  "currency": "USD",
  "description": "Waterproof Bluetooth speaker with 12-hour battery.",
  "category": "electronics",
  "inventory": 250
}
```

Returns the created product with a `201 Created` status.

## Orders

### Create Order

```
POST /orders
```

Creates a new order from the customer's shopping cart. The request must include
at least one line item and a valid shipping address.

### Get Order Status

```
GET /orders/:id
```

Returns the current order status, shipping information, and tracking details.

## Webhooks

KapiMart sends webhook events for order lifecycle changes, inventory updates,
and payment confirmations. Configure your webhook endpoint in the Dashboard
under **Settings > Webhooks**.

### Event Types

| Event                      | Description                                |
|----------------------------|--------------------------------------------|
| `order.created`            | A new order was placed                     |
| `order.paid`               | Payment was confirmed                      |
| `order.shipped`            | Shipment was dispatched                    |
| `order.delivered`          | Delivery was confirmed                     |
| `order.refunded`           | A refund was processed                     |
| `inventory.low`            | Stock fell below the reorder threshold     |
| `product.updated`          | A product listing was modified             |

## Rate Limits

The API enforces the following rate limits per API key:

- **Sandbox:** 100 requests per second
- **Production:** 500 requests per second

When the limit is exceeded, the API returns `429 Too Many Requests` with a
`Retry-After` header indicating when the next request can be made.

## Error Codes

All errors return a JSON body with a `code` and `message` field. See
[Troubleshooting](./troubleshooting.md) for common error resolutions.
