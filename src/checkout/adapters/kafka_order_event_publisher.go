// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"

	"github.com/IBM/sarama"
	pb "github.com/open-telemetry/opentelemetry-demo/src/checkout/genproto/oteldemo"
	"github.com/open-telemetry/opentelemetry-demo/src/checkout/kafka"
	"github.com/open-telemetry/opentelemetry-demo/src/checkout/ports"
)

// KafkaOrderEventPublisher implements the OrderEventPublisher port using Apache Kafka.
// This adapter handles all Kafka-specific concerns including:
// - Message serialization and routing
// - Producer error handling and retries
// - Distributed tracing context propagation
// - Performance monitoring and metrics
// - It implements the OrderEventPublisher port
type KafkaOrderEventPublisher struct {
	producer sarama.AsyncProducer
	logger   *slog.Logger
	tracer   trace.Tracer
}

// Compile-time check that KafkaOrderEventPublisher implements OrderEventPublisher
var _ ports.OrderEventPublisher = (*KafkaOrderEventPublisher)(nil)

// NewKafkaOrderEventPublisher creates a new Kafka-based order event publisher.
func NewKafkaOrderEventPublisher(producer sarama.AsyncProducer, logger *slog.Logger) *KafkaOrderEventPublisher {
	return &KafkaOrderEventPublisher{
		producer: producer,
		logger:   logger,
		tracer:   otel.Tracer("checkout-kafka-adapter"),
	}
}

// PublishOrderCompleted publishes an order completion event to Kafka.
// This method implements the OrderEventPublisher interface.
func (k *KafkaOrderEventPublisher) PublishOrderCompleted(ctx context.Context, order *pb.OrderResult) error {
	if k.producer == nil {
		k.logger.Warn("Kafka producer not configured, skipping order event publication")
		return nil
	}

	// Serialize the order to protobuf
	message, err := proto.Marshal(order)
	if err != nil {
		return fmt.Errorf("failed to marshal order result to protobuf: %w", err)
	}

	// Create Kafka message
	msg := &sarama.ProducerMessage{
		Topic: kafka.Topic,
		Value: sarama.ByteEncoder(message),
	}

	// Add tracing context to message
	span := k.createProducerSpan(ctx, msg)
	defer span.End()

	// Send message asynchronously
	startTime := time.Now()
	select {
	case k.producer.Input() <- msg:
		// Message queued successfully, now wait for ack
		return k.waitForAcknowledgment(ctx, span, startTime)
	case <-ctx.Done():
		span.SetStatus(otelcodes.Error, "Context cancelled before message could be queued")
		return fmt.Errorf("failed to queue message: %w", ctx.Err())
	}
}

// waitForAcknowledgment waits for the Kafka producer to acknowledge the message.
func (k *KafkaOrderEventPublisher) waitForAcknowledgment(ctx context.Context, span trace.Span, startTime time.Time) error {
	select {
	case successMsg := <-k.producer.Successes():
		duration := time.Since(startTime)
		span.SetAttributes(
			attribute.Bool("messaging.kafka.producer.success", true),
			attribute.Int("messaging.kafka.producer.duration_ms", int(duration.Milliseconds())),
			semconv.MessagingKafkaMessageOffset(int(successMsg.Offset)),
		)
		k.logger.InfoContext(ctx, "Successfully published order event",
			slog.String("offset", fmt.Sprintf("%d", successMsg.Offset)),
			slog.Duration("duration", duration),
		)
		return nil

	case errMsg := <-k.producer.Errors():
		duration := time.Since(startTime)
		span.SetAttributes(
			attribute.Bool("messaging.kafka.producer.success", false),
			attribute.Int("messaging.kafka.producer.duration_ms", int(duration.Milliseconds())),
		)
		span.SetStatus(otelcodes.Error, errMsg.Err.Error())
		k.logger.ErrorContext(ctx, "Failed to publish order event",
			slog.String("error", errMsg.Err.Error()),
			slog.Duration("duration", duration),
		)
		return fmt.Errorf("kafka producer error: %w", errMsg.Err)

	case <-ctx.Done():
		duration := time.Since(startTime)
		span.SetAttributes(
			attribute.Bool("messaging.kafka.producer.success", false),
			attribute.Int("messaging.kafka.producer.duration_ms", int(duration.Milliseconds())),
		)
		span.SetStatus(otelcodes.Error, "Context cancelled while waiting for acknowledgment")
		k.logger.WarnContext(ctx, "Context cancelled while waiting for Kafka acknowledgment",
			slog.Duration("duration", duration),
		)
		return fmt.Errorf("context cancelled while waiting for kafka acknowledgment: %w", ctx.Err())
	}
}

// createProducerSpan creates a distributed tracing span for the Kafka producer operation.
func (k *KafkaOrderEventPublisher) createProducerSpan(ctx context.Context, msg *sarama.ProducerMessage) trace.Span {
	spanContext, span := k.tracer.Start(
		ctx,
		fmt.Sprintf("%s publish", msg.Topic),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			semconv.PeerService("kafka"),
			semconv.NetworkTransportTCP,
			semconv.MessagingSystemKafka,
			semconv.MessagingDestinationName(msg.Topic),
			semconv.MessagingOperationPublish,
			semconv.MessagingKafkaDestinationPartition(int(msg.Partition)),
		),
	)

	// Inject tracing context into message headers
	carrier := make(map[string]string)
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(spanContext, &MapCarrier{m: carrier})

	// Add headers to Kafka message
	for key, value := range carrier {
		msg.Headers = append(msg.Headers, sarama.RecordHeader{
			Key:   []byte(key),
			Value: []byte(value),
		})
	}

	return span
}

// MapCarrier implements the TextMapCarrier interface for OpenTelemetry propagation.
type MapCarrier struct {
	m map[string]string
}

func (c *MapCarrier) Get(key string) string {
	return c.m[key]
}

func (c *MapCarrier) Set(key, value string) {
	c.m[key] = value
}

func (c *MapCarrier) Keys() []string {
	keys := make([]string, 0, len(c.m))
	for k := range c.m {
		keys = append(keys, k)
	}
	return keys
}

// NoOpOrderEventPublisher is a no-operation implementation of OrderEventPublisher.
// This adapter is used when Kafka is not configured or unavailable.
// It implements the OrderEventPublisher port but doesn't actually publish messages.
type NoOpOrderEventPublisher struct{}

// Compile-time check that NoOpOrderEventPublisher implements OrderEventPublisher
var _ ports.OrderEventPublisher = (*NoOpOrderEventPublisher)(nil)

// PublishOrderCompleted implements the OrderEventPublisher interface but does nothing.
// This allows the system to continue functioning even when the messaging infrastructure is unavailable.
func (n *NoOpOrderEventPublisher) PublishOrderCompleted(ctx context.Context, order *pb.OrderResult) error {
	// Log that we're skipping the publication
	// In a real system, you might want to store these events for later replay
	return nil
}
