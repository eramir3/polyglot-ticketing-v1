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

	expirationevents "polyglot-ticketing-v1/contracts/expiration"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	expirationv1 "polyglot-ticketing-v1/protogen/go/expiration/v1"
)

func TestExpirationCompleteConsumerAcknowledgesSuccessfulEvent(t *testing.T) {
	delivery := &fakeExpirationEventDelivery{}
	consumer := NewExpirationCompleteConsumer(&fakeExpirationCompleteRepository{}, "", testLogger())

	consumer.handleDelivery(context.Background(), expirationDeliveryFor(delivery, validExpirationCompletePayload(t), 1))

	assertExpirationDelivery(t, delivery, 1, 0, 0)
}

func TestExpirationCompleteConsumerAcknowledgesJetStreamEvent(t *testing.T) {
	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()

	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     expirationEventsStreamName,
		Subjects: []string{"expiration.>"},
	}); err != nil {
		t.Fatalf("add expiration stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewExpirationCompleteConsumer(&fakeExpirationCompleteRepository{}, nc.ConnectedUrl(), testLogger())
	go func() {
		consumer.Run(consumeCtx)
		close(consumerDone)
	}()
	defer func() {
		cancel()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
			t.Error("expiration-complete consumer did not stop")
		}
	}()

	if _, err := js.Publish(expirationevents.ExpirationCompleteSubject, validExpirationCompletePayload(t)); err != nil {
		t.Fatalf("publish expiration-complete event: %v", err)
	}

	waitForExpirationAcknowledgement(t, js)
}

func TestExpirationCompleteConsumerNegativeAcknowledgesRetryableEvent(t *testing.T) {
	delivery := &fakeExpirationEventDelivery{}
	consumer := NewExpirationCompleteConsumer(
		&fakeExpirationCompleteRepository{err: errors.New("database unavailable")},
		"",
		testLogger(),
	)

	consumer.handleDelivery(context.Background(), expirationDeliveryFor(delivery, validExpirationCompletePayload(t), 1))

	assertExpirationDelivery(t, delivery, 0, 1, 0)
}

func TestExpirationCompleteConsumerParksInvalidEvent(t *testing.T) {
	delivery := &fakeExpirationEventDelivery{}
	consumer := NewExpirationCompleteConsumer(&fakeExpirationCompleteRepository{}, "", testLogger())
	deadLetters := &fakeOrdersDeadLetterer{}
	consumer.deadLetters = deadLetters

	consumer.handleDelivery(context.Background(), expirationDeliveryFor(delivery, nil, 1))

	assertExpirationDelivery(t, delivery, 1, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].FailureClass != "invalid" {
		t.Fatalf("expected one invalid event to be parked, got %+v", deadLetters.events)
	}
}

func TestExpirationCompleteConsumerParksSixthFailedJetStreamDelivery(t *testing.T) {
	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: expirationEventsStreamName, Subjects: []string{"expiration.>"}}); err != nil {
		t.Fatalf("add expiration stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewExpirationCompleteConsumer(&fakeExpirationCompleteRepository{err: errors.New("database unavailable")}, nc.ConnectedUrl(), testLogger())
	go func() {
		consumer.Run(consumeCtx)
		close(consumerDone)
	}()
	defer func() {
		cancel()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
			t.Error("expiration-complete consumer did not stop")
		}
	}()

	payload := validExpirationCompletePayload(t)
	if _, err := js.Publish(expirationevents.ExpirationCompleteSubject, payload); err != nil {
		t.Fatalf("publish expiration-complete event: %v", err)
	}
	waitForOrdersDeadLetter(t, js, orderevents.ExpirationCompleteDeadLetterSubject, payload)
	waitForConsumerAcknowledgement(t, js, expirationEventsStreamName, expirationCompleteDurableName)
}

func TestExpirationCompleteConsumerRetriesWhenParkingFails(t *testing.T) {
	delivery := &fakeExpirationEventDelivery{}
	consumer := NewExpirationCompleteConsumer(&fakeExpirationCompleteRepository{err: errors.New("database unavailable")}, "", testLogger())
	consumer.deadLetters = &fakeOrdersDeadLetterer{err: errors.New("DLQ unavailable")}

	consumer.handleDelivery(context.Background(), expirationDeliveryFor(delivery, validExpirationCompletePayload(t), orderEventMaxRetries+1))

	assertExpirationDelivery(t, delivery, 0, 1, 0)
}

func validExpirationCompletePayload(t *testing.T) []byte {
	t.Helper()
	payload, err := proto.Marshal(&expirationv1.ExpirationComplete{
		EventId:    uuid.NewString(),
		OccurredAt: timestamppb.New(time.Now().UTC()),
		OrderId:    uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("marshal expiration-complete event: %v", err)
	}
	return payload
}

func waitForExpirationAcknowledgement(t *testing.T, js nats.JetStreamContext) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		info, err := js.ConsumerInfo(expirationEventsStreamName, expirationCompleteDurableName)
		if err == nil && info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 &&
			info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expiration-complete event was not acknowledged by JetStream")
}

func assertExpirationDelivery(t *testing.T, delivery *fakeExpirationEventDelivery, acknowledged, negativelyAcknowledged, terminated int) {
	t.Helper()
	if delivery.acknowledged != acknowledged ||
		delivery.negativelyAcknowledged != negativelyAcknowledged ||
		delivery.terminated != terminated {
		t.Fatalf("unexpected expiration-complete acknowledgement: %+v", delivery)
	}
}

func expirationDeliveryFor(delivery ordersEventDelivery, payload []byte, deliveryCount uint64) ordersDelivery {
	return ordersDelivery{
		delivery:          delivery,
		deadLetterSubject: orderevents.ExpirationCompleteDeadLetterSubject,
		deliveryCount:     deliveryCount,
		headers:           nats.Header{},
		payload:           payload,
		stream:            expirationEventsStreamName,
		streamSeq:         1,
		subject:           expirationevents.ExpirationCompleteSubject,
	}
}

type fakeExpirationCompleteRepository struct {
	err error
}

func (repository *fakeExpirationCompleteRepository) ApplyExpirationComplete(
	_ context.Context,
	_ string,
	_ string,
) error {
	return repository.err
}

type fakeExpirationEventDelivery struct {
	acknowledged           int
	negativelyAcknowledged int
	terminated             int
}

type fakeOrdersDeadLetterer struct {
	err    error
	events []ordersDeadLetter
}

func (deadLetterer *fakeOrdersDeadLetterer) Park(_ context.Context, event ordersDeadLetter) error {
	if deadLetterer.err != nil {
		return deadLetterer.err
	}
	deadLetterer.events = append(deadLetterer.events, event)
	return nil
}

func (delivery *fakeExpirationEventDelivery) Ack(...nats.AckOpt) error {
	delivery.acknowledged++
	return nil
}

func (delivery *fakeExpirationEventDelivery) Nak(...nats.AckOpt) error {
	delivery.negativelyAcknowledged++
	return nil
}

func (delivery *fakeExpirationEventDelivery) NakWithDelay(time.Duration, ...nats.AckOpt) error {
	return nil
}

func (delivery *fakeExpirationEventDelivery) Term(...nats.AckOpt) error {
	delivery.terminated++
	return nil
}
