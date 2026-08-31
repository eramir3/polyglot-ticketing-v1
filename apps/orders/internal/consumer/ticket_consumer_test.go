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

	"polyglot-ticketing-v1/apps/orders/internal/order"
	ticketevents "polyglot-ticketing-v1/contracts/tickets"
	ticketsv1 "polyglot-ticketing-v1/protogen/go/tickets/v1"
)

func TestTicketConsumerAcknowledgesSuccessfulTicketEvents(t *testing.T) {
	testCases := []struct {
		name    string
		payload []byte
		subject string
	}{
		{
			name:    "created",
			payload: marshalTicketEvent(t, &ticketsv1.TicketCreated{EventId: "created-event", Ticket: validTicket()}),
			subject: ticketevents.TicketCreatedSubject,
		},
		{
			name:    "updated",
			payload: marshalTicketEvent(t, &ticketsv1.TicketUpdated{EventId: "updated-event", AggregateVersion: 1, Ticket: validTicket()}),
			subject: ticketevents.TicketUpdatedSubject,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			delivery := &fakeTicketEventDelivery{}
			consumer := NewTicketConsumer(&fakeTicketProjectionRepository{}, "", testLogger())

			consumer.handleDelivery(context.Background(), ticketDeliveryFor(delivery, testCase.subject, testCase.payload, 1))

			assertTicketDelivery(t, delivery, 1, 0, 0)
		})
	}
}

func TestTicketConsumerAcknowledgesJetStreamEvent(t *testing.T) {
	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()

	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{"tickets.>"},
	}); err != nil {
		t.Fatalf("add tickets stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewTicketConsumer(&fakeTicketProjectionRepository{}, nc.ConnectedUrl(), testLogger())
	go func() {
		consumer.Run(consumeCtx)
		close(consumerDone)
	}()
	defer func() {
		cancel()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
			t.Error("ticket consumer did not stop")
		}
	}()

	if _, err := js.Publish(
		ticketevents.TicketCreatedSubject,
		marshalTicketEvent(t, &ticketsv1.TicketCreated{EventId: "jetstream-event", Ticket: validTicket()}),
	); err != nil {
		t.Fatalf("publish ticket event: %v", err)
	}

	waitForTicketAcknowledgement(t, js)
}

func TestTicketConsumerParksSixthFailedJetStreamDelivery(t *testing.T) {
	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: streamName, Subjects: []string{"tickets.>"}}); err != nil {
		t.Fatalf("add tickets stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewTicketConsumer(&fakeTicketProjectionRepository{err: errors.New("database unavailable")}, nc.ConnectedUrl(), testLogger())
	go func() {
		consumer.Run(consumeCtx)
		close(consumerDone)
	}()
	defer func() {
		cancel()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
			t.Error("ticket consumer did not stop")
		}
	}()

	payload := marshalTicketEvent(t, &ticketsv1.TicketCreated{EventId: "retry-to-dlq", Ticket: validTicket()})
	if _, err := js.Publish(ticketevents.TicketCreatedSubject, payload); err != nil {
		t.Fatalf("publish ticket event: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		parked, err := js.GetMsg(ticketDeadLetterStreamName, 1)
		if err == nil {
			if parked.Header.Get(ticketDeadLetterHeader+"Delivery-Count") != "6" || string(parked.Data) != string(payload) {
				t.Fatalf("unexpected parked event: %+v", parked)
			}
			waitForTicketAcknowledgement(t, js)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("ticket event was not parked after five retries")
}

func TestTicketConsumerNegativeAcknowledgesRetryableEvent(t *testing.T) {
	delivery := &fakeTicketEventDelivery{}
	consumer := NewTicketConsumer(&fakeTicketProjectionRepository{err: errors.New("database unavailable")}, "", testLogger())

	consumer.handleDelivery(context.Background(), ticketDeliveryFor(
		delivery,
		ticketevents.TicketCreatedSubject,
		marshalTicketEvent(t, &ticketsv1.TicketCreated{EventId: "retry-event", Ticket: validTicket()}),
		1,
	))

	assertTicketDelivery(t, delivery, 0, 1, 0)
}

func TestTicketConsumerDoesNotAcknowledgeSkippedTicketVersion(t *testing.T) {
	delivery := &fakeTicketEventDelivery{}
	consumer := NewTicketConsumer(
		&fakeTicketProjectionRepository{err: order.ErrTicketEventVersionGap},
		"",
		testLogger(),
	)

	consumer.handleDelivery(context.Background(), ticketDeliveryFor(
		delivery,
		ticketevents.TicketUpdatedSubject,
		marshalTicketEvent(t, &ticketsv1.TicketUpdated{
			EventId:          "skipped-version-event",
			AggregateVersion: 3,
			Ticket:           validTicket(),
		}),
		1,
	))

	assertTicketDelivery(t, delivery, 0, 1, 0)
}

func TestTicketConsumerParksInvalidEvent(t *testing.T) {
	delivery := &fakeTicketEventDelivery{}
	consumer := NewTicketConsumer(&fakeTicketProjectionRepository{}, "", testLogger())
	deadLetters := &fakeTicketDeadLetterer{}
	consumer.deadLetters = deadLetters

	consumer.handleDelivery(context.Background(), ticketDeliveryFor(delivery, ticketevents.TicketCreatedSubject, nil, 1))

	assertTicketDelivery(t, delivery, 1, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].FailureClass != "invalid" {
		t.Fatalf("expected one invalid event to be parked, got %+v", deadLetters.events)
	}
}

func TestTicketConsumerParksRetryableEventAfterFiveRetries(t *testing.T) {
	consumer := NewTicketConsumer(&fakeTicketProjectionRepository{err: errors.New("database unavailable")}, "", testLogger())
	deadLetters := &fakeTicketDeadLetterer{}
	consumer.deadLetters = deadLetters
	payload := marshalTicketEvent(t, &ticketsv1.TicketUpdated{EventId: "retry-event", AggregateVersion: 1, Ticket: validTicket()})

	for attempt := uint64(1); attempt <= ticketEventMaxRetries; attempt++ {
		delivery := &fakeTicketEventDelivery{}
		consumer.handleDelivery(context.Background(), ticketDeliveryFor(delivery, ticketevents.TicketUpdatedSubject, payload, attempt))
		assertTicketDelivery(t, delivery, 0, 1, 0)
	}

	delivery := &fakeTicketEventDelivery{}
	consumer.handleDelivery(context.Background(), ticketDeliveryFor(delivery, ticketevents.TicketUpdatedSubject, payload, ticketEventMaxRetries+1))
	assertTicketDelivery(t, delivery, 1, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].DeliveryCount != ticketEventMaxRetries+1 {
		t.Fatalf("expected retryable event to be parked on attempt %d, got %+v", ticketEventMaxRetries+1, deadLetters.events)
	}
}

func TestTicketConsumerRetriesWhenParkingFails(t *testing.T) {
	delivery := &fakeTicketEventDelivery{}
	consumer := NewTicketConsumer(&fakeTicketProjectionRepository{err: errors.New("database unavailable")}, "", testLogger())
	consumer.deadLetters = &fakeTicketDeadLetterer{err: errors.New("DLQ unavailable")}

	consumer.handleDelivery(context.Background(), ticketDeliveryFor(
		delivery,
		ticketevents.TicketCreatedSubject,
		marshalTicketEvent(t, &ticketsv1.TicketCreated{EventId: "retry-event", Ticket: validTicket()}),
		ticketEventMaxRetries+1,
	))

	assertTicketDelivery(t, delivery, 0, 1, 0)
}

func TestTicketDeadLettererRetainsAndReplaysOriginalTicketEvent(t *testing.T) {
	_, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: streamName, Subjects: []string{"tickets.>"}}); err != nil {
		t.Fatalf("add ticket stream: %v", err)
	}
	if err := ensureTicketDeadLetterStream(js); err != nil {
		t.Fatalf("create ticket DLQ stream: %v", err)
	}
	payload := marshalTicketEvent(t, &ticketsv1.TicketCreated{EventId: "dlq-event", Ticket: validTicket()})
	if err := (jetStreamTicketDeadLetterer{js: js}).Park(context.Background(), ticketDeadLetter{
		Consumer:        durableName,
		DeliveryCount:   6,
		FailureClass:    "retryable",
		FailureReason:   "database unavailable",
		OriginalHeader:  nats.Header{"traceparent": []string{"00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"}},
		OriginalStream:  streamName,
		OriginalSeq:     42,
		OriginalSubject: ticketevents.TicketCreatedSubject,
		Payload:         payload,
	}); err != nil {
		t.Fatalf("park ticket event: %v", err)
	}

	parked, err := js.GetMsg(ticketDeadLetterStreamName, 1)
	if err != nil {
		t.Fatalf("get parked event: %v", err)
	}
	if string(parked.Data) != string(payload) || parked.Header.Get(ticketDeadLetterHeader+"Original-Subject") != ticketevents.TicketCreatedSubject {
		t.Fatalf("unexpected parked message: %+v", parked)
	}
	if _, err := ReplayTicketDeadLetter(context.Background(), js, 1); err != nil {
		t.Fatalf("replay ticket event: %v", err)
	}
	replayed, err := js.GetMsg(streamName, 1)
	if err != nil {
		t.Fatalf("get replayed event: %v", err)
	}
	if replayed.Subject != ticketevents.TicketCreatedSubject || string(replayed.Data) != string(payload) || replayed.Header.Get("traceparent") == "" {
		t.Fatalf("unexpected replayed message: %+v", replayed)
	}
}

func marshalTicketEvent(t *testing.T, event proto.Message) []byte {
	t.Helper()
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal ticket event: %v", err)
	}
	return payload
}

func validTicket() *ticketsv1.Ticket {
	return &ticketsv1.Ticket{
		Id:    "ticket-1",
		Price: 10_000,
		Title: "Concert ticket",
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func startTicketJetStream(t *testing.T) (*nats.Conn, nats.JetStreamContext, func()) {
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

func waitForTicketAcknowledgement(t *testing.T, js nats.JetStreamContext) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		info, err := js.ConsumerInfo(streamName, durableName)
		if err == nil && info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 &&
			info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("ticket event was not acknowledged by JetStream")
}

func assertTicketDelivery(t *testing.T, delivery *fakeTicketEventDelivery, acknowledged, negativelyAcknowledged, terminated int) {
	t.Helper()
	if delivery.acknowledged != acknowledged ||
		delivery.negativelyAcknowledged != negativelyAcknowledged ||
		delivery.terminated != terminated {
		t.Fatalf("unexpected ticket event acknowledgement: %+v", delivery)
	}
}

func ticketDeliveryFor(delivery ticketEventDelivery, subject string, payload []byte, deliveryCount uint64) ticketDelivery {
	return ticketDelivery{
		delivery:      delivery,
		deliveryCount: deliveryCount,
		headers:       nats.Header{},
		payload:       payload,
		stream:        streamName,
		streamSeq:     1,
		subject:       subject,
	}
}

type fakeTicketProjectionRepository struct {
	err error
}

func (repository *fakeTicketProjectionRepository) UpsertTicketFromEvent(
	_ context.Context,
	_ string,
	_ order.Ticket,
) error {
	return repository.err
}

type fakeTicketEventDelivery struct {
	acknowledged           int
	negativelyAcknowledged int
	terminated             int
}

type fakeTicketDeadLetterer struct {
	err    error
	events []ticketDeadLetter
}

func (deadLetterer *fakeTicketDeadLetterer) Park(_ context.Context, event ticketDeadLetter) error {
	if deadLetterer.err != nil {
		return deadLetterer.err
	}
	deadLetterer.events = append(deadLetterer.events, event)
	return nil
}

func (delivery *fakeTicketEventDelivery) Ack(...nats.AckOpt) error {
	delivery.acknowledged++
	return nil
}

func (delivery *fakeTicketEventDelivery) Nak(...nats.AckOpt) error {
	delivery.negativelyAcknowledged++
	return nil
}

func (delivery *fakeTicketEventDelivery) Term(...nats.AckOpt) error {
	delivery.terminated++
	return nil
}
