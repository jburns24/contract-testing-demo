// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
package ports

import (
	"context"

	pb "github.com/open-telemetry/opentelemetry-demo/src/checkout/genproto/oteldemo"
)

// OrderEventPublisher defines the port for publishing order completion events.
// This is the interface that the core business logic depends on for notifying
// downstream systems about completed orders.
//
// In hexagonal architecture terms:
// - This is a Secondary Port (output port)
// - It defines WHAT the business logic needs to do
// - It abstracts away HOW the events are published (Kafka, RabbitMQ, etc.)
type OrderEventPublisher interface {
	// PublishOrderCompleted publishes an order completion event to downstream systems.
	// This method should be non-blocking and handle errors gracefully.
	//
	// Parameters:
	//   ctx: Context for cancellation and tracing
	//   order: The completed order details to publish
	//
	// Returns:
	//   error: Any error that occurred during publishing
	PublishOrderCompleted(ctx context.Context, order *pb.OrderResult) error
}
