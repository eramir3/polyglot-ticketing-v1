// Package orderevents defines public routing contracts for order events.
package orderevents

const (
	OrderCanceledSubject                = "orders.order.canceled.v1"
	OrderCreatedSubject                 = "orders.order.created.v1"
	TicketProjectionDeadLetterSubject   = "dlq.orders.ticket-projection.v1"
	ExpirationCompleteDeadLetterSubject = "dlq.orders.expiration-complete.v1"
	PaymentCreatedDeadLetterSubject     = "dlq.orders.payment-created.v1"
	PaymentSucceededDeadLetterSubject   = "dlq.orders.payment-succeeded.v1"
	PaymentFailedDeadLetterSubject      = "dlq.orders.payment-failed.v1"
)
