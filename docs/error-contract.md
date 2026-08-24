# Error Contract

The API gateway is the public error boundary. REST endpoints respond with:

```json
{
  "errors": [
    {
      "message": "Email must be valid",
      "code": "INVALID_EMAIL",
      "field": "email"
    }
  ]
}
```

GraphQL endpoints use the native GraphQL `errors` array. Each item has its
message at the top level and its application code and optional field in
`extensions`.

Services communicate errors over gRPC using the relevant gRPC status code and
the JSON representation of `common.v1.ErrorResponse` in the status details.
This keeps the contract usable by NestJS, Go, and Python until protobuf code
generation is introduced.

## Public Codes

| Code                         | Meaning                                                 |
| ---------------------------- | ------------------------------------------------------- |
| `INVALID_ARGUMENT`           | A request is invalid but no more specific code applies. |
| `INVALID_NAME`               | The `name` field is missing or invalid.                 |
| `INVALID_EMAIL`              | The `email` field is missing or invalid.                |
| `INVALID_PASSWORD`           | The `password` field is missing or invalid.             |
| `INVALID_TITLE`              | The `title` field is missing or invalid.                |
| `INVALID_PRICE`              | The `price` field is missing or invalid.                |
| `INVALID_CREDENTIALS`        | The supplied email or password is incorrect.            |
| `EMAIL_NOT_VERIFIED`         | The account email must be verified before sign-in.      |
| `INVALID_VERIFICATION_TOKEN` | An email verification token is invalid or expired.      |
| `ALREADY_EXISTS`             | The requested resource already exists.                  |
| `UNAUTHENTICATED`            | Authentication is required or invalid.                  |
| `FORBIDDEN`                  | The caller lacks permission.                            |
| `NOT_FOUND`                  | The requested resource does not exist.                  |
| `RATE_LIMITED`               | The caller exceeded a rate limit.                       |
| `SERVICE_UNAVAILABLE`        | A required service is unavailable.                      |
| `INTERNAL_ERROR`             | An unexpected error occurred.                           |

`field`, when present, uses the public lower-camel-case request field name.
Service implementation details, stack traces, and database errors must not be
included in public error messages.
