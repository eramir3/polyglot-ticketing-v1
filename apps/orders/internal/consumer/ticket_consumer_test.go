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

			consumer.handleDelivery(context.Background(), testCase.subject, testCase.payload, delivery)

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

func TestTicketConsumerNegativeAcknowledgesRetryableEvent(t *testing.T) {
	delivery := &fakeTicketEventDelivery{}
	consumer := NewTicketConsumer(&fakeTicketProjectionRepository{err: errors.New("database unavailable")}, "", testLogger())

	consumer.handleDelivery(
		context.Background(),
		ticketevents.TicketCreatedSubject,
		marshalTicketEvent(t, &ticketsv1.TicketCreated{EventId: "retry-event", Ticket: validTicket()}),
		delivery,
	)

	assertTicketDelivery(t, delivery, 0, 1, 0)
}

func TestTicketConsumerDoesNotAcknowledgeSkippedTicketVersion(t *testing.T) {
	delivery := &fakeTicketEventDelivery{}
	consumer := NewTicketConsumer(
		&fakeTicketProjectionRepository{err: order.ErrTicketEventVersionGap},
		"",
		testLogger(),
	)

	consumer.handleDelivery(
		context.Background(),
		ticketevents.TicketUpdatedSubject,
		marshalTicketEvent(t, &ticketsv1.TicketUpdated{
			EventId:          "skipped-version-event",
			AggregateVersion: 3,
			Ticket:           validTicket(),
		}),
		delivery,
	)

	assertTicketDelivery(t, delivery, 0, 1, 0)
}

func TestTicketConsumerTerminatesInvalidEvent(t *testing.T) {
	delivery := &fakeTicketEventDelivery{}
	consumer := NewTicketConsumer(&fakeTicketProjectionRepository{}, "", testLogger())

	consumer.handleDelivery(context.Background(), ticketevents.TicketCreatedSubject, nil, delivery)

	assertTicketDelivery(t, delivery, 0, 0, 1)
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
