// Package orderevents defines public routing contracts for order events.
package orderevents

const (
	OrderCanceledSubject              = "orders.order.canceled.v1"
	OrderCreatedSubject               = "orders.order.created.v1"
	TicketProjectionDeadLetterSubject = "dlq.orders.ticket-projection.v1"
)
