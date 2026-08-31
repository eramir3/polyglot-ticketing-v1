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
	ticketevents "polyglot-ticketing-v1/contracts/tickets"
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

			testCase.consumer.handleDelivery(context.Background(), orderDeliveryFor(delivery, testCase.consumer, testCase.subject, testCase.payload, 1))

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

	waitForOrderAcknowledgement(t, js, orderCreatedDurableName)
}

func TestOrderConsumerParksSixthFailedJetStreamDelivery(t *testing.T) {
	testCases := []struct {
		consumer          *OrderConsumer
		deadLetterSubject string
		name              string
		payload           []byte
		subject           string
	}{
		{
			consumer:          NewOrderCreatedConsumer(&fakeOrderReservationRepository{reserveErr: errors.New("database unavailable")}, "", orderTestLogger()),
			deadLetterSubject: ticketevents.OrderReservationDeadLetterSubject,
			name:              "created",
			payload:           marshalOrderEvent(t, validOrderCreatedEvent()),
			subject:           orderevents.OrderCreatedSubject,
		},
		{
			consumer:          NewOrderCanceledConsumer(&fakeOrderReservationRepository{unreserveErr: errors.New("database unavailable")}, "", orderTestLogger()),
			deadLetterSubject: ticketevents.OrderCancellationDeadLetterSubject,
			name:              "canceled",
			payload:           marshalOrderEvent(t, validOrderCanceledEvent()),
			subject:           orderevents.OrderCanceledSubject,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
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
			testCase.consumer.url = nc.ConnectedUrl()
			go func() {
				testCase.consumer.Run(consumeCtx)
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

			waitForTicketsDeadLetter(t, js, testCase.deadLetterSubject, testCase.payload)
			waitForOrderAcknowledgement(t, js, testCase.consumer.durableName)
		})
	}
}

func TestOrderConsumerNegativeAcknowledgesRetryableEvent(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderCreatedConsumer(
		&fakeOrderReservationRepository{reserveErr: errors.New("database unavailable")},
		"",
		orderTestLogger(),
	)

	consumer.handleDelivery(context.Background(), orderDeliveryFor(
		delivery,
		consumer,
		orderevents.OrderCreatedSubject,
		marshalOrderEvent(t, validOrderCreatedEvent()),
		1,
	))

	assertOrderDelivery(t, delivery, 0, 1, 0, 0)
}

func TestOrderConsumerDelaysRetryWhenReservationIsPending(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderCanceledConsumer(
		&fakeOrderReservationRepository{unreserveErr: ticket.ErrOrderReservationPending},
		"",
		orderTestLogger(),
	)

	consumer.handleDelivery(context.Background(), orderDeliveryFor(
		delivery,
		consumer,
		orderevents.OrderCanceledSubject,
		marshalOrderEvent(t, validOrderCanceledEvent()),
		1,
	))

	assertOrderDelivery(t, delivery, 0, 0, 1, 0)
	if delivery.delay != orderReservationRetryDelay {
		t.Fatalf("expected retry delay %s, got %s", orderReservationRetryDelay, delivery.delay)
	}
}

func TestOrderConsumerParksInvalidEvent(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderCreatedConsumer(&fakeOrderReservationRepository{}, "", orderTestLogger())
	deadLetters := &fakeTicketsDeadLetterer{}
	consumer.deadLetters = deadLetters

	consumer.handleDelivery(context.Background(), orderDeliveryFor(delivery, consumer, orderevents.OrderCreatedSubject, nil, 1))

	assertOrderDelivery(t, delivery, 1, 0, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].FailureClass != "invalid" {
		t.Fatalf("expected one invalid event to be parked, got %+v", deadLetters.events)
	}
}

func TestOrderConsumerParksRetryableEventAfterFiveRetries(t *testing.T) {
	consumer := NewOrderCreatedConsumer(
		&fakeOrderReservationRepository{reserveErr: errors.New("database unavailable")},
		"",
		orderTestLogger(),
	)
	deadLetters := &fakeTicketsDeadLetterer{}
	consumer.deadLetters = deadLetters
	payload := marshalOrderEvent(t, validOrderCreatedEvent())

	for attempt := uint64(1); attempt <= orderEventMaxRetries; attempt++ {
		delivery := &fakeOrderEventDelivery{}
		consumer.handleDelivery(context.Background(), orderDeliveryFor(delivery, consumer, orderevents.OrderCreatedSubject, payload, attempt))
		assertOrderDelivery(t, delivery, 0, 1, 0, 0)
	}

	delivery := &fakeOrderEventDelivery{}
	consumer.handleDelivery(context.Background(), orderDeliveryFor(delivery, consumer, orderevents.OrderCreatedSubject, payload, orderEventMaxRetries+1))
	assertOrderDelivery(t, delivery, 1, 0, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].DeliveryCount != orderEventMaxRetries+1 {
		t.Fatalf("expected retryable event to be parked on attempt %d, got %+v", orderEventMaxRetries+1, deadLetters.events)
	}
}

func TestOrderConsumerParksPendingReservationAfterFiveDelayedRetries(t *testing.T) {
	consumer := NewOrderCanceledConsumer(
		&fakeOrderReservationRepository{unreserveErr: ticket.ErrOrderReservationPending},
		"",
		orderTestLogger(),
	)
	deadLetters := &fakeTicketsDeadLetterer{}
	consumer.deadLetters = deadLetters
	payload := marshalOrderEvent(t, validOrderCanceledEvent())

	for attempt := uint64(1); attempt <= orderEventMaxRetries; attempt++ {
		delivery := &fakeOrderEventDelivery{}
		consumer.handleDelivery(context.Background(), orderDeliveryFor(delivery, consumer, orderevents.OrderCanceledSubject, payload, attempt))
		assertOrderDelivery(t, delivery, 0, 0, 1, 0)
	}

	delivery := &fakeOrderEventDelivery{}
	consumer.handleDelivery(context.Background(), orderDeliveryFor(delivery, consumer, orderevents.OrderCanceledSubject, payload, orderEventMaxRetries+1))
	assertOrderDelivery(t, delivery, 1, 0, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].FailureClass != "reservation_pending" {
		t.Fatalf("expected pending reservation event to be parked, got %+v", deadLetters.events)
	}
}

func TestOrderConsumerRetriesWhenParkingFails(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderCreatedConsumer(
		&fakeOrderReservationRepository{reserveErr: errors.New("database unavailable")},
		"",
		orderTestLogger(),
	)
	consumer.deadLetters = &fakeTicketsDeadLetterer{err: errors.New("DLQ unavailable")}

	consumer.handleDelivery(context.Background(), orderDeliveryFor(
		delivery,
		consumer,
		orderevents.OrderCreatedSubject,
		marshalOrderEvent(t, validOrderCreatedEvent()),
		orderEventMaxRetries+1,
	))

	assertOrderDelivery(t, delivery, 0, 1, 0, 0)
}

func TestTicketsDeadLettererRetainsAndReplaysOriginalOrderEvent(t *testing.T) {
	_, js, shutdown := startOrderJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: orderStreamName, Subjects: []string{"orders.>"}}); err != nil {
		t.Fatalf("add orders stream: %v", err)
	}
	if err := ensureTicketsDeadLetterStream(js); err != nil {
		t.Fatalf("create Tickets DLQ stream: %v", err)
	}

	testCases := []struct {
		consumer          *OrderConsumer
		deadLetterSubject string
		name              string
		payload           []byte
		subject           string
	}{
		{
			consumer:          NewOrderCreatedConsumer(&fakeOrderReservationRepository{}, "", orderTestLogger()),
			deadLetterSubject: ticketevents.OrderReservationDeadLetterSubject,
			name:              "created",
			payload:           marshalOrderEvent(t, validOrderCreatedEvent()),
			subject:           orderevents.OrderCreatedSubject,
		},
		{
			consumer:          NewOrderCanceledConsumer(&fakeOrderReservationRepository{}, "", orderTestLogger()),
			deadLetterSubject: ticketevents.OrderCancellationDeadLetterSubject,
			name:              "canceled",
			payload:           marshalOrderEvent(t, validOrderCanceledEvent()),
			subject:           orderevents.OrderCanceledSubject,
		},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := (jetStreamTicketsDeadLetterer{js: js}).Park(context.Background(), ticketsDeadLetter{
				Consumer:        testCase.consumer.durableName,
				DeliveryCount:   6,
				FailureClass:    "retryable",
				FailureReason:   "database unavailable",
				OriginalHeader:  nats.Header{"traceparent": []string{"00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"}},
				OriginalStream:  orderStreamName,
				OriginalSeq:     uint64(index + 1),
				OriginalSubject: testCase.subject,
				Payload:         testCase.payload,
				Subject:         testCase.deadLetterSubject,
			}); err != nil {
				t.Fatalf("park order event: %v", err)
			}

			sequence := uint64(index + 1)
			parked, err := js.GetMsg(ticketsDeadLetterStreamName, sequence)
			if err != nil {
				t.Fatalf("get parked event: %v", err)
			}
			if string(parked.Data) != string(testCase.payload) || parked.Header.Get(ticketsDeadLetterHeader+"Original-Subject") != testCase.subject {
				t.Fatalf("unexpected parked message: %+v", parked)
			}
			ack, err := ReplayTicketsDeadLetter(context.Background(), js, sequence)
			if err != nil {
				t.Fatalf("replay order event: %v", err)
			}
			replayed, err := js.GetMsg(orderStreamName, ack.Sequence)
			if err != nil {
				t.Fatalf("get replayed event: %v", err)
			}
			if replayed.Subject != testCase.subject || string(replayed.Data) != string(testCase.payload) || replayed.Header.Get("traceparent") == "" {
				t.Fatalf("unexpected replayed message: %+v", replayed)
			}
		})
	}
}

func TestEnsureTicketsDeadLetterStreamAddsSubjectsToExistingStream(t *testing.T) {
	_, js, shutdown := startOrderJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     ticketsDeadLetterStreamName,
		Subjects: []string{ticketevents.OrderReservationDeadLetterSubject},
		Storage:  nats.FileStorage,
	}); err != nil {
		t.Fatalf("add legacy Tickets DLQ stream: %v", err)
	}

	if err := ensureTicketsDeadLetterStream(js); err != nil {
		t.Fatalf("upgrade Tickets DLQ stream: %v", err)
	}
	info, err := js.StreamInfo(ticketsDeadLetterStreamName)
	if err != nil {
		t.Fatalf("read Tickets DLQ stream: %v", err)
	}
	configured := make(map[string]bool, len(info.Config.Subjects))
	for _, subject := range info.Config.Subjects {
		configured[subject] = true
	}
	for _, subject := range ticketsDeadLetterSubjects {
		if !configured[subject] {
			t.Fatalf("Tickets DLQ is missing subject %q: %+v", subject, info.Config.Subjects)
		}
	}
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

func waitForOrderAcknowledgement(t *testing.T, js nats.JetStreamContext, durable string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		info, err := js.ConsumerInfo(orderStreamName, durable)
		if err == nil && info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 &&
			info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("order event was not acknowledged by JetStream: durable=%s", durable)
}

func waitForTicketsDeadLetter(t *testing.T, js nats.JetStreamContext, subject string, payload []byte) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		parked, err := js.GetMsg(ticketsDeadLetterStreamName, 1)
		if err == nil {
			if parked.Subject != subject || parked.Header.Get(ticketsDeadLetterHeader+"Delivery-Count") != "6" || string(parked.Data) != string(payload) {
				t.Fatalf("unexpected Tickets DLQ event: %+v", parked)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("event was not parked in Tickets DLQ for subject %s", subject)
}

func orderDeliveryFor(delivery orderEventDelivery, consumer *OrderConsumer, subject string, payload []byte, deliveryCount uint64) ticketsDelivery {
	return ticketsDelivery{
		deadLetterSubject: consumer.deadLetterSubject(),
		delivery:          delivery,
		deliveryCount:     deliveryCount,
		headers:           nats.Header{},
		payload:           payload,
		stream:            orderStreamName,
		streamSeq:         1,
		subject:           subject,
	}
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

type fakeTicketsDeadLetterer struct {
	err    error
	events []ticketsDeadLetter
}

func (deadLetterer *fakeTicketsDeadLetterer) Park(_ context.Context, event ticketsDeadLetter) error {
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

func (delivery *fakeOrderEventDelivery) NakWithDelay(delay time.Duration, _ ...nats.AckOpt) error {
	delivery.delayedNegativeAcknowledged++
	delivery.delay = delay
	return nil
}

func (delivery *fakeOrderEventDelivery) Term(...nats.AckOpt) error {
	delivery.terminated++
	return nil
}
