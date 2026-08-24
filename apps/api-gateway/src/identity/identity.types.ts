import { Metadata } from '@grpc/grpc-js';
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

export interface SignInRequest {
  email: string;
  password: string;
}

export interface SignInResponse {
  user: {
    id: string;
    name: string;
    email: string;
    emailVerified: boolean;
  };
  session: {
    expiresAt: string;
  };
}

export interface IdentitySignInResponse {
  userId: string;
  name: string;
  email: string;
  emailVerified: boolean;
  sessionToken: string;
  sessionExpiresAt: Timestamp;
}

export interface SignOutRequest {}

export interface SignOutResponse {}

export interface Timestamp {
  seconds: string | number | bigint;
  nanos: number;
}

export interface VerifyEmailRequest {
  token: string;
}

export interface VerifyEmailResponse {
  verified: boolean;
}

export interface IdentityGrpcService {
  signUp(request: SignUpRequest): Observable<SignUpResponse>;
  signIn(
    request: SignInRequest,
    metadata?: Metadata,
  ): Observable<IdentitySignInResponse>;
  signOut(
    request: SignOutRequest,
    metadata?: Metadata,
  ): Observable<SignOutResponse>;
  verifyEmail(request: VerifyEmailRequest): Observable<VerifyEmailResponse>;
}
