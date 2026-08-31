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
	paymentevents "polyglot-ticketing-v1/contracts/payments"
)

const (
	paymentsDeadLetterStreamName = "PAYMENTS_DLQ"
	orderEventMaxRetries         = uint64(5)
	paymentsDeadLetterHeader     = "X-Payments-DLQ-"
)

var paymentsDeadLetterSubjects = []string{
	paymentevents.OrderProjectionDeadLetterSubject,
}

type paymentsDeadLetter struct {
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

type paymentsDeadLetterer interface {
	Park(context.Context, paymentsDeadLetter) error
}

type jetStreamPaymentsDeadLetterer struct {
	js nats.JetStreamContext
}

func ensurePaymentsDeadLetterStream(js nats.JetStreamContext) error {
	info, err := js.StreamInfo(paymentsDeadLetterStreamName)
	if errors.Is(err, nats.ErrStreamNotFound) {
		_, addErr := js.AddStream(&nats.StreamConfig{
			Name:       paymentsDeadLetterStreamName,
			Subjects:   append([]string(nil), paymentsDeadLetterSubjects...),
			Storage:    nats.FileStorage,
			Retention:  nats.LimitsPolicy,
			MaxAge:     7 * 24 * time.Hour,
			Duplicates: 7 * 24 * time.Hour,
		})
		if addErr == nil {
			return nil
		}
		info, err = js.StreamInfo(paymentsDeadLetterStreamName)
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
	for _, subject := range paymentsDeadLetterSubjects {
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

func (deadLetterer jetStreamPaymentsDeadLetterer) Park(ctx context.Context, event paymentsDeadLetter) error {
	headers := clonePaymentsHeaders(event.OriginalHeader)
	headers.Set("Nats-Msg-Id", fmt.Sprintf("payments-dlq-v1:%s:%d", event.OriginalStream, event.OriginalSeq))
	headers.Set(paymentsDeadLetterHeader+"Original-Subject", event.OriginalSubject)
	headers.Set(paymentsDeadLetterHeader+"Original-Stream", event.OriginalStream)
	headers.Set(paymentsDeadLetterHeader+"Original-Stream-Sequence", strconv.FormatUint(event.OriginalSeq, 10))
	headers.Set(paymentsDeadLetterHeader+"Consumer", event.Consumer)
	headers.Set(paymentsDeadLetterHeader+"Delivery-Count", strconv.FormatUint(event.DeliveryCount, 10))
	headers.Set(paymentsDeadLetterHeader+"Failure-Class", event.FailureClass)
	headers.Set(paymentsDeadLetterHeader+"Failure-Reason", strings.ReplaceAll(strings.ReplaceAll(event.FailureReason, "\r", " "), "\n", " "))
	headers.Set(paymentsDeadLetterHeader+"Parked-At", time.Now().UTC().Format(time.RFC3339Nano))

	_, err := deadLetterer.js.PublishMsg(&nats.Msg{
		Subject: event.Subject,
		Header:  headers,
		Data:    event.Payload,
	}, nats.Context(ctx))
	return err
}

// ReplayPaymentsDeadLetter republishes the original Payments-consumed order event represented by a retained DLQ message.
// It intentionally leaves the DLQ record in place for audit; Payments consumers make a repeat replay idempotent by event ID.
func ReplayPaymentsDeadLetter(ctx context.Context, js nats.JetStreamContext, sequence uint64) (*nats.PubAck, error) {
	message, err := js.GetMsg(paymentsDeadLetterStreamName, sequence)
	if err != nil {
		return nil, fmt.Errorf("get Payments DLQ message %d: %w", sequence, err)
	}
	if message.Subject != paymentevents.OrderProjectionDeadLetterSubject {
		return nil, fmt.Errorf("message %d is not a Payments DLQ message", sequence)
	}

	subject := message.Header.Get(paymentsDeadLetterHeader + "Original-Subject")
	if !isPaymentsConsumedOrderSubject(subject) {
		return nil, fmt.Errorf("message %d has unsupported original subject %q", sequence, subject)
	}

	headers := nats.Header{}
	for key, values := range message.Header {
		if strings.HasPrefix(key, paymentsDeadLetterHeader) || strings.EqualFold(key, "Nats-Msg-Id") {
			continue
		}
		headers[key] = append([]string(nil), values...)
	}
	headers.Set("Nats-Msg-Id", fmt.Sprintf("payments-dlq-replay-v1:%d", sequence))

	ack, err := js.PublishMsg(&nats.Msg{Subject: subject, Header: headers, Data: message.Data}, nats.Context(ctx))
	if err != nil {
		return nil, fmt.Errorf("republish Payments DLQ message %d: %w", sequence, err)
	}
	return ack, nil
}

func newPaymentsDelivery(message *nats.Msg) (paymentsDelivery, error) {
	metadata, err := message.Metadata()
	if err != nil {
		return paymentsDelivery{}, err
	}
	return paymentsDelivery{
		delivery:      message,
		deliveryCount: metadata.NumDelivered,
		headers:       message.Header,
		payload:       message.Data,
		stream:        metadata.Stream,
		streamSeq:     metadata.Sequence.Stream,
		subject:       message.Subject,
	}, nil
}

type paymentsDelivery struct {
	delivery      orderEventDelivery
	deliveryCount uint64
	headers       nats.Header
	payload       []byte
	stream        string
	streamSeq     uint64
	subject       string
}

func (event paymentsDelivery) deadLetter(consumer, failureClass string, failure error) paymentsDeadLetter {
	return paymentsDeadLetter{
		Consumer:        consumer,
		DeliveryCount:   event.deliveryCount,
		FailureClass:    failureClass,
		FailureReason:   failure.Error(),
		OriginalHeader:  event.headers,
		OriginalStream:  event.stream,
		OriginalSeq:     event.streamSeq,
		OriginalSubject: event.subject,
		Payload:         event.payload,
		Subject:         paymentevents.OrderProjectionDeadLetterSubject,
	}
}

func isPaymentsConsumedOrderSubject(subject string) bool {
	return subject == orderevents.OrderCreatedSubject || subject == orderevents.OrderCanceledSubject
}

func clonePaymentsHeaders(headers nats.Header) nats.Header {
	clone := make(nats.Header, len(headers)+8)
	for key, values := range headers {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}
