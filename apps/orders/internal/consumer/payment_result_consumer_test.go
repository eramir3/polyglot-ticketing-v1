package consumer

import (
	"context"
	"errors"
	"log/slog"
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

func TestPaymentResultConsumersAcknowledgeValidEvents(t *testing.T) {
	for _, testCase := range paymentResultConsumerCases() {
		t.Run(testCase.name, func(t *testing.T) {
			delivery := &fakePaymentResultDelivery{}
			consumer := testCase.newConsumer(&fakePaymentResultRepository{}, "", testLogger())

			consumer.handleDelivery(context.Background(), testCase.payload(t), delivery)

			assertPaymentResultDelivery(t, delivery, 1, 0, 0)
		})
	}
}

func TestPaymentResultConsumersHandleRetryableAndInvalidEvents(t *testing.T) {
	for _, testCase := range paymentResultConsumerCases() {
		t.Run(testCase.name+" retry", func(t *testing.T) {
			delivery := &fakePaymentResultDelivery{}
			consumer := testCase.newConsumer(&fakePaymentResultRepository{err: errors.New("database unavailable")}, "", testLogger())
			consumer.handleDelivery(context.Background(), testCase.payload(t), delivery)
			assertPaymentResultDelivery(t, delivery, 0, 1, 0)
		})
		t.Run(testCase.name+" invalid", func(t *testing.T) {
			delivery := &fakePaymentResultDelivery{}
			consumer := testCase.newConsumer(&fakePaymentResultRepository{}, "", testLogger())
			consumer.handleDelivery(context.Background(), nil, delivery)
			assertPaymentResultDelivery(t, delivery, 0, 0, 1)
		})
	}
}

func TestPaymentResultConsumersAcknowledgeJetStreamEvents(t *testing.T) {
	for _, testCase := range paymentResultConsumerCases() {
		t.Run(testCase.name, func(t *testing.T) {
			nc, js, shutdown := startTicketJetStream(t)
			defer shutdown()
			if _, err := js.AddStream(&nats.StreamConfig{Name: paymentEventsStreamName, Subjects: []string{"payments.>"}}); err != nil {
				t.Fatalf("add payments stream: %v", err)
			}
			consumeCtx, cancel := context.WithCancel(context.Background())
			consumerDone := make(chan struct{})
			consumer := testCase.newConsumer(&fakePaymentResultRepository{}, nc.ConnectedUrl(), testLogger())
			go func() { consumer.Run(consumeCtx); close(consumerDone) }()
			defer func() {
				cancel()
				select {
				case <-consumerDone:
				case <-time.After(2 * time.Second):
					t.Error("payment result consumer did not stop")
				}
			}()

			if _, err := js.Publish(testCase.subject, testCase.payload(t)); err != nil {
				t.Fatalf("publish payment result: %v", err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				info, err := js.ConsumerInfo(paymentEventsStreamName, testCase.durable)
				if err == nil && info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 && info.NumAckPending == 0 && info.NumPending == 0 {
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatal("payment result event was not acknowledged by JetStream")
		})
	}
}

type paymentResultConsumerCase struct {
	durable     string
	name        string
	newConsumer func(order.PaymentResultEventRepository, string, *slog.Logger) *PaymentResultConsumer
	payload     func(*testing.T) []byte
	subject     string
}

func paymentResultConsumerCases() []paymentResultConsumerCase {
	return []paymentResultConsumerCase{
		{name: "succeeded", durable: paymentSucceededDurableName, newConsumer: func(repository order.PaymentResultEventRepository, url string, logger *slog.Logger) *PaymentResultConsumer {
			return NewPaymentSucceededConsumer(repository, url, logger)
		}, payload: validPaymentSucceededPayload, subject: paymentevents.PaymentSucceededSubject},
		{name: "failed", durable: paymentFailedDurableName, newConsumer: func(repository order.PaymentResultEventRepository, url string, logger *slog.Logger) *PaymentResultConsumer {
			return NewPaymentFailedConsumer(repository, url, logger)
		}, payload: validPaymentFailedPayload, subject: paymentevents.PaymentFailedSubject},
	}
}

func validPaymentSucceededPayload(t *testing.T) []byte {
	t.Helper()
	payload, err := proto.Marshal(&paymentsv1.PaymentSucceeded{EventId: uuid.NewString(), OccurredAt: timestamppb.New(time.Now().UTC()), PaymentId: uuid.NewString(), OrderId: uuid.NewString()})
	if err != nil {
		t.Fatalf("marshal payment-succeeded: %v", err)
	}
	return payload
}

func validPaymentFailedPayload(t *testing.T) []byte {
	t.Helper()
	payload, err := proto.Marshal(&paymentsv1.PaymentFailed{EventId: uuid.NewString(), OccurredAt: timestamppb.New(time.Now().UTC()), PaymentId: uuid.NewString(), OrderId: uuid.NewString()})
	if err != nil {
		t.Fatalf("marshal payment-failed: %v", err)
	}
	return payload
}

func assertPaymentResultDelivery(t *testing.T, delivery *fakePaymentResultDelivery, acknowledged, negativelyAcknowledged, terminated int) {
	t.Helper()
	if delivery.acknowledged != acknowledged || delivery.negativelyAcknowledged != negativelyAcknowledged || delivery.terminated != terminated {
		t.Fatalf("unexpected payment result acknowledgement: %+v", delivery)
	}
}

type fakePaymentResultRepository struct{ err error }

func (repository *fakePaymentResultRepository) ApplyPaymentSucceeded(context.Context, string, string) error {
	return repository.err
}
func (repository *fakePaymentResultRepository) ApplyPaymentFailed(context.Context, string, string) error {
	return repository.err
}

type fakePaymentResultDelivery struct{ acknowledged, negativelyAcknowledged, terminated int }

func (delivery *fakePaymentResultDelivery) Ack(...nats.AckOpt) error {
	delivery.acknowledged++
	return nil
}
func (delivery *fakePaymentResultDelivery) Nak(...nats.AckOpt) error {
	delivery.negativelyAcknowledged++
	return nil
}
func (delivery *fakePaymentResultDelivery) Term(...nats.AckOpt) error {
	delivery.terminated++
	return nil
}

var _ order.PaymentResultEventRepository = (*fakePaymentResultRepository)(nil)
