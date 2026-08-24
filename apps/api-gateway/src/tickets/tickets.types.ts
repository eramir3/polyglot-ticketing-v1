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

export interface TicketsGrpcService {
  createTicket(request: CreateTicketRequest): Observable<CreateTicketResponse>;
}
