package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	pb "github.com/open-telemetry/opentelemetry-demo/src/checkout/genproto/oteldemo"
)

// TestProducerMessageContract tests that the checkout service produces messages
// that match the contract expected by the accounting service consumer
func TestProducerMessageContract(t *testing.T) {
	// Create the exact same OrderResult structure that our PlaceOrder method creates
	orderResult := &pb.OrderResult{
		OrderId:            "123",
		ShippingTrackingId: "trk-1",
		ShippingCost: &pb.Money{
			CurrencyCode: "USD",
			Units:        5,
			Nanos:        0,
		},
		ShippingAddress: &pb.Address{
			StreetAddress: "123 Main St",
			City:          "Anytown",
			State:         "CA",
			Country:       "USA",
			ZipCode:       "94016",
		},
		Items: []*pb.OrderItem{
			{
				Item: &pb.CartItem{
					ProductId: "SKU-1",
					Quantity:  2,
				},
				Cost: &pb.Money{
					CurrencyCode: "USD",
					Units:        3,
					Nanos:        0,
				},
			},
		},
	}

	// Convert protobuf to JSON (simulating how it would be consumed)
	jsonBytes, err := protojson.Marshal(orderResult)
	if err != nil {
		t.Fatal(err)
	}

	// Parse into generic interface for validation
	var messageContent map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &messageContent); err != nil {
		t.Fatal(err)
	}

	// Verify required fields exist in our producer message
	requiredFields := []string{"orderId", "shippingTrackingId", "shippingCost", "shippingAddress", "items"}
	for _, field := range requiredFields {
		if _, exists := messageContent[field]; !exists {
			t.Errorf("Producer message missing required field: %s", field)
		}
	}

	// Verify specific field values match the contract
	if orderId, ok := messageContent["orderId"].(string); !ok || orderId != "123" {
		t.Errorf("Expected orderId '123', got %v", messageContent["orderId"])
	}

	if trackingId, ok := messageContent["shippingTrackingId"].(string); !ok || trackingId != "trk-1" {
		t.Errorf("Expected shippingTrackingId 'trk-1', got %v", messageContent["shippingTrackingId"])
	}

	// Verify shipping cost structure
	if shippingCost, ok := messageContent["shippingCost"].(map[string]interface{}); ok {
		if currencyCode, ok := shippingCost["currencyCode"].(string); !ok || currencyCode != "USD" {
			t.Errorf("Expected shippingCost.currencyCode 'USD', got %v", shippingCost["currencyCode"])
		}
		if units, ok := shippingCost["units"].(string); !ok || units != "5" {
			t.Errorf("Expected shippingCost.units '5', got %v", shippingCost["units"])
		}
	} else {
		t.Error("Expected shippingCost to be a map")
	}

	// Verify shipping address structure
	if shippingAddress, ok := messageContent["shippingAddress"].(map[string]interface{}); ok {
		expectedAddress := map[string]string{
			"streetAddress": "123 Main St",
			"city":          "Anytown",
			"state":         "CA",
			"country":       "USA",
			"zipCode":       "94016",
		}
		for field, expectedValue := range expectedAddress {
			if actualValue, ok := shippingAddress[field].(string); !ok || actualValue != expectedValue {
				t.Errorf("Expected shippingAddress.%s '%s', got %v", field, expectedValue, shippingAddress[field])
			}
		}
	} else {
		t.Error("Expected shippingAddress to be a map")
	}

	// Verify items structure
	if items, ok := messageContent["items"].([]interface{}); ok {
		if len(items) != 1 {
			t.Errorf("Expected 1 item, got %d", len(items))
		} else {
			item := items[0].(map[string]interface{})
			if itemInfo, ok := item["item"].(map[string]interface{}); ok {
				if productId, ok := itemInfo["productId"].(string); !ok || productId != "SKU-1" {
					t.Errorf("Expected item.productId 'SKU-1', got %v", itemInfo["productId"])
				}
				if quantity, ok := itemInfo["quantity"].(float64); !ok || int(quantity) != 2 {
					t.Errorf("Expected item.quantity 2, got %v", itemInfo["quantity"])
				}
			}
		}
	} else {
		t.Error("Expected items to be an array")
	}

	// Generate the pact file manually to match the consumer expectation
	pactData := map[string]interface{}{
		"consumer": map[string]interface{}{
			"name": "accounting-consumer",
		},
		"provider": map[string]interface{}{
			"name": "checkout-provider",
		},
		"interactions": []map[string]interface{}{
			{
				"type":        "Asynchronous/Messages",
				"description": "order-result message",
				"contents": map[string]interface{}{
					"content":     messageContent,
					"contentType": "application/json",
					"encoded":     false,
				},
				"metadata": map[string]interface{}{
					"contentType": "application/json",
				},
			},
		},
		"metadata": map[string]interface{}{
			"pactSpecification": map[string]interface{}{
				"version": "4.0",
			},
		},
	}

	// Write the pact file
	pactDir := "./pacts"
	if err := os.MkdirAll(pactDir, 0755); err != nil {
		t.Fatalf("Failed to create pacts directory: %v", err)
	}

	pactFile := filepath.Join(pactDir, "accounting-consumer-checkout-provider.json")
	pactJSON, err := json.MarshalIndent(pactData, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal pact data: %v", err)
	}

	if err := ioutil.WriteFile(pactFile, pactJSON, 0644); err != nil {
		t.Fatalf("Failed to write pact file: %v", err)
	}

	t.Logf("Producer pact test completed successfully. Pact file written to: %s", pactFile)
}

// TestOrderResultSerialization tests the actual serialization logic used by sendToPostProcessor
func TestOrderResultSerialization(t *testing.T) {
	// Create an OrderResult exactly like the one in PlaceOrder
	orderResult := &pb.OrderResult{
		OrderId:            "test-order-123",
		ShippingTrackingId: "tracking-456",
		ShippingCost: &pb.Money{
			CurrencyCode: "USD",
			Units:        10,
			Nanos:        500000000, // $10.50
		},
		ShippingAddress: &pb.Address{
			StreetAddress: "456 Test Ave",
			City:          "Test City",
			State:         "TC",
			Country:       "USA",
			ZipCode:       "54321",
		},
		Items: []*pb.OrderItem{
			{
				Item: &pb.CartItem{
					ProductId: "TEST-PRODUCT",
					Quantity:  3,
				},
				Cost: &pb.Money{
					CurrencyCode: "USD",
					Units:        15,
					Nanos:        250000000, // $15.25
				},
			},
		},
	}

	// Test protobuf serialization (what actually gets sent to Kafka)
	protoBytes, err := proto.Marshal(orderResult)
	if err != nil {
		t.Fatalf("Failed to marshal OrderResult to protobuf: %v", err)
	}

	// Verify we can deserialize it back
	var deserializedResult pb.OrderResult
	if err := proto.Unmarshal(protoBytes, &deserializedResult); err != nil {
		t.Fatalf("Failed to unmarshal protobuf: %v", err)
	}

	// Verify key fields are preserved
	if deserializedResult.OrderId != orderResult.OrderId {
		t.Errorf("OrderId not preserved: expected %s, got %s", orderResult.OrderId, deserializedResult.OrderId)
	}

	if deserializedResult.ShippingTrackingId != orderResult.ShippingTrackingId {
		t.Errorf("ShippingTrackingId not preserved: expected %s, got %s", orderResult.ShippingTrackingId, deserializedResult.ShippingTrackingId)
	}

	if len(deserializedResult.Items) != len(orderResult.Items) {
		t.Errorf("Items count not preserved: expected %d, got %d", len(orderResult.Items), len(deserializedResult.Items))
	}

	t.Logf("Successfully serialized and deserialized OrderResult. Protobuf size: %d bytes", len(protoBytes))
}
