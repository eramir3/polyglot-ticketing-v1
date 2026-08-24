export interface SignInRequest {
  email: string;
  password: string;
}

export interface SignInResponse {
  userId: string;
  name: string;
  email: string;
  emailVerified: boolean;
  sessionToken: string;
  sessionExpiresAt: {
    seconds: string;
    nanos: number;
  };
}

export interface SignOutRequest {}

export interface SignOutResponse {}

export interface CurrentUserRequest {}

export interface CurrentUserResponse {
  userId: string;
  name: string;
  email: string;
  emailVerified: boolean;
}

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
