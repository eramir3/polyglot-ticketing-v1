package consumer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	expirationevents "polyglot-ticketing-v1/contracts/expiration"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	paymentevents "polyglot-ticketing-v1/contracts/payments"
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

func TestTicketConsumerIntegrationProjectsTicketCreatedEvent(t *testing.T) {
	ctx := context.Background()
	pool := startOrdersPostgres(t, ctx)
	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: streamName, Subjects: []string{"tickets.>"}}); err != nil {
		t.Fatalf("add tickets stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewTicketConsumer(order.NewPostgresRepository(pool), nc.ConnectedUrl(), testLogger())
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

	ticketID := uuid.NewString()
	eventID := uuid.NewString()
	if _, err := js.Publish(ticketevents.TicketCreatedSubject, marshalTicketEvent(t, &ticketsv1.TicketCreated{
		EventId:          eventID,
		OccurredAt:       timestamppb.New(time.Now().UTC()),
		AggregateVersion: 0,
		Ticket:           &ticketsv1.Ticket{Id: ticketID, Title: "Projected concert ticket", Price: 10_000},
	})); err != nil {
		t.Fatalf("publish ticket-created event: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var title string
		var price int64
		var aggregateVersion int64
		projectionErr := pool.QueryRow(ctx, `
			SELECT title, price, aggregate_version
			FROM tickets
			WHERE id = $1`, ticketID).Scan(&title, &price, &aggregateVersion)
		var processedCount int
		processedErr := pool.QueryRow(ctx, `SELECT COUNT(*) FROM processed_events WHERE event_id = $1`, eventID).Scan(&processedCount)
		info, consumerErr := js.ConsumerInfo(streamName, durableName)
		if projectionErr == nil && processedErr == nil && consumerErr == nil &&
			title == "Projected concert ticket" && price == 10_000 && aggregateVersion == 0 && processedCount == 1 &&
			info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 && info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	var title string
	var price int64
	var aggregateVersion int64
	projectionErr := pool.QueryRow(ctx, `
		SELECT title, price, aggregate_version
		FROM tickets
		WHERE id = $1`, ticketID).Scan(&title, &price, &aggregateVersion)
	info, consumerErr := js.ConsumerInfo(streamName, durableName)
	t.Fatalf("ticket-created event was not fully projected: title=%q price=%d aggregate_version=%d projection_err=%v consumer=%+v consumer_err=%v", title, price, aggregateVersion, projectionErr, info, consumerErr)
}

func TestTicketConsumerIntegrationProjectsTicketUpdatedEvent(t *testing.T) {
	ctx := context.Background()
	pool := startOrdersPostgres(t, ctx)
	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: streamName, Subjects: []string{"tickets.>"}}); err != nil {
		t.Fatalf("add tickets stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewTicketConsumer(order.NewPostgresRepository(pool), nc.ConnectedUrl(), testLogger())
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

	ticketID := uuid.NewString()
	if _, err := js.Publish(ticketevents.TicketCreatedSubject, marshalTicketEvent(t, &ticketsv1.TicketCreated{
		EventId:          uuid.NewString(),
		OccurredAt:       timestamppb.New(time.Now().UTC()),
		AggregateVersion: 0,
		Ticket:           &ticketsv1.Ticket{Id: ticketID, Title: "Original concert ticket", Price: 10_000},
	})); err != nil {
		t.Fatalf("publish ticket-created event: %v", err)
	}
	if _, err := js.Publish(ticketevents.TicketUpdatedSubject, marshalTicketEvent(t, &ticketsv1.TicketUpdated{
		EventId:          uuid.NewString(),
		OccurredAt:       timestamppb.New(time.Now().UTC()),
		AggregateVersion: 1,
		Ticket:           &ticketsv1.Ticket{Id: ticketID, Title: "Updated concert ticket", Price: 12_500},
	})); err != nil {
		t.Fatalf("publish ticket-updated event: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var title string
		var price int64
		var aggregateVersion int64
		projectionErr := pool.QueryRow(ctx, `
			SELECT title, price, aggregate_version
			FROM tickets
			WHERE id = $1`, ticketID).Scan(&title, &price, &aggregateVersion)
		var processedCount int
		processedErr := pool.QueryRow(ctx, `SELECT COUNT(*) FROM processed_events`).Scan(&processedCount)
		info, consumerErr := js.ConsumerInfo(streamName, durableName)
		if projectionErr == nil && processedErr == nil && consumerErr == nil &&
			title == "Updated concert ticket" && price == 12_500 && aggregateVersion == 1 && processedCount == 2 &&
			info.Delivered.Stream == 2 && info.AckFloor.Stream == 2 && info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	var title string
	var price int64
	var aggregateVersion int64
	projectionErr := pool.QueryRow(ctx, `
		SELECT title, price, aggregate_version
		FROM tickets
		WHERE id = $1`, ticketID).Scan(&title, &price, &aggregateVersion)
	info, consumerErr := js.ConsumerInfo(streamName, durableName)
	t.Fatalf("ticket-updated event was not fully projected: title=%q price=%d aggregate_version=%d projection_err=%v consumer=%+v consumer_err=%v", title, price, aggregateVersion, projectionErr, info, consumerErr)
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
		parked, err := js.GetMsg(ordersDeadLetterStreamName, 1)
		if err == nil {
			if parked.Header.Get(ordersDeadLetterHeader+"Delivery-Count") != "6" || string(parked.Data) != string(payload) {
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

func TestTicketConsumerDelaysSkippedTicketVersion(t *testing.T) {
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

	assertTicketDelivery(t, delivery, 0, 0, 0)
	assertTicketDelayedNak(t, delivery, time.Second)
}

func TestTicketConsumerExponentiallyDelaysTicketVersionGapRetries(t *testing.T) {
	for attempt, expectedDelay := range ticketVersionGapRetryDelays {
		t.Run(expectedDelay.String(), func(t *testing.T) {
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
					EventId:          "version-gap-event",
					AggregateVersion: 2,
					Ticket:           validTicket(),
				}),
				uint64(attempt+1),
			))

			assertTicketDelivery(t, delivery, 0, 0, 0)
			assertTicketDelayedNak(t, delivery, expectedDelay)
		})
	}
}

func TestTicketConsumerParksTicketVersionGapAfterDelayedRetries(t *testing.T) {
	delivery := &fakeTicketEventDelivery{}
	consumer := NewTicketConsumer(
		&fakeTicketProjectionRepository{err: order.ErrTicketEventVersionGap},
		"",
		testLogger(),
	)
	deadLetters := &fakeTicketDeadLetterer{}
	consumer.deadLetters = deadLetters

	consumer.handleDelivery(context.Background(), ticketDeliveryFor(
		delivery,
		ticketevents.TicketUpdatedSubject,
		marshalTicketEvent(t, &ticketsv1.TicketUpdated{
			EventId:          "persistent-version-gap-event",
			AggregateVersion: 6,
			Ticket:           validTicket(),
		}),
		orderEventMaxRetries+1,
	))

	assertTicketDelivery(t, delivery, 1, 0, 0)
	if len(delivery.delayedNakDurations) != 0 {
		t.Fatalf("expected no delayed negative acknowledgement after retry limit, got %v", delivery.delayedNakDurations)
	}
	if len(deadLetters.events) != 1 || deadLetters.events[0].FailureClass != "version_gap" {
		t.Fatalf("expected version gap event to be parked, got %+v", deadLetters.events)
	}
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

	for attempt := uint64(1); attempt <= orderEventMaxRetries; attempt++ {
		delivery := &fakeTicketEventDelivery{}
		consumer.handleDelivery(context.Background(), ticketDeliveryFor(delivery, ticketevents.TicketUpdatedSubject, payload, attempt))
		assertTicketDelivery(t, delivery, 0, 1, 0)
	}

	delivery := &fakeTicketEventDelivery{}
	consumer.handleDelivery(context.Background(), ticketDeliveryFor(delivery, ticketevents.TicketUpdatedSubject, payload, orderEventMaxRetries+1))
	assertTicketDelivery(t, delivery, 1, 0, 0)
	if len(deadLetters.events) != 1 || deadLetters.events[0].DeliveryCount != orderEventMaxRetries+1 {
		t.Fatalf("expected retryable event to be parked on attempt %d, got %+v", orderEventMaxRetries+1, deadLetters.events)
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
		orderEventMaxRetries+1,
	))

	assertTicketDelivery(t, delivery, 0, 1, 0)
}

func TestTicketDeadLettererRetainsAndReplaysOriginalTicketEvent(t *testing.T) {
	_, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: streamName, Subjects: []string{"tickets.>"}}); err != nil {
		t.Fatalf("add ticket stream: %v", err)
	}
	if err := ensureOrdersDeadLetterStream(js); err != nil {
		t.Fatalf("create ticket DLQ stream: %v", err)
	}
	payload := marshalTicketEvent(t, &ticketsv1.TicketCreated{EventId: "dlq-event", Ticket: validTicket()})
	if err := (jetStreamOrdersDeadLetterer{js: js}).Park(context.Background(), ordersDeadLetter{
		Consumer:        durableName,
		DeliveryCount:   6,
		FailureClass:    "retryable",
		FailureReason:   "database unavailable",
		OriginalHeader:  nats.Header{"traceparent": []string{"00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"}},
		OriginalStream:  streamName,
		OriginalSeq:     42,
		OriginalSubject: ticketevents.TicketCreatedSubject,
		Payload:         payload,
		Subject:         orderevents.TicketProjectionDeadLetterSubject,
	}); err != nil {
		t.Fatalf("park ticket event: %v", err)
	}

	parked, err := js.GetMsg(ordersDeadLetterStreamName, 1)
	if err != nil {
		t.Fatalf("get parked event: %v", err)
	}
	if string(parked.Data) != string(payload) || parked.Header.Get(ordersDeadLetterHeader+"Original-Subject") != ticketevents.TicketCreatedSubject {
		t.Fatalf("unexpected parked message: %+v", parked)
	}
	if _, err := ReplayOrdersDeadLetter(context.Background(), js, 1); err != nil {
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

func TestOrdersDeadLettererRetainsAndReplaysLifecycleEvents(t *testing.T) {
	_, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{Name: expirationEventsStreamName, Subjects: []string{"expiration.>"}}); err != nil {
		t.Fatalf("add expiration stream: %v", err)
	}
	if _, err := js.AddStream(&nats.StreamConfig{Name: paymentEventsStreamName, Subjects: []string{"payments.>"}}); err != nil {
		t.Fatalf("add payments stream: %v", err)
	}
	if err := ensureOrdersDeadLetterStream(js); err != nil {
		t.Fatalf("create Orders DLQ stream: %v", err)
	}

	testCases := []struct {
		deadLetterSubject string
		name              string
		originalSubject   string
		stream            string
	}{
		{deadLetterSubject: orderevents.ExpirationCompleteDeadLetterSubject, name: "expiration", originalSubject: expirationevents.ExpirationCompleteSubject, stream: expirationEventsStreamName},
		{deadLetterSubject: orderevents.PaymentCreatedDeadLetterSubject, name: "payment-created", originalSubject: paymentevents.PaymentCreatedSubject, stream: paymentEventsStreamName},
		{deadLetterSubject: orderevents.PaymentSucceededDeadLetterSubject, name: "payment-succeeded", originalSubject: paymentevents.PaymentSucceededSubject, stream: paymentEventsStreamName},
		{deadLetterSubject: orderevents.PaymentFailedDeadLetterSubject, name: "payment-failed", originalSubject: paymentevents.PaymentFailedSubject, stream: paymentEventsStreamName},
	}
	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := []byte(testCase.name)
			if err := (jetStreamOrdersDeadLetterer{js: js}).Park(context.Background(), ordersDeadLetter{
				Consumer:        "orders-" + testCase.name + "-v1",
				DeliveryCount:   6,
				FailureClass:    "retryable",
				FailureReason:   "database unavailable",
				OriginalHeader:  nats.Header{"traceparent": []string{"00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"}},
				OriginalStream:  testCase.stream,
				OriginalSeq:     uint64(index + 1),
				OriginalSubject: testCase.originalSubject,
				Payload:         payload,
				Subject:         testCase.deadLetterSubject,
			}); err != nil {
				t.Fatalf("park lifecycle event: %v", err)
			}

			sequence := uint64(index + 1)
			parked, err := js.GetMsg(ordersDeadLetterStreamName, sequence)
			if err != nil {
				t.Fatalf("get parked lifecycle event: %v", err)
			}
			if parked.Subject != testCase.deadLetterSubject || parked.Header.Get(ordersDeadLetterHeader+"Original-Subject") != testCase.originalSubject {
				t.Fatalf("unexpected parked lifecycle event: %+v", parked)
			}
			ack, err := ReplayOrdersDeadLetter(context.Background(), js, sequence)
			if err != nil {
				t.Fatalf("replay lifecycle event: %v", err)
			}
			replayed, err := js.GetMsg(testCase.stream, ack.Sequence)
			if err != nil {
				t.Fatalf("get replayed lifecycle event: %v", err)
			}
			if replayed.Subject != testCase.originalSubject || string(replayed.Data) != string(payload) || replayed.Header.Get("traceparent") == "" {
				t.Fatalf("unexpected replayed lifecycle event: %+v", replayed)
			}
		})
	}
}

func TestEnsureOrdersDeadLetterStreamAddsLifecycleSubjectsToExistingStream(t *testing.T) {
	_, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     ordersDeadLetterStreamName,
		Subjects: []string{orderevents.TicketProjectionDeadLetterSubject},
		Storage:  nats.FileStorage,
	}); err != nil {
		t.Fatalf("add legacy Orders DLQ stream: %v", err)
	}

	if err := ensureOrdersDeadLetterStream(js); err != nil {
		t.Fatalf("upgrade Orders DLQ stream: %v", err)
	}
	info, err := js.StreamInfo(ordersDeadLetterStreamName)
	if err != nil {
		t.Fatalf("read Orders DLQ stream: %v", err)
	}
	configured := make(map[string]bool, len(info.Config.Subjects))
	for _, subject := range info.Config.Subjects {
		configured[subject] = true
	}
	for _, subject := range ordersDeadLetterSubjects {
		if !configured[subject] {
			t.Fatalf("Orders DLQ is missing subject %q: %+v", subject, info.Config.Subjects)
		}
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
	waitForConsumerAcknowledgement(t, js, streamName, durableName)
}

func waitForConsumerAcknowledgement(t *testing.T, js nats.JetStreamContext, stream, durable string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		info, err := js.ConsumerInfo(stream, durable)
		if err == nil && info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 &&
			info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("event was not acknowledged by JetStream: stream=%s durable=%s", stream, durable)
}

func waitForOrdersDeadLetter(t *testing.T, js nats.JetStreamContext, subject string, payload []byte) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		parked, err := js.GetMsg(ordersDeadLetterStreamName, 1)
		if err == nil {
			if parked.Subject != subject || parked.Header.Get(ordersDeadLetterHeader+"Delivery-Count") != "6" || string(parked.Data) != string(payload) {
				t.Fatalf("unexpected Orders DLQ event: %+v", parked)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("event was not parked in Orders DLQ for subject %s", subject)
}

func assertTicketDelivery(t *testing.T, delivery *fakeTicketEventDelivery, acknowledged, negativelyAcknowledged, terminated int) {
	t.Helper()
	if delivery.acknowledged != acknowledged ||
		delivery.negativelyAcknowledged != negativelyAcknowledged ||
		delivery.terminated != terminated {
		t.Fatalf("unexpected ticket event acknowledgement: %+v", delivery)
	}
}

func assertTicketDelayedNak(t *testing.T, delivery *fakeTicketEventDelivery, expectedDelay time.Duration) {
	t.Helper()
	if len(delivery.delayedNakDurations) != 1 || delivery.delayedNakDurations[0] != expectedDelay {
		t.Fatalf("expected one delayed negative acknowledgement of %s, got %v", expectedDelay, delivery.delayedNakDurations)
	}
}

func ticketDeliveryFor(delivery ordersEventDelivery, subject string, payload []byte, deliveryCount uint64) ordersDelivery {
	return ordersDelivery{
		delivery:          delivery,
		deadLetterSubject: orderevents.TicketProjectionDeadLetterSubject,
		deliveryCount:     deliveryCount,
		headers:           nats.Header{},
		payload:           payload,
		stream:            streamName,
		streamSeq:         1,
		subject:           subject,
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
	delayedNakDurations    []time.Duration
	terminated             int
}

type fakeTicketDeadLetterer struct {
	err    error
	events []ordersDeadLetter
}

func (deadLetterer *fakeTicketDeadLetterer) Park(_ context.Context, event ordersDeadLetter) error {
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

func (delivery *fakeTicketEventDelivery) NakWithDelay(delay time.Duration, _ ...nats.AckOpt) error {
	delivery.delayedNakDurations = append(delivery.delayedNakDurations, delay)
	return nil
}

func (delivery *fakeTicketEventDelivery) Term(...nats.AckOpt) error {
	delivery.terminated++
	return nil
}
