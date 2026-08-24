import type { IncomingHttpHeaders } from 'node:http';
import type { CurrentUserResponse } from './identity.types';

export type AuthenticatedUser = CurrentUserResponse['user'];

export interface AuthenticatedRequest {
  headers: IncomingHttpHeaders;
  user?: AuthenticatedUser;
}
