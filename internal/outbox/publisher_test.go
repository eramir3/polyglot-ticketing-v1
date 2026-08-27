package outbox

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	ticketevents "polyglot-ticketing-v1/contracts/tickets"
	ticketsv1 "polyglot-ticketing-v1/protogen/go/tickets/v1"
)

func TestPublisherPublishesTicketUpdatedEvent(t *testing.T) {
	const eventID = "2b997858-1973-498a-ad25-e5c0620e73fb"
	occurredAt := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	expectedEvent := &ticketsv1.TicketUpdated{
		EventId:          eventID,
		OccurredAt:       timestamppb.New(occurredAt),
		AggregateVersion: 1,
		Ticket: &ticketsv1.Ticket{
			Id:     "f446d2f3-4515-4b78-8e6a-81797a2517a3",
			Title:  "Updated concert ticket",
			Price:  20_000,
			UserId: "user-1",
		},
	}
	payload, err := proto.Marshal(expectedEvent)
	if err != nil {
		t.Fatalf("marshal ticket-updated event: %v", err)
	}

	url, js, shutdown := startPublisherJetStream(t)
	defer shutdown()

	repository := &fakePublisherRepository{events: []Event{{
		EventID: eventID,
		Subject: ticketevents.TicketUpdatedSubject,
		Payload: payload,
	}}}
	publisher := NewPublisher(
		repository,
		Config{StreamName: "TICKETS_EVENTS", Subjects: []string{"tickets.>"}},
		url,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	defer publisher.close()

	if err := publisher.publishPending(context.Background()); err != nil {
		t.Fatalf("publish pending events: %v", err)
	}

	message, err := js.GetMsg("TICKETS_EVENTS", 1)
	if err != nil {
		t.Fatalf("get published ticket event: %v", err)
	}
	if message.Subject != ticketevents.TicketUpdatedSubject {
		t.Fatalf("expected subject %q, got %q", ticketevents.TicketUpdatedSubject, message.Subject)
	}
	if got := message.Header.Get("Nats-Msg-Id"); got != eventID {
		t.Fatalf("expected Nats-Msg-Id %q, got %q", eventID, got)
	}

	var actualEvent ticketsv1.TicketUpdated
	if err := proto.Unmarshal(message.Data, &actualEvent); err != nil {
		t.Fatalf("unmarshal published ticket-updated event: %v", err)
	}
	if !proto.Equal(expectedEvent, &actualEvent) {
		t.Fatalf("unexpected published ticket-updated event: %+v", &actualEvent)
	}
	if len(repository.publishedEventIDs) != 1 || repository.publishedEventIDs[0] != eventID {
		t.Fatalf("expected event %q to be marked published, got %v", eventID, repository.publishedEventIDs)
	}
	if len(repository.failedEventIDs) != 0 {
		t.Fatalf("expected no failed events, got %v", repository.failedEventIDs)
	}
}

func startPublisherJetStream(t *testing.T) (string, nats.JetStreamContext, func()) {
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

	return server.ClientURL(), js, func() {
		nc.Close()
		server.Shutdown()
	}
}

type fakePublisherRepository struct {
	events            []Event
	publishedEventIDs []string
	failedEventIDs    []string
}

func (repository *fakePublisherRepository) ClaimPending(
	context.Context,
	int,
	time.Duration,
) ([]Event, error) {
	return repository.events, nil
}

func (repository *fakePublisherRepository) MarkPublished(_ context.Context, eventID string) error {
	repository.publishedEventIDs = append(repository.publishedEventIDs, eventID)
	return nil
}

func (repository *fakePublisherRepository) MarkFailed(_ context.Context, eventID string, _ error) error {
	repository.failedEventIDs = append(repository.failedEventIDs, eventID)
	return nil
}
