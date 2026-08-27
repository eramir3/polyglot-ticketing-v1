package consumer

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	paymentevents "polyglot-ticketing-v1/contracts/payments"
	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

func TestPaymentCreatedConsumerIntegration(t *testing.T) {
	ctx := context.Background()
	pool := startOrdersPostgres(t, ctx)
	orderID, _ := seedCreatedOrder(t, ctx, pool)

	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: paymentEventsStreamName, Subjects: []string{"payments.>"}}); err != nil {
		t.Fatalf("add payments events stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewPaymentCreatedConsumer(order.NewPostgresRepository(pool), nc.ConnectedUrl(), testLogger())
	go func() { consumer.Run(consumeCtx); close(consumerDone) }()
	defer func() {
		cancel()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
			t.Error("payment-created consumer did not stop")
		}
	}()

	eventID := uuid.NewString()
	payload, err := proto.Marshal(&paymentsv1.PaymentCreated{
		EventId: eventID, OccurredAt: timestamppb.New(time.Now().UTC()), PaymentId: uuid.NewString(), OrderId: orderID,
	})
	if err != nil {
		t.Fatalf("marshal payment-created event: %v", err)
	}
	if _, err := js.Publish(paymentevents.PaymentCreatedSubject, payload); err != nil {
		t.Fatalf("publish payment-created event: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		var version int64
		statusErr := pool.QueryRow(ctx, `SELECT status::text, aggregate_version FROM orders WHERE id = $1`, orderID).Scan(&status, &version)
		var processedCount int
		processedErr := pool.QueryRow(ctx, `SELECT COUNT(*) FROM processed_events WHERE event_id = $1`, eventID).Scan(&processedCount)
		info, consumerErr := js.ConsumerInfo(paymentEventsStreamName, paymentCreatedDurableName)
		if statusErr == nil && processedErr == nil && consumerErr == nil &&
			status == string(order.StatusAwaitingPayment) && version == 1 && processedCount == 1 &&
			info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 && info.NumAckPending == 0 && info.NumPending == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	var status string
	var version int64
	if err := pool.QueryRow(ctx, `SELECT status::text, aggregate_version FROM orders WHERE id = $1`, orderID).Scan(&status, &version); err != nil || status != string(order.StatusAwaitingPayment) || version != 1 {
		t.Fatalf("payment-created event was not fully processed: status=%q version=%d err=%v", status, version, err)
	}

	if _, err := js.Publish(paymentevents.PaymentCreatedSubject, payload); err != nil {
		t.Fatalf("republish duplicate payment-created event: %v", err)
	}
	latePayload, err := proto.Marshal(&paymentsv1.PaymentCreated{
		EventId: uuid.NewString(), OccurredAt: timestamppb.New(time.Now().UTC()), PaymentId: uuid.NewString(), OrderId: orderID,
	})
	if err != nil {
		t.Fatalf("marshal late payment-created event: %v", err)
	}
	if _, err := js.Publish(paymentevents.PaymentCreatedSubject, latePayload); err != nil {
		t.Fatalf("publish late payment-created event: %v", err)
	}

	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		statusErr := pool.QueryRow(ctx, `SELECT status::text, aggregate_version FROM orders WHERE id = $1`, orderID).Scan(&status, &version)
		info, consumerErr := js.ConsumerInfo(paymentEventsStreamName, paymentCreatedDurableName)
		if statusErr == nil && consumerErr == nil && status == string(order.StatusAwaitingPayment) && version == 1 &&
			info.Delivered.Stream == 3 && info.AckFloor.Stream == 3 && info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("duplicate and late payment-created events were not acknowledged as no-ops")
}
