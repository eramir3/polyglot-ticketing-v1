package consumer

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	paymentevents "polyglot-ticketing-v1/contracts/payments"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

func TestPaymentResultConsumersIntegration(t *testing.T) {
	ctx := context.Background()
	pool := startOrdersPostgres(t, ctx)
	repository := order.NewPostgresRepository(pool)

	succeededCreatedID, _ := seedCreatedOrder(t, ctx, pool)
	succeededAwaitingID, _ := seedCreatedOrder(t, ctx, pool)
	if err := repository.ApplyPaymentCreated(ctx, uuid.NewString(), succeededAwaitingID); err != nil {
		t.Fatalf("seed awaiting-payment order: %v", err)
	}
	failedCreatedID, _ := seedCreatedOrder(t, ctx, pool)
	failedAwaitingID, _ := seedCreatedOrder(t, ctx, pool)
	if err := repository.ApplyPaymentCreated(ctx, uuid.NewString(), failedAwaitingID); err != nil {
		t.Fatalf("seed awaiting-payment order: %v", err)
	}

	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: paymentEventsStreamName, Subjects: []string{"payments.>"}}); err != nil {
		t.Fatalf("add payments events stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	succeededDone := make(chan struct{})
	failedDone := make(chan struct{})
	go func() {
		NewPaymentSucceededConsumer(repository, nc.ConnectedUrl(), testLogger()).Run(consumeCtx)
		close(succeededDone)
	}()
	go func() {
		NewPaymentFailedConsumer(repository, nc.ConnectedUrl(), testLogger()).Run(consumeCtx)
		close(failedDone)
	}()
	defer func() {
		cancel()
		for _, done := range []<-chan struct{}{succeededDone, failedDone} {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("payment result consumer did not stop")
			}
		}
	}()

	succeededCreatedEventID := uuid.NewString()
	succeededAwaitingEventID := uuid.NewString()
	failedCreatedEventID := uuid.NewString()
	failedAwaitingEventID := uuid.NewString()
	publishPaymentSucceeded(t, js, succeededCreatedEventID, succeededCreatedID)
	publishPaymentSucceeded(t, js, succeededAwaitingEventID, succeededAwaitingID)
	publishPaymentFailed(t, js, failedCreatedEventID, failedCreatedID)
	publishPaymentFailed(t, js, failedAwaitingEventID, failedAwaitingID)

	waitForPaymentResultProcessing(t, ctx, pool, js, []expectedOrderStatus{
		{orderID: succeededCreatedID, status: order.StatusComplete, version: 1},
		{orderID: succeededAwaitingID, status: order.StatusComplete, version: 2},
		{orderID: failedCreatedID, status: order.StatusCanceled, version: 1},
		{orderID: failedAwaitingID, status: order.StatusCanceled, version: 2},
	}, 2, 2, 2)
	assertPaymentFailureCancellationEvents(t, ctx, pool, failedCreatedID, failedAwaitingID)

	// Duplicate result events and a success after cancellation are valid no-ops.
	publishPaymentSucceeded(t, js, succeededCreatedEventID, succeededCreatedID)
	publishPaymentFailed(t, js, failedCreatedEventID, failedCreatedID)
	publishPaymentSucceeded(t, js, uuid.NewString(), failedCreatedID)
	waitForPaymentResultProcessing(t, ctx, pool, js, []expectedOrderStatus{
		{orderID: succeededCreatedID, status: order.StatusComplete, version: 1},
		{orderID: failedCreatedID, status: order.StatusCanceled, version: 1},
	}, 4, 3, 2)
	assertPaymentFailureCancellationEvents(t, ctx, pool, failedCreatedID, failedAwaitingID)
}

type expectedOrderStatus struct {
	orderID string
	status  order.Status
	version int64
}

func publishPaymentSucceeded(t *testing.T, js nats.JetStreamContext, eventID, orderID string) {
	t.Helper()
	payload, err := proto.Marshal(&paymentsv1.PaymentSucceeded{
		EventId: eventID, OccurredAt: timestamppb.New(time.Now().UTC()), PaymentId: uuid.NewString(), OrderId: orderID,
	})
	if err != nil {
		t.Fatalf("marshal payment-succeeded event: %v", err)
	}
	if _, err := js.Publish(paymentevents.PaymentSucceededSubject, payload); err != nil {
		t.Fatalf("publish payment-succeeded event: %v", err)
	}
}

func publishPaymentFailed(t *testing.T, js nats.JetStreamContext, eventID, orderID string) {
	t.Helper()
	payload, err := proto.Marshal(&paymentsv1.PaymentFailed{
		EventId: eventID, OccurredAt: timestamppb.New(time.Now().UTC()), PaymentId: uuid.NewString(), OrderId: orderID,
	})
	if err != nil {
		t.Fatalf("marshal payment-failed event: %v", err)
	}
	if _, err := js.Publish(paymentevents.PaymentFailedSubject, payload); err != nil {
		t.Fatalf("publish payment-failed event: %v", err)
	}
}

func waitForPaymentResultProcessing(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	js nats.JetStreamContext,
	expected []expectedOrderStatus,
	succeededDeliveries uint64,
	failedDeliveries uint64,
	expectedCancellationEvents int,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		matched := true
		for _, expectedOrder := range expected {
			var status string
			var version int64
			if err := pool.QueryRow(ctx, `SELECT status::text, aggregate_version FROM orders WHERE id = $1`, expectedOrder.orderID).Scan(&status, &version); err != nil || status != string(expectedOrder.status) || version != expectedOrder.version {
				matched = false
				break
			}
		}
		var cancellations int
		cancellationErr := pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE subject = $1`, orderevents.OrderCanceledSubject).Scan(&cancellations)
		succeededInfo, succeededErr := js.ConsumerInfo(paymentEventsStreamName, paymentSucceededDurableName)
		failedInfo, failedErr := js.ConsumerInfo(paymentEventsStreamName, paymentFailedDurableName)
		if matched && cancellationErr == nil && succeededErr == nil && failedErr == nil && cancellations == expectedCancellationEvents &&
			succeededInfo.Delivered.Consumer == succeededDeliveries && succeededInfo.AckFloor.Consumer == succeededDeliveries && succeededInfo.NumAckPending == 0 &&
			failedInfo.Delivered.Consumer == failedDeliveries && failedInfo.AckFloor.Consumer == failedDeliveries && failedInfo.NumAckPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("payment result events were not fully processed")
}

func assertPaymentFailureCancellationEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderIDs ...string) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT payload FROM outbox_events WHERE subject = $1`, orderevents.OrderCanceledSubject)
	if err != nil {
		t.Fatalf("read payment failure cancellation events: %v", err)
	}
	defer rows.Close()
	found := make(map[string]bool, len(orderIDs))
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan cancellation event: %v", err)
		}
		var event ordersv1.OrderCanceled
		if err := proto.Unmarshal(payload, &event); err != nil {
			t.Fatalf("unmarshal cancellation event: %v", err)
		}
		found[event.GetOrderId()] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate cancellation events: %v", err)
	}
	for _, orderID := range orderIDs {
		if !found[orderID] {
			t.Fatalf("expected OrderCanceled event for order %q", orderID)
		}
	}
}
