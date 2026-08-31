package consumer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/payments/internal/payment"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	paymentevents "polyglot-ticketing-v1/contracts/payments"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

func TestOrderConsumerAcknowledgesValidOrderEvent(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderConsumer(&fakeOrderProjectionRepository{}, "", testLogger())

	consumer.handleDelivery(context.Background(), paymentDeliveryFor(delivery, orderevents.OrderCreatedSubject, validOrderCreatedPayload(t), 1))

	assertDelivery(t, delivery, 1, 0, 0)
}

func TestOrderConsumerNegativeAcknowledgesRetryableEvent(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderConsumer(&fakeOrderProjectionRepository{err: errors.New("database unavailable")}, "", testLogger())

	consumer.handleDelivery(context.Background(), paymentDeliveryFor(delivery, orderevents.OrderCreatedSubject, validOrderCreatedPayload(t), 1))

	assertDelivery(t, delivery, 0, 1, 0)
}

func TestOrderConsumerParksInvalidEvent(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderConsumer(&fakeOrderProjectionRepository{}, "", testLogger())
	deadLetters := &fakePaymentsDeadLetterer{}
	consumer.deadLetters = deadLetters

	consumer.handleDelivery(context.Background(), paymentDeliveryFor(delivery, orderevents.OrderCreatedSubject, nil, 1))

	assertDelivery(t, delivery, 1, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].FailureClass != "invalid" {
		t.Fatalf("expected one invalid event to be parked, got %+v", deadLetters.events)
	}
}

func TestOrderConsumerParksRetryableEventAfterFiveRetries(t *testing.T) {
	consumer := NewOrderConsumer(&fakeOrderProjectionRepository{err: errors.New("database unavailable")}, "", testLogger())
	deadLetters := &fakePaymentsDeadLetterer{}
	consumer.deadLetters = deadLetters
	payload := validOrderCreatedPayload(t)

	for attempt := uint64(1); attempt <= orderEventMaxRetries; attempt++ {
		delivery := &fakeOrderEventDelivery{}
		consumer.handleDelivery(context.Background(), paymentDeliveryFor(delivery, orderevents.OrderCreatedSubject, payload, attempt))
		assertDelivery(t, delivery, 0, 1, 0)
	}

	delivery := &fakeOrderEventDelivery{}
	consumer.handleDelivery(context.Background(), paymentDeliveryFor(delivery, orderevents.OrderCreatedSubject, payload, orderEventMaxRetries+1))
	assertDelivery(t, delivery, 1, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].DeliveryCount != orderEventMaxRetries+1 {
		t.Fatalf("expected retryable event to be parked on attempt %d, got %+v", orderEventMaxRetries+1, deadLetters.events)
	}
}

func TestOrderConsumerParksVersionGapAfterFiveRetries(t *testing.T) {
	consumer := NewOrderConsumer(&fakeOrderProjectionRepository{err: payment.ErrOrderEventVersionGap}, "", testLogger())
	deadLetters := &fakePaymentsDeadLetterer{}
	consumer.deadLetters = deadLetters
	payload := validOrderCanceledPayload(t)

	for attempt := uint64(1); attempt <= orderEventMaxRetries; attempt++ {
		delivery := &fakeOrderEventDelivery{}
		consumer.handleDelivery(context.Background(), paymentDeliveryFor(delivery, orderevents.OrderCanceledSubject, payload, attempt))
		assertDelivery(t, delivery, 0, 1, 0)
	}

	delivery := &fakeOrderEventDelivery{}
	consumer.handleDelivery(context.Background(), paymentDeliveryFor(delivery, orderevents.OrderCanceledSubject, payload, orderEventMaxRetries+1))
	assertDelivery(t, delivery, 1, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].FailureClass != "version_gap" {
		t.Fatalf("expected version-gap event to be parked, got %+v", deadLetters.events)
	}
}

func TestOrderConsumerRetriesWhenParkingFails(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderConsumer(&fakeOrderProjectionRepository{err: errors.New("database unavailable")}, "", testLogger())
	consumer.deadLetters = &fakePaymentsDeadLetterer{err: errors.New("DLQ unavailable")}

	consumer.handleDelivery(context.Background(), paymentDeliveryFor(
		delivery,
		orderevents.OrderCreatedSubject,
		validOrderCreatedPayload(t),
		orderEventMaxRetries+1,
	))

	assertDelivery(t, delivery, 0, 1, 0)
}

func TestOrderConsumerParksSixthFailedJetStreamDelivery(t *testing.T) {
	testCases := []struct {
		name    string
		payload []byte
		subject string
	}{
		{name: "created", payload: validOrderCreatedPayload(t), subject: orderevents.OrderCreatedSubject},
		{name: "canceled", payload: validOrderCanceledPayload(t), subject: orderevents.OrderCanceledSubject},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			nc, js, shutdown := startOrderJetStream(t)
			defer shutdown()
			if _, err := js.AddStream(&nats.StreamConfig{Name: orderEventsStreamName, Subjects: []string{"orders.>"}}); err != nil {
				t.Fatalf("add orders events stream: %v", err)
			}

			consumeCtx, cancel := context.WithCancel(context.Background())
			consumerDone := make(chan struct{})
			consumer := NewOrderConsumer(&fakeOrderProjectionRepository{err: errors.New("database unavailable")}, nc.ConnectedUrl(), testLogger())
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

			if _, err := js.Publish(testCase.subject, testCase.payload); err != nil {
				t.Fatalf("publish order event: %v", err)
			}

			waitForPaymentsDeadLetter(t, js, testCase.subject, testCase.payload)
			waitForOrderAcknowledgement(t, js)
		})
	}
}

func TestPaymentsDeadLettererRetainsAndReplaysOriginalOrderEvent(t *testing.T) {
	_, js, shutdown := startOrderJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: orderEventsStreamName, Subjects: []string{"orders.>"}}); err != nil {
		t.Fatalf("add orders events stream: %v", err)
	}
	if err := ensurePaymentsDeadLetterStream(js); err != nil {
		t.Fatalf("create Payments DLQ stream: %v", err)
	}

	testCases := []struct {
		name    string
		payload []byte
		subject string
	}{
		{name: "created", payload: validOrderCreatedPayload(t), subject: orderevents.OrderCreatedSubject},
		{name: "canceled", payload: validOrderCanceledPayload(t), subject: orderevents.OrderCanceledSubject},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := (jetStreamPaymentsDeadLetterer{js: js}).Park(context.Background(), paymentsDeadLetter{
				Consumer:        orderConsumerDurableName,
				DeliveryCount:   6,
				FailureClass:    "retryable",
				FailureReason:   "database unavailable",
				OriginalHeader:  nats.Header{"traceparent": []string{"00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"}},
				OriginalStream:  orderEventsStreamName,
				OriginalSeq:     uint64(index + 1),
				OriginalSubject: testCase.subject,
				Payload:         testCase.payload,
				Subject:         paymentevents.OrderProjectionDeadLetterSubject,
			}); err != nil {
				t.Fatalf("park order event: %v", err)
			}

			sequence := uint64(index + 1)
			parked, err := js.GetMsg(paymentsDeadLetterStreamName, sequence)
			if err != nil {
				t.Fatalf("get parked event: %v", err)
			}
			if string(parked.Data) != string(testCase.payload) || parked.Header.Get(paymentsDeadLetterHeader+"Original-Subject") != testCase.subject {
				t.Fatalf("unexpected parked message: %+v", parked)
			}
			ack, err := ReplayPaymentsDeadLetter(context.Background(), js, sequence)
			if err != nil {
				t.Fatalf("replay order event: %v", err)
			}
			replayed, err := js.GetMsg(orderEventsStreamName, ack.Sequence)
			if err != nil {
				t.Fatalf("get replayed event: %v", err)
			}
			if replayed.Subject != testCase.subject || string(replayed.Data) != string(testCase.payload) || replayed.Header.Get("traceparent") == "" {
				t.Fatalf("unexpected replayed message: %+v", replayed)
			}
		})
	}
}

func TestEnsurePaymentsDeadLetterStreamAddsSubjectsToExistingStream(t *testing.T) {
	_, js, shutdown := startOrderJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     paymentsDeadLetterStreamName,
		Subjects: []string{paymentevents.OrderProjectionDeadLetterSubject},
		Storage:  nats.FileStorage,
	}); err != nil {
		t.Fatalf("add legacy Payments DLQ stream: %v", err)
	}

	if err := ensurePaymentsDeadLetterStream(js); err != nil {
		t.Fatalf("upgrade Payments DLQ stream: %v", err)
	}
	info, err := js.StreamInfo(paymentsDeadLetterStreamName)
	if err != nil {
		t.Fatalf("read Payments DLQ stream: %v", err)
	}
	if len(info.Config.Subjects) != 1 || info.Config.Subjects[0] != paymentevents.OrderProjectionDeadLetterSubject {
		t.Fatalf("unexpected Payments DLQ subjects: %+v", info.Config.Subjects)
	}
}

func validOrderCreatedPayload(t *testing.T) []byte {
	t.Helper()
	payload, err := proto.Marshal(&ordersv1.OrderCreated{
		EventId:          uuid.NewString(),
		OccurredAt:       timestamppb.New(time.Now().UTC()),
		OrderId:          uuid.NewString(),
		OrderStatus:      ordersv1.OrderStatus_ORDER_STATUS_CREATED,
		UserId:           "user-1",
		ExpiresAt:        timestamppb.New(time.Now().UTC().Add(time.Minute)),
		AggregateVersion: 0,
		Ticket:           &ordersv1.OrderTicket{Id: uuid.NewString(), Price: 10_000},
	})
	if err != nil {
		t.Fatalf("marshal order-created event: %v", err)
	}
	return payload
}

func validOrderCanceledPayload(t *testing.T) []byte {
	t.Helper()
	payload, err := proto.Marshal(&ordersv1.OrderCanceled{
		EventId:          uuid.NewString(),
		OccurredAt:       timestamppb.New(time.Now().UTC()),
		OrderId:          uuid.NewString(),
		AggregateVersion: 1,
		Ticket:           &ordersv1.OrderCanceledTicket{Id: uuid.NewString()},
	})
	if err != nil {
		t.Fatalf("marshal order-canceled event: %v", err)
	}
	return payload
}

func assertDelivery(t *testing.T, delivery *fakeOrderEventDelivery, acknowledged, negativelyAcknowledged, terminated int) {
	t.Helper()
	if delivery.acknowledged != acknowledged || delivery.negativelyAcknowledged != negativelyAcknowledged || delivery.terminated != terminated {
		t.Fatalf("unexpected event acknowledgement: %+v", delivery)
	}
}

func waitForPaymentsDeadLetter(t *testing.T, js nats.JetStreamContext, originalSubject string, payload []byte) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		parked, err := js.GetMsg(paymentsDeadLetterStreamName, 1)
		if err == nil {
			if parked.Subject != paymentevents.OrderProjectionDeadLetterSubject ||
				parked.Header.Get(paymentsDeadLetterHeader+"Original-Subject") != originalSubject ||
				parked.Header.Get(paymentsDeadLetterHeader+"Delivery-Count") != "6" ||
				string(parked.Data) != string(payload) {
				t.Fatalf("unexpected Payments DLQ event: %+v", parked)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("order event was not parked in Payments DLQ")
}

func waitForOrderAcknowledgement(t *testing.T, js nats.JetStreamContext) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		info, err := js.ConsumerInfo(orderEventsStreamName, orderConsumerDurableName)
		if err == nil && info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 &&
			info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("order event was not acknowledged by JetStream")
}

func paymentDeliveryFor(delivery orderEventDelivery, subject string, payload []byte, deliveryCount uint64) paymentsDelivery {
	return paymentsDelivery{
		delivery:      delivery,
		deliveryCount: deliveryCount,
		headers:       nats.Header{},
		payload:       payload,
		stream:        orderEventsStreamName,
		streamSeq:     1,
		subject:       subject,
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeOrderProjectionRepository struct {
	err error
}

func (repository *fakeOrderProjectionRepository) CancelOrderFromEvent(_ context.Context, _ string, _ string, _ int64) error {
	return repository.err
}

func (repository *fakeOrderProjectionRepository) UpsertOrderFromEvent(_ context.Context, _ string, _ payment.Order) error {
	return repository.err
}

type fakeOrderEventDelivery struct {
	acknowledged           int
	negativelyAcknowledged int
	terminated             int
}

type fakePaymentsDeadLetterer struct {
	err    error
	events []paymentsDeadLetter
}

func (deadLetterer *fakePaymentsDeadLetterer) Park(_ context.Context, event paymentsDeadLetter) error {
	if deadLetterer.err != nil {
		return deadLetterer.err
	}
	deadLetterer.events = append(deadLetterer.events, event)
	return nil
}

func (delivery *fakeOrderEventDelivery) Ack(...nats.AckOpt) error {
	delivery.acknowledged++
	return nil
}

func (delivery *fakeOrderEventDelivery) Nak(...nats.AckOpt) error {
	delivery.negativelyAcknowledged++
	return nil
}

func (delivery *fakeOrderEventDelivery) Term(...nats.AckOpt) error {
	delivery.terminated++
	return nil
}
