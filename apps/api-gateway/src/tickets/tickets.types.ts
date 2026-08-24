import { Observable } from 'rxjs';

export interface CreateTicketRequest {
  title: string;
  price: number;
  userId: string;
}

export interface CreateTicketResponse {
  id: string;
  title: string;
  price: number;
  userId: string;
}

export interface ListTicketsRequest {}

export interface ListTicketsResponse {
  tickets: Ticket[];
}

export interface Ticket {
  id: string;
  title: string;
  price: number;
  userId: string;
}

export interface TicketsGrpcService {
  createTicket(request: CreateTicketRequest): Observable<CreateTicketResponse>;
  listTickets(request: ListTicketsRequest): Observable<ListTicketsResponse>;
}
