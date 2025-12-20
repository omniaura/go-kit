# errs

A structured error handling package for Go HTTP services with built-in zerolog integration, JSON serialization, and the abort pattern.

## Installation

```bash
go get github.com/omniaura/go-kit/errs
```

## Features

- **Error Factories** — Define reusable error types with HTTP status codes and messages
- **Zerolog Integration** — Automatic structured logging with configurable log levels
- **JSON Responses** — Errors serialize to clean JSON for API responses
- **Method Chaining** — Fluent API for annotating errors with context
- **Abort Pattern** — One-liner error handling in HTTP handlers
- **Error Matching** — `Is`/`Not` methods compatible with Go's error handling idioms

## Usage

### Defining Error Factories

Create error factories at package level for reusable error types:

```go
package myapi

import (
    "net/http"

    "github.com/omniaura/go-kit/errs"
    "github.com/rs/zerolog"
)

var (
    ErrNotFound     = errs.NewFactory(http.StatusNotFound, "resource not found")
    ErrUnauthorized = errs.NewFactory(http.StatusUnauthorized, "unauthorized")
    ErrBadRequest   = errs.NewFactory(http.StatusBadRequest, "invalid request")
    
    // With custom log level
    ErrRateLimit = errs.NewFactory(http.StatusTooManyRequests, "rate limit exceeded", 
        errs.WithLevel(zerolog.WarnLevel))
)
```

### The Abort Pattern

Functions that only return an error should return `*errs.Error`. This enables a clean chain pattern for early returns in HTTP handlers:

```go
// Function returns only *errs.Error
func ValidateUserID(ctx context.Context, userID string) *errs.Error {
    if userID == "" {
        return ErrBadRequest.New(ctx)
    }
    if !isValidUUID(userID) {
        return ErrBadRequest.New(ctx).Strs([]string{"invalid user ID format"})
    }
    return nil
}

// Handler uses the abort pattern
func HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    if ValidateUserID(ctx, r.PathValue("id")).Abort(w) {
        return
    }
    
    // continue processing...
}
```

`Abort` writes the JSON error response, logs via zerolog, and returns `true` if the error is non-nil.

#### Aborting Any Error

Use `AsError` to abort errors from external libraries that return `error`:

```go
if errs.AsError(ctx, externalLib.DoSomething()).Abort(w) {
    return
}
```

`AsError` returns `nil` if the error is `nil`, so the chain works seamlessly.

#### When to Use Tuples

If you need to return a value alongside the error, use the standard `error` interface:

```go
func GetUser(ctx context.Context, id string) (*User, error) {
    user, err := db.FindUser(ctx, id)
    if err != nil {
        return nil, ErrNotFound.New(ctx).Err(err)
    }
    return user, nil
}
```

This allows reusing `var err error` across multiple calls while still benefiting from structured errors.

### Annotating Errors

Chain methods to add context to errors:

```go
// Add error message
err := ErrBadRequest.New(ctx).Err(parseErr)

// Add string slice (joins with ", ")
err := ErrBadRequest.New(ctx).Strs([]string{"field1", "field2"})

// Add custom log fields
err := ErrNotFound.New(ctx).
    Err(dbErr).
    Log(func(e *zerolog.Event) {
        e.Str("user_id", userID).
          Str("action", "lookup")
    })
```

### Error Matching

Check if an error matches a specific factory:

```go
err := doSomething()

if ErrNotFound.Is(err) {
    // Handle not found case
}

if ErrNotFound.Not(err) {
    // Handle any other error
}
```

### Converting Unknown Errors

Wrap arbitrary errors as `*errs.Error`:

```go
err := someExternalLibrary()
e := errs.AsError(ctx, err) // Wraps as Unknown (500) if not already an *errs.Error
```

### JSON Response Format

Errors serialize to JSON automatically:

```json
{
    "message": "resource not found: record does not exist",
    "status": 404
}
```

## Validation Subpackage

The `validation` subpackage provides common validation helpers:

```go
import "github.com/omniaura/go-kit/errs/validation"

func CreateUser(ctx context.Context, name, email string) error {
    if err := validation.CheckEmptyStringFields(ctx,
        "name", name,
        "email", email,
    ); err != nil {
        return err
    }
    // ...
}
```

Returns a `422 Unprocessable Entity` with the missing field names.

## API Reference

### ErrorFactory

| Method | Description |
|--------|-------------|
| `NewFactory(status int, msg string, opts ...optFunc)` | Create a new error factory |
| `(f ErrorFactory) New(ctx context.Context) *Error` | Create a new error instance |
| `(f ErrorFactory) Is(err error) bool` | Check if error matches this factory |
| `(f ErrorFactory) Not(err error) bool` | Check if error does not match |

### Error

| Method | Description |
|--------|-------------|
| `Err(err error) *Error` | Annotate with an error message |
| `Strs(values []string) *Error` | Annotate with string slice |
| `Log(f func(*zerolog.Event)) *Error` | Add custom log fields |
| `Abort(w http.ResponseWriter) bool` | Write response & log; returns true if error exists |
| `Message() string` | Get the full error message |
| `Error() string` | Implements `error` interface |
| `Is(err error) bool` | Check if errors match |
| `Not(err error) bool` | Check if errors don't match |
| `MarshalJSON() ([]byte, error)` | JSON serialization |

### Options

| Function | Description |
|----------|-------------|
| `WithLevel(zerolog.Level)` | Set the log level for this error factory |

## License

See [LICENSE](../LICENSE) in the repository root.

