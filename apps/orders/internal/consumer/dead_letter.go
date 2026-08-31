package consumer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	expirationevents "polyglot-ticketing-v1/contracts/expiration"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	paymentevents "polyglot-ticketing-v1/contracts/payments"
	ticketevents "polyglot-ticketing-v1/contracts/tickets"
)

const (
	ordersDeadLetterStreamName   = "ORDERS_DLQ"
	orderEventMaxRetries         = uint64(5)
	ordersDeadLetterHeader       = "X-Orders-DLQ-"
	legacyTicketDeadLetterHeader = "X-Ticket-DLQ-"
)

var ordersDeadLetterSubjects = []string{
	orderevents.TicketProjectionDeadLetterSubject,
	orderevents.ExpirationCompleteDeadLetterSubject,
	orderevents.PaymentCreatedDeadLetterSubject,
	orderevents.PaymentSucceededDeadLetterSubject,
	orderevents.PaymentFailedDeadLetterSubject,
}

type ordersDeadLetter struct {
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

type ordersDeadLetterer interface {
	Park(context.Context, ordersDeadLetter) error
}

type jetStreamOrdersDeadLetterer struct {
	js nats.JetStreamContext
}

func ensureOrdersDeadLetterStream(js nats.JetStreamContext) error {
	info, err := js.StreamInfo(ordersDeadLetterStreamName)
	if errors.Is(err, nats.ErrStreamNotFound) {
		_, addErr := js.AddStream(&nats.StreamConfig{
			Name:       ordersDeadLetterStreamName,
			Subjects:   append([]string(nil), ordersDeadLetterSubjects...),
			Storage:    nats.FileStorage,
			Retention:  nats.LimitsPolicy,
			MaxAge:     7 * 24 * time.Hour,
			Duplicates: 7 * 24 * time.Hour,
		})
		if addErr == nil {
			return nil
		}
		info, err = js.StreamInfo(ordersDeadLetterStreamName)
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
	for _, subject := range ordersDeadLetterSubjects {
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

func (deadLetterer jetStreamOrdersDeadLetterer) Park(ctx context.Context, event ordersDeadLetter) error {
	headers := cloneHeaders(event.OriginalHeader)
	headers.Set("Nats-Msg-Id", fmt.Sprintf("orders-dlq-v1:%s:%d", event.OriginalStream, event.OriginalSeq))
	headers.Set(ordersDeadLetterHeader+"Original-Subject", event.OriginalSubject)
	headers.Set(ordersDeadLetterHeader+"Original-Stream", event.OriginalStream)
	headers.Set(ordersDeadLetterHeader+"Original-Stream-Sequence", strconv.FormatUint(event.OriginalSeq, 10))
	headers.Set(ordersDeadLetterHeader+"Consumer", event.Consumer)
	headers.Set(ordersDeadLetterHeader+"Delivery-Count", strconv.FormatUint(event.DeliveryCount, 10))
	headers.Set(ordersDeadLetterHeader+"Failure-Class", event.FailureClass)
	headers.Set(ordersDeadLetterHeader+"Failure-Reason", strings.ReplaceAll(strings.ReplaceAll(event.FailureReason, "\r", " "), "\n", " "))
	headers.Set(ordersDeadLetterHeader+"Parked-At", time.Now().UTC().Format(time.RFC3339Nano))

	_, err := deadLetterer.js.PublishMsg(&nats.Msg{
		Subject: event.Subject,
		Header:  headers,
		Data:    event.Payload,
	}, nats.Context(ctx))
	return err
}

// ReplayOrdersDeadLetter republishes the original Orders-consumed event represented by a retained DLQ message.
// It intentionally leaves the DLQ record in place for audit; Orders consumers make a repeat replay idempotent by event ID.
func ReplayOrdersDeadLetter(ctx context.Context, js nats.JetStreamContext, sequence uint64) (*nats.PubAck, error) {
	message, err := js.GetMsg(ordersDeadLetterStreamName, sequence)
	if err != nil {
		return nil, fmt.Errorf("get Orders DLQ message %d: %w", sequence, err)
	}
	if !isOrdersDeadLetterSubject(message.Subject) {
		return nil, fmt.Errorf("message %d is not an Orders DLQ message", sequence)
	}

	subject := originalSubject(message.Header)
	if !isOrdersConsumedSubject(subject) {
		return nil, fmt.Errorf("message %d has unsupported original subject %q", sequence, subject)
	}

	headers := nats.Header{}
	for key, values := range message.Header {
		if strings.HasPrefix(key, ordersDeadLetterHeader) || strings.HasPrefix(key, legacyTicketDeadLetterHeader) || strings.EqualFold(key, "Nats-Msg-Id") {
			continue
		}
		headers[key] = append([]string(nil), values...)
	}
	headers.Set("Nats-Msg-Id", fmt.Sprintf("orders-dlq-replay-v1:%d", sequence))

	ack, err := js.PublishMsg(&nats.Msg{Subject: subject, Header: headers, Data: message.Data}, nats.Context(ctx))
	if err != nil {
		return nil, fmt.Errorf("republish Orders DLQ message %d: %w", sequence, err)
	}
	return ack, nil
}

func newOrdersDelivery(message *nats.Msg, deadLetterSubject string) (ordersDelivery, error) {
	metadata, err := message.Metadata()
	if err != nil {
		return ordersDelivery{}, err
	}
	return ordersDelivery{
		delivery:          message,
		deadLetterSubject: deadLetterSubject,
		deliveryCount:     metadata.NumDelivered,
		headers:           message.Header,
		payload:           message.Data,
		stream:            metadata.Stream,
		streamSeq:         metadata.Sequence.Stream,
		subject:           message.Subject,
	}, nil
}

type ordersEventDelivery interface {
	Ack(...nats.AckOpt) error
	Nak(...nats.AckOpt) error
}

type ordersDelivery struct {
	delivery          ordersEventDelivery
	deadLetterSubject string
	deliveryCount     uint64
	headers           nats.Header
	payload           []byte
	stream            string
	streamSeq         uint64
	subject           string
}

func (event ordersDelivery) deadLetter(consumer, failureClass string, failure error) ordersDeadLetter {
	return ordersDeadLetter{
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

func originalSubject(headers nats.Header) string {
	if subject := headers.Get(ordersDeadLetterHeader + "Original-Subject"); subject != "" {
		return subject
	}
	return headers.Get(legacyTicketDeadLetterHeader + "Original-Subject")
}

func isOrdersDeadLetterSubject(subject string) bool {
	for _, candidate := range ordersDeadLetterSubjects {
		if subject == candidate {
			return true
		}
	}
	return false
}

func isOrdersConsumedSubject(subject string) bool {
	switch subject {
	case ticketevents.TicketCreatedSubject,
		ticketevents.TicketUpdatedSubject,
		expirationevents.ExpirationCompleteSubject,
		paymentevents.PaymentCreatedSubject,
		paymentevents.PaymentSucceededSubject,
		paymentevents.PaymentFailedSubject:
		return true
	default:
		return false
	}
}

func cloneHeaders(headers nats.Header) nats.Header {
	clone := make(nats.Header, len(headers)+8)
	for key, values := range headers {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}
