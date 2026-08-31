package consumer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	orderevents "polyglot-ticketing-v1/contracts/orders"
)

const (
	ticketDeadLetterStreamName = "ORDERS_DLQ"
	ticketEventMaxRetries      = uint64(5)
	ticketDeadLetterHeader     = "X-Ticket-DLQ-"
)

type ticketDeadLetter struct {
	Consumer        string
	DeliveryCount   uint64
	FailureClass    string
	FailureReason   string
	OriginalHeader  nats.Header
	OriginalStream  string
	OriginalSeq     uint64
	OriginalSubject string
	Payload         []byte
}

type ticketDeadLetterer interface {
	Park(context.Context, ticketDeadLetter) error
}

type jetStreamTicketDeadLetterer struct {
	js nats.JetStreamContext
}

func ensureTicketDeadLetterStream(js nats.JetStreamContext) error {
	if _, err := js.StreamInfo(ticketDeadLetterStreamName); err == nil {
		return nil
	} else if !errors.Is(err, nats.ErrStreamNotFound) {
		return err
	}

	_, err := js.AddStream(&nats.StreamConfig{
		Name:       ticketDeadLetterStreamName,
		Subjects:   []string{orderevents.TicketProjectionDeadLetterSubject},
		Storage:    nats.FileStorage,
		Retention:  nats.LimitsPolicy,
		MaxAge:     7 * 24 * time.Hour,
		Duplicates: 7 * 24 * time.Hour,
	})
	return err
}

func (deadLetterer jetStreamTicketDeadLetterer) Park(ctx context.Context, event ticketDeadLetter) error {
	headers := cloneHeaders(event.OriginalHeader)
	headers.Set("Nats-Msg-Id", fmt.Sprintf("orders-ticket-projection-dlq-v1:%d", event.OriginalSeq))
	headers.Set(ticketDeadLetterHeader+"Original-Subject", event.OriginalSubject)
	headers.Set(ticketDeadLetterHeader+"Original-Stream", event.OriginalStream)
	headers.Set(ticketDeadLetterHeader+"Original-Stream-Sequence", strconv.FormatUint(event.OriginalSeq, 10))
	headers.Set(ticketDeadLetterHeader+"Consumer", event.Consumer)
	headers.Set(ticketDeadLetterHeader+"Delivery-Count", strconv.FormatUint(event.DeliveryCount, 10))
	headers.Set(ticketDeadLetterHeader+"Failure-Class", event.FailureClass)
	headers.Set(ticketDeadLetterHeader+"Failure-Reason", strings.ReplaceAll(strings.ReplaceAll(event.FailureReason, "\r", " "), "\n", " "))
	headers.Set(ticketDeadLetterHeader+"Parked-At", time.Now().UTC().Format(time.RFC3339Nano))

	_, err := deadLetterer.js.PublishMsg(&nats.Msg{
		Subject: orderevents.TicketProjectionDeadLetterSubject,
		Header:  headers,
		Data:    event.Payload,
	}, nats.Context(ctx))
	return err
}

// ReplayTicketDeadLetter republishes the original ticket event represented by
// a retained DLQ message. It intentionally leaves the DLQ record in place for
// audit; ticket-event consumers make a repeat replay idempotent by event ID.
func ReplayTicketDeadLetter(ctx context.Context, js nats.JetStreamContext, sequence uint64) (*nats.PubAck, error) {
	message, err := js.GetMsg(ticketDeadLetterStreamName, sequence)
	if err != nil {
		return nil, fmt.Errorf("get ticket DLQ message %d: %w", sequence, err)
	}
	if message.Subject != orderevents.TicketProjectionDeadLetterSubject {
		return nil, fmt.Errorf("message %d is not a ticket projection DLQ message", sequence)
	}

	subject := message.Header.Get(ticketDeadLetterHeader + "Original-Subject")
	if subject != "tickets.ticket.created.v1" && subject != "tickets.ticket.updated.v1" {
		return nil, fmt.Errorf("message %d has unsupported original subject %q", sequence, subject)
	}

	headers := nats.Header{}
	for key, values := range message.Header {
		if strings.HasPrefix(key, ticketDeadLetterHeader) || strings.EqualFold(key, "Nats-Msg-Id") {
			continue
		}
		headers[key] = append([]string(nil), values...)
	}
	headers.Set("Nats-Msg-Id", fmt.Sprintf("orders-ticket-projection-replay-v1:%d", sequence))

	ack, err := js.PublishMsg(&nats.Msg{Subject: subject, Header: headers, Data: message.Data}, nats.Context(ctx))
	if err != nil {
		return nil, fmt.Errorf("republish ticket DLQ message %d: %w", sequence, err)
	}
	return ack, nil
}

func cloneHeaders(headers nats.Header) nats.Header {
	clone := make(nats.Header, len(headers)+8)
	for key, values := range headers {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}
