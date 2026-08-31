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
	ticketevents "polyglot-ticketing-v1/contracts/tickets"
)

const (
	ticketsDeadLetterStreamName = "TICKETS_DLQ"
	orderEventMaxRetries        = uint64(5)
	ticketsDeadLetterHeader     = "X-Tickets-DLQ-"
)

var ticketsDeadLetterSubjects = []string{
	ticketevents.OrderReservationDeadLetterSubject,
	ticketevents.OrderCancellationDeadLetterSubject,
}

type ticketsDeadLetter struct {
	Consumer        string
	DeliveryCount   uint64
	FailureClass    string
	FailureReason   string
	OriginalHeader  nats.Header
	OriginalStream  string
	OriginalSeq     uint64
	OriginalSubject string
	Payload         []byte
	Subject         string
}

type ticketsDeadLetterer interface {
	Park(context.Context, ticketsDeadLetter) error
}

type jetStreamTicketsDeadLetterer struct {
	js nats.JetStreamContext
}

func ensureTicketsDeadLetterStream(js nats.JetStreamContext) error {
	info, err := js.StreamInfo(ticketsDeadLetterStreamName)
	if errors.Is(err, nats.ErrStreamNotFound) {
		_, addErr := js.AddStream(&nats.StreamConfig{
			Name:       ticketsDeadLetterStreamName,
			Subjects:   append([]string(nil), ticketsDeadLetterSubjects...),
			Storage:    nats.FileStorage,
			Retention:  nats.LimitsPolicy,
			MaxAge:     7 * 24 * time.Hour,
			Duplicates: 7 * 24 * time.Hour,
		})
		if addErr == nil {
			return nil
		}
		info, err = js.StreamInfo(ticketsDeadLetterStreamName)
		if err != nil {
			return addErr
		}
	} else if err != nil {
		return err
	}

	configured := make(map[string]struct{}, len(info.Config.Subjects))
	for _, subject := range info.Config.Subjects {
		configured[subject] = struct{}{}
	}
	updated := info.Config
	for _, subject := range ticketsDeadLetterSubjects {
		if _, exists := configured[subject]; !exists {
			updated.Subjects = append(updated.Subjects, subject)
		}
	}
	if len(updated.Subjects) == len(info.Config.Subjects) {
		return nil
	}
	_, err = js.UpdateStream(&updated)
	return err
}

func (deadLetterer jetStreamTicketsDeadLetterer) Park(ctx context.Context, event ticketsDeadLetter) error {
	headers := cloneTicketsHeaders(event.OriginalHeader)
	headers.Set("Nats-Msg-Id", fmt.Sprintf("tickets-dlq-v1:%s:%d", event.OriginalStream, event.OriginalSeq))
	headers.Set(ticketsDeadLetterHeader+"Original-Subject", event.OriginalSubject)
	headers.Set(ticketsDeadLetterHeader+"Original-Stream", event.OriginalStream)
	headers.Set(ticketsDeadLetterHeader+"Original-Stream-Sequence", strconv.FormatUint(event.OriginalSeq, 10))
	headers.Set(ticketsDeadLetterHeader+"Consumer", event.Consumer)
	headers.Set(ticketsDeadLetterHeader+"Delivery-Count", strconv.FormatUint(event.DeliveryCount, 10))
	headers.Set(ticketsDeadLetterHeader+"Failure-Class", event.FailureClass)
	headers.Set(ticketsDeadLetterHeader+"Failure-Reason", strings.ReplaceAll(strings.ReplaceAll(event.FailureReason, "\r", " "), "\n", " "))
	headers.Set(ticketsDeadLetterHeader+"Parked-At", time.Now().UTC().Format(time.RFC3339Nano))

	_, err := deadLetterer.js.PublishMsg(&nats.Msg{
		Subject: event.Subject,
		Header:  headers,
		Data:    event.Payload,
	}, nats.Context(ctx))
	return err
}

// ReplayTicketsDeadLetter republishes the original Tickets-consumed order event represented by a retained DLQ message.
// It intentionally leaves the DLQ record in place for audit; Tickets consumers make a repeat replay idempotent by event ID.
func ReplayTicketsDeadLetter(ctx context.Context, js nats.JetStreamContext, sequence uint64) (*nats.PubAck, error) {
	message, err := js.GetMsg(ticketsDeadLetterStreamName, sequence)
	if err != nil {
		return nil, fmt.Errorf("get Tickets DLQ message %d: %w", sequence, err)
	}
	if !isTicketsDeadLetterSubject(message.Subject) {
		return nil, fmt.Errorf("message %d is not a Tickets DLQ message", sequence)
	}

	subject := message.Header.Get(ticketsDeadLetterHeader + "Original-Subject")
	if !isTicketsConsumedOrderSubject(subject) {
		return nil, fmt.Errorf("message %d has unsupported original subject %q", sequence, subject)
	}

	headers := nats.Header{}
	for key, values := range message.Header {
		if strings.HasPrefix(key, ticketsDeadLetterHeader) || strings.EqualFold(key, "Nats-Msg-Id") {
			continue
		}
		headers[key] = append([]string(nil), values...)
	}
	headers.Set("Nats-Msg-Id", fmt.Sprintf("tickets-dlq-replay-v1:%d", sequence))

	ack, err := js.PublishMsg(&nats.Msg{Subject: subject, Header: headers, Data: message.Data}, nats.Context(ctx))
	if err != nil {
		return nil, fmt.Errorf("republish Tickets DLQ message %d: %w", sequence, err)
	}
	return ack, nil
}

func newTicketsDelivery(message *nats.Msg, deadLetterSubject string) (ticketsDelivery, error) {
	metadata, err := message.Metadata()
	if err != nil {
		return ticketsDelivery{}, err
	}
	return ticketsDelivery{
		deadLetterSubject: deadLetterSubject,
		delivery:          message,
		deliveryCount:     metadata.NumDelivered,
		headers:           message.Header,
		payload:           message.Data,
		stream:            metadata.Stream,
		streamSeq:         metadata.Sequence.Stream,
		subject:           message.Subject,
	}, nil
}

type ticketsDelivery struct {
	deadLetterSubject string
	delivery          orderEventDelivery
	deliveryCount     uint64
	headers           nats.Header
	payload           []byte
	stream            string
	streamSeq         uint64
	subject           string
}

func (event ticketsDelivery) deadLetter(consumer, failureClass string, failure error) ticketsDeadLetter {
	return ticketsDeadLetter{
		Consumer:        consumer,
		DeliveryCount:   event.deliveryCount,
		FailureClass:    failureClass,
		FailureReason:   failure.Error(),
		OriginalHeader:  event.headers,
		OriginalStream:  event.stream,
		OriginalSeq:     event.streamSeq,
		OriginalSubject: event.subject,
		Payload:         event.payload,
		Subject:         event.deadLetterSubject,
	}
}

func isTicketsDeadLetterSubject(subject string) bool {
	for _, candidate := range ticketsDeadLetterSubjects {
		if subject == candidate {
			return true
		}
	}
	return false
}

func isTicketsConsumedOrderSubject(subject string) bool {
	return subject == orderevents.OrderCreatedSubject || subject == orderevents.OrderCanceledSubject
}

func cloneTicketsHeaders(headers nats.Header) nats.Header {
	clone := make(nats.Header, len(headers)+8)
	for key, values := range headers {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}
