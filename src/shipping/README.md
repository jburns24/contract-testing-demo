# Shipping Service

The Shipping service queries `quote` for price quote, provides tracking IDs,
and the impression of order fulfillment & shipping processes.

## Local

This repo assumes you have rust 1.82 installed. You may use docker, or
[install rust](https://www.rust-lang.org/tools/install).

## Build

From `../../`, run:

```sh
docker compose build shipping
```

## Test

```sh
cargo test
```

## API Reference

### `POST /get-quote`
Returns a shipping price quote given the items in the customer’s cart.

Request body (`application/json`):
```jsonc
{
  "items": [
    { "quantity": 2 },
    { "quantity": 1 }
  ],
  "address": {               // optional
    "zip_code": "94043"
  }
}
```
Required fields:
* `items` – array of at least one object containing `quantity` (unsigned integer).
* `address` – optional; if supplied must contain `zip_code` (string).

Success response (HTTP 200):
```json
{
  "cost_usd": {
    "currency_code": "USD",
    "units": 5,
    "nanos": 990000000
  }
}
```
`units` are whole US dollars. `nanos` are fractional cents multiplied by 10 000 000 (e.g., `$5.99` → `units = 5`, `nanos = 99 * 10_000_000`).

### `POST /ship-order`
Allocates a tracking number for the order.

Request body (`application/json`):
```json
{}
```

Success response (HTTP 200):
```json
{
  "tracking_id": "SHP123456789"
}
```
`tracking_id` is an opaque string unique per order.

## Contract Coverage
These endpoints are covered by a consumer-driven contract defined in
`src/frontend/pacts/Frontend-ShippingService.json`. The `contract_verification`
integration test runs the Pact verifier against the live service during `cargo test`.

