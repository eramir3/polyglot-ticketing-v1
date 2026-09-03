package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	paymentevents "polyglot-ticketing-v1/contracts/payments"
	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

func TestPaymentCreatedConsumerAcknowledgesSuccessfulEvent(t *testing.T) {
	delivery := &fakePaymentEventDelivery{}
	consumer := NewPaymentCreatedConsumer(&fakePaymentCreatedRepository{}, "", testLogger())

	consumer.handleDelivery(context.Background(), paymentCreatedDeliveryFor(delivery, validPaymentCreatedPayload(t), 1))

	assertPaymentDelivery(t, delivery, 1, 0, 0)
}

func TestPaymentCreatedConsumerNegativeAcknowledgesRetryableEvent(t *testing.T) {
	delivery := &fakePaymentEventDelivery{}
	consumer := NewPaymentCreatedConsumer(&fakePaymentCreatedRepository{err: errors.New("database unavailable")}, "", testLogger())

	consumer.handleDelivery(context.Background(), paymentCreatedDeliveryFor(delivery, validPaymentCreatedPayload(t), 1))

	assertPaymentDelivery(t, delivery, 0, 1, 0)
}

func TestPaymentCreatedConsumerParksInvalidEvent(t *testing.T) {
	delivery := &fakePaymentEventDelivery{}
	consumer := NewPaymentCreatedConsumer(&fakePaymentCreatedRepository{}, "", testLogger())
	deadLetters := &fakeOrdersDeadLetterer{}
	consumer.deadLetters = deadLetters

	consumer.handleDelivery(context.Background(), paymentCreatedDeliveryFor(delivery, nil, 1))

	assertPaymentDelivery(t, delivery, 1, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].FailureClass != "invalid" {
		t.Fatalf("expected one invalid event to be parked, got %+v", deadLetters.events)
	}
}

func TestPaymentCreatedConsumerParksSixthFailedJetStreamDelivery(t *testing.T) {
	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: paymentEventsStreamName, Subjects: []string{"payments.>"}}); err != nil {
		t.Fatalf("add payments stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewPaymentCreatedConsumer(&fakePaymentCreatedRepository{err: errors.New("database unavailable")}, nc.ConnectedUrl(), testLogger())
	go func() {
		consumer.Run(consumeCtx)
		close(consumerDone)
	}()
	defer func() {
		cancel()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
			t.Error("payment-created consumer did not stop")
		}
	}()

	payload := validPaymentCreatedPayload(t)
	if _, err := js.Publish(paymentevents.PaymentCreatedSubject, payload); err != nil {
		t.Fatalf("publish payment-created event: %v", err)
	}
	waitForOrdersDeadLetter(t, js, orderevents.PaymentCreatedDeadLetterSubject, payload)
	waitForConsumerAcknowledgement(t, js, paymentEventsStreamName, paymentCreatedDurableName)
}

func TestPaymentCreatedConsumerRetriesWhenParkingFails(t *testing.T) {
	delivery := &fakePaymentEventDelivery{}
	consumer := NewPaymentCreatedConsumer(&fakePaymentCreatedRepository{err: errors.New("database unavailable")}, "", testLogger())
	consumer.deadLetters = &fakeOrdersDeadLetterer{err: errors.New("DLQ unavailable")}

	consumer.handleDelivery(context.Background(), paymentCreatedDeliveryFor(delivery, validPaymentCreatedPayload(t), orderEventMaxRetries+1))

	assertPaymentDelivery(t, delivery, 0, 1, 0)
}

func TestPaymentCreatedConsumerAcknowledgesJetStreamEvent(t *testing.T) {
	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: paymentEventsStreamName, Subjects: []string{"payments.>"}}); err != nil {
		t.Fatalf("add payments stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewPaymentCreatedConsumer(&fakePaymentCreatedRepository{}, nc.ConnectedUrl(), testLogger())
	go func() { consumer.Run(consumeCtx); close(consumerDone) }()
	defer func() {
		cancel()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
			t.Error("payment-created consumer did not stop")
		}
	}()

	if _, err := js.Publish(paymentevents.PaymentCreatedSubject, validPaymentCreatedPayload(t)); err != nil {
		t.Fatalf("publish payment-created event: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		info, err := js.ConsumerInfo(paymentEventsStreamName, paymentCreatedDurableName)
		if err == nil && info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 && info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("payment-created event was not acknowledged by JetStream")
}

func validPaymentCreatedPayload(t *testing.T) []byte {
	t.Helper()
	payload, err := proto.Marshal(&paymentsv1.PaymentCreated{
		EventId: uuid.NewString(), OccurredAt: timestamppb.New(time.Now().UTC()), PaymentId: uuid.NewString(), OrderId: uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("marshal payment-created event: %v", err)
	}
	return payload
}

func assertPaymentDelivery(t *testing.T, delivery *fakePaymentEventDelivery, acknowledged, negativelyAcknowledged, terminated int) {
	t.Helper()
	if delivery.acknowledged != acknowledged || delivery.negativelyAcknowledged != negativelyAcknowledged || delivery.terminated != terminated {
		t.Fatalf("unexpected payment-created acknowledgement: %+v", delivery)
	}
}

func paymentCreatedDeliveryFor(delivery ordersEventDelivery, payload []byte, deliveryCount uint64) ordersDelivery {
	return ordersDelivery{
		delivery:          delivery,
		deadLetterSubject: orderevents.PaymentCreatedDeadLetterSubject,
		deliveryCount:     deliveryCount,
		headers:           nats.Header{},
		payload:           payload,
		stream:            paymentEventsStreamName,
		streamSeq:         1,
		subject:           paymentevents.PaymentCreatedSubject,
	}
}

type fakePaymentCreatedRepository struct{ err error }

func (repository *fakePaymentCreatedRepository) ApplyPaymentCreated(context.Context, string, string) error {
	return repository.err
}

type fakePaymentEventDelivery struct{ acknowledged, negativelyAcknowledged, terminated int }

func (delivery *fakePaymentEventDelivery) Ack(...nats.AckOpt) error {
	delivery.acknowledged++
	return nil
}
func (delivery *fakePaymentEventDelivery) Nak(...nats.AckOpt) error {
	delivery.negativelyAcknowledged++
	return nil
}

func (delivery *fakePaymentEventDelivery) NakWithDelay(time.Duration, ...nats.AckOpt) error {
	return nil
}
func (delivery *fakePaymentEventDelivery) Term(...nats.AckOpt) error {
	delivery.terminated++
	return nil
}

var _ order.PaymentEventRepository = (*fakePaymentCreatedRepository)(nil)
