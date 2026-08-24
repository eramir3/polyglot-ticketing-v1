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

export interface GetTicketRequest {
  id: string;
}

export interface GetTicketResponse {
  ticket: Ticket;
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
  getTicket(request: GetTicketRequest): Observable<GetTicketResponse>;
  listTickets(request: ListTicketsRequest): Observable<ListTicketsResponse>;
}
