package consumer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/tickets/internal/ticket"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

func TestOrderConsumerAcknowledgesSuccessfulOrderEvents(t *testing.T) {
	testCases := []struct {
		consumer *OrderConsumer
		name     string
		payload  []byte
		subject  string
	}{
		{
			consumer: NewOrderCreatedConsumer(&fakeOrderReservationRepository{}, "", orderTestLogger()),
			name:     "created",
			payload:  marshalOrderEvent(t, validOrderCreatedEvent()),
			subject:  orderevents.OrderCreatedSubject,
		},
		{
			consumer: NewOrderCanceledConsumer(&fakeOrderReservationRepository{}, "", orderTestLogger()),
			name:     "canceled",
			payload:  marshalOrderEvent(t, validOrderCanceledEvent()),
			subject:  orderevents.OrderCanceledSubject,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			delivery := &fakeOrderEventDelivery{}

			testCase.consumer.handleDelivery(context.Background(), testCase.subject, testCase.payload, delivery)

			assertOrderDelivery(t, delivery, 1, 0, 0, 0)
		})
	}
}

func TestOrderConsumerAcknowledgesJetStreamEvent(t *testing.T) {
	nc, js, shutdown := startOrderJetStream(t)
	defer shutdown()

	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     orderStreamName,
		Subjects: []string{"orders.>"},
	}); err != nil {
		t.Fatalf("add orders stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewOrderCreatedConsumer(&fakeOrderReservationRepository{}, nc.ConnectedUrl(), orderTestLogger())
	go func() {
		consumer.Run(consumeCtx)
		close(consumerDone)
	}()
	defer func() {
		cancel()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
			t.Error("order consumer did not stop")
		}
	}()

	if _, err := js.Publish(
		orderevents.OrderCreatedSubject,
		marshalOrderEvent(t, validOrderCreatedEvent()),
	); err != nil {
		t.Fatalf("publish order event: %v", err)
	}

	waitForOrderAcknowledgement(t, js)
}

func TestOrderConsumerNegativeAcknowledgesRetryableEvent(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderCreatedConsumer(
		&fakeOrderReservationRepository{reserveErr: errors.New("database unavailable")},
		"",
		orderTestLogger(),
	)

	consumer.handleDelivery(
		context.Background(),
		orderevents.OrderCreatedSubject,
		marshalOrderEvent(t, validOrderCreatedEvent()),
		delivery,
	)

	assertOrderDelivery(t, delivery, 0, 1, 0, 0)
}

func TestOrderConsumerDelaysRetryWhenReservationIsPending(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderCanceledConsumer(
		&fakeOrderReservationRepository{unreserveErr: ticket.ErrOrderReservationPending},
		"",
		orderTestLogger(),
	)

	consumer.handleDelivery(
		context.Background(),
		orderevents.OrderCanceledSubject,
		marshalOrderEvent(t, validOrderCanceledEvent()),
		delivery,
	)

	assertOrderDelivery(t, delivery, 0, 0, 1, 0)
	if delivery.delay != orderReservationRetryDelay {
		t.Fatalf("expected retry delay %s, got %s", orderReservationRetryDelay, delivery.delay)
	}
}

func TestOrderConsumerTerminatesInvalidEvent(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderCreatedConsumer(&fakeOrderReservationRepository{}, "", orderTestLogger())

	consumer.handleDelivery(context.Background(), orderevents.OrderCreatedSubject, nil, delivery)

	assertOrderDelivery(t, delivery, 0, 0, 0, 1)
}

func validOrderCreatedEvent() *ordersv1.OrderCreated {
	occurredAt := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	return &ordersv1.OrderCreated{
		EventId:     "created-event",
		OccurredAt:  timestamppb.New(occurredAt),
		OrderId:     "order-1",
		OrderStatus: ordersv1.OrderStatus_ORDER_STATUS_CREATED,
		UserId:      "user-1",
		ExpiresAt:   timestamppb.New(occurredAt.Add(15 * time.Minute)),
		Ticket: &ordersv1.OrderTicket{
			Id:    "ticket-1",
			Price: 10_000,
		},
	}
}

func validOrderCanceledEvent() *ordersv1.OrderCanceled {
	return &ordersv1.OrderCanceled{
		EventId:    "canceled-event",
		OccurredAt: timestamppb.New(time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)),
		OrderId:    "order-1",
		Ticket:     &ordersv1.OrderCanceledTicket{Id: "ticket-1"},
	}
}

func marshalOrderEvent(t *testing.T, event proto.Message) []byte {
	t.Helper()
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal order event: %v", err)
	}
	return payload
}

func orderTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func startOrderJetStream(t *testing.T) (*nats.Conn, nats.JetStreamContext, func()) {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{
		JetStream: true,
		NoLog:     true,
		NoSigs:    true,
		Port:      -1,
		StoreDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create JetStream server: %v", err)
	}
	go server.Start()
	if !server.ReadyForConnections(2 * time.Second) {
		server.Shutdown()
		t.Fatal("JetStream server did not become ready")
	}

	nc, err := nats.Connect(server.ClientURL(), nats.Timeout(time.Second))
	if err != nil {
		server.Shutdown()
		t.Fatalf("connect to JetStream server: %v", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		server.Shutdown()
		t.Fatalf("create JetStream context: %v", err)
	}

	return nc, js, func() {
		nc.Close()
		server.Shutdown()
	}
}

func waitForOrderAcknowledgement(t *testing.T, js nats.JetStreamContext) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		info, err := js.ConsumerInfo(orderStreamName, orderCreatedDurableName)
		if err == nil && info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 &&
			info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("order event was not acknowledged by JetStream")
}

func assertOrderDelivery(t *testing.T, delivery *fakeOrderEventDelivery, acknowledged, negativelyAcknowledged, delayedNegativeAcknowledged, terminated int) {
	t.Helper()
	if delivery.acknowledged != acknowledged ||
		delivery.negativelyAcknowledged != negativelyAcknowledged ||
		delivery.delayedNegativeAcknowledged != delayedNegativeAcknowledged ||
		delivery.terminated != terminated {
		t.Fatalf("unexpected order event acknowledgement: %+v", delivery)
	}
}

type fakeOrderReservationRepository struct {
	reserveErr   error
	unreserveErr error
}

func (repository *fakeOrderReservationRepository) ReserveTicketFromOrder(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) error {
	return repository.reserveErr
}

func (repository *fakeOrderReservationRepository) UnreserveTicketFromOrder(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) error {
	return repository.unreserveErr
}

type fakeOrderEventDelivery struct {
	acknowledged                int
	delay                       time.Duration
	delayedNegativeAcknowledged int
	negativelyAcknowledged      int
	terminated                  int
}

func (delivery *fakeOrderEventDelivery) Ack(...nats.AckOpt) error {
	delivery.acknowledged++
	return nil
}

func (delivery *fakeOrderEventDelivery) Nak(...nats.AckOpt) error {
	delivery.negativelyAcknowledged++
	return nil
}

func (delivery *fakeOrderEventDelivery) NakWithDelay(delay time.Duration, _ ...nats.AckOpt) error {
	delivery.delayedNegativeAcknowledged++
	delivery.delay = delay
	return nil
}

func (delivery *fakeOrderEventDelivery) Term(...nats.AckOpt) error {
	delivery.terminated++
	return nil
}
