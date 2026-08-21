import { Observable } from 'rxjs';

export interface SignUpRequest {
  name: string;
  email: string;
  password: string;
}

export interface SignUpResponse {
  userId: string;
  email: string;
}

export interface VerifyEmailRequest {
  token: string;
}

export interface VerifyEmailResponse {
  verified: boolean;
}

export interface IdentityGrpcService {
  signUp(request: SignUpRequest): Observable<SignUpResponse>;
  verifyEmail(request: VerifyEmailRequest): Observable<VerifyEmailResponse>;
}
