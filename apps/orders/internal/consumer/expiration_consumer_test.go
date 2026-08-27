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
	expirationv1 "polyglot-ticketing-v1/protogen/go/expiration/v1"
)

func TestExpirationCompleteConsumerAcknowledgesSuccessfulEvent(t *testing.T) {
	delivery := &fakeExpirationEventDelivery{}
	consumer := NewExpirationCompleteConsumer(&fakeExpirationCompleteRepository{}, "", testLogger())

	consumer.handleDelivery(context.Background(), validExpirationCompletePayload(t), delivery)

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

	consumer.handleDelivery(context.Background(), validExpirationCompletePayload(t), delivery)

	assertExpirationDelivery(t, delivery, 0, 1, 0)
}

func TestExpirationCompleteConsumerTerminatesInvalidEvent(t *testing.T) {
	delivery := &fakeExpirationEventDelivery{}
	consumer := NewExpirationCompleteConsumer(&fakeExpirationCompleteRepository{}, "", testLogger())

	consumer.handleDelivery(context.Background(), nil, delivery)

	assertExpirationDelivery(t, delivery, 0, 0, 1)
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

func (delivery *fakeExpirationEventDelivery) Ack(...nats.AckOpt) error {
	delivery.acknowledged++
	return nil
}

func (delivery *fakeExpirationEventDelivery) Nak(...nats.AckOpt) error {
	delivery.negativelyAcknowledged++
	return nil
}

func (delivery *fakeExpirationEventDelivery) Term(...nats.AckOpt) error {
	delivery.terminated++
	return nil
}
