export interface SignUpRequest {
  name: string;
  email: string;
  password: string;
}

export interface SignUpResponse {
  userId: string;
  email: string;
}
