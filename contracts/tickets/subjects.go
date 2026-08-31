// Package ticketevents defines public routing contracts for ticket events.
package ticketevents

const (
	TicketCreatedSubject               = "tickets.ticket.created.v1"
	TicketUpdatedSubject               = "tickets.ticket.updated.v1"
	OrderCancellationDeadLetterSubject = "dlq.tickets.order-cancellation.v1"
	OrderReservationDeadLetterSubject  = "dlq.tickets.order-reservation.v1"
)
