# errs

Errors as values for HTTP services and the clients they call.

An error has to answer five questions. `errs` makes each one a declaration on
the error instead of something every caller re-derives:

| Question | Declared with | Reaches the client? |
|---|---|---|
| When can you try again? | `WithRetryPolicy`, `RetryAfter`, `StatusRetryPolicy` | `retryable`, `retryAfterMs`, `Retry-After` header |
| What can you do about it? | `WithAction` / `Action` | `action` (the app turns it into a button) |
| How serious is it? | `WithSeverity` (defaults from status) | no: picks the log level and whether it is recorded |
| What does the client see? | the factory message, `WithCode`, `Field`, `Param` | yes: `message`, `code`, `ref`, `fields`, `params` |
| What do we keep? | `AddError`, `AddAttrs`, `AddMessage`, `Upstream` | **never**: log line + every registered `Sink` |

## Client-facing vs server-only

The builder has two halves, and nothing crosses between them:

```go
var ErrKeyUnreadable = errs.NewFactory(http.StatusUnprocessableEntity,
	"Your saved provider key could not be read. Re-enter it to continue.",
	errs.WithCode("provider_key_unreadable"),
	errs.WithAction(errs.ActionUpdateSettings),
)

func handler(w http.ResponseWriter, r *http.Request) {
	key, err := decryptKey(r.Context(), uid)
	if err != nil {
		ErrKeyUnreadable.New(r.Context()).
			AddError(err).                           // server-only
			AddAttrs(errs.String("provider", "gm")). // server-only
			Abort(w)                                 // respond + log + record
		return
	}
}
```

The client receives exactly:

```json
{"message":"Your saved provider key could not be read. Re-enter it to continue.","code":"provider_key_unreadable","ref":"d3k1q8a0p8m2","action":"update_settings","status":422}
```

and the log line carries `error`, `provider`, `error_ref`, `severity` and
`error_code`. No method copies an error's text into the response. The factory
message is fixed when it is declared, so an internal detail can only reach a
client if someone deliberately writes it into a message.

Client-facing: `Action`, `Field(name, problem)`, `Param(key, value)`,
`RetryAfter(d)`, `WithRetryPolicy`, `Severity`.
Server-only: `AddError`, `AddErrors`, `AddAttrs`, `AddMessage`, `Upstream`,
`Source`, `Log`.

`AddError` also keeps the error as a cause, so `errors.Is(err, context.Canceled)`
works through an `*errs.Error`. `Error()` only ever contains the status and the
public message.

## One reference everywhere

Every error has a `ref`. By default it is the request id that zerolog's `hlog`
puts on each log line, so the string a user copies out of the app finds both
the log line and the stored record. Work outside a request mints its own ref
with `WithNewRef(ctx)`.

## Writers

`Abort(w)` does three things, each through its own writer:

1. **Response.** The `Public` view, rendered by the configured `Encoder`:
   `errs.JSON` (the default) or `errs.ProblemJSON` / `errs.ProblemEncoder{TypeBase: …}`
   for RFC 9457. It sets `Retry-After` when there is a hint.
2. **Log.** zerolog, at the level implied by `Severity`.
3. **Record.** Every `Sink` registered with `errs.AddSink`, when severity is Error
   or worse, or when `WithRecord(true)` forces it (use that for audit-worthy 4xx).
   Sinks run on the request goroutine, so buffer and write asynchronously.

Other transports, such as websocket frames, SSE events and queue messages, call
`e.Emit("chat_stream")`. It logs and records the error, then returns the
`Public` view to put in the frame.

## Retry

`RetryPolicy` carries attempts, backoff, exponential growth, a backoff cap, and
`MaxRetryAfter`, which is how long a loop may wait on a server's `Retry-After`
before giving up and passing the hint on. `policy.Next(attempt, err)` combines
the policy with the error's hint. `TransientStatusRetry(policy)` is a starting
per-status table (408, 425, 429, 5xx); override single statuses with
`.With(409, …)`.

## Go 1.27: typed reads off the error

This module requires Go 1.27 for generic methods, which let an error answer
typed questions directly:

```go
id, ok := e.AttrAs[string]("user_id")            // a server-only attribute, typed
body, ok := e.UpstreamAs[anthropicError]()       // the provider's decoded error body
```

`UpstreamAs` is how a `net/hit` caller branches on a provider's own error type
without re-parsing the body. The same Go feature is what makes
`hit.Client[E].GET[Out]` possible. Interface methods still cannot have type
parameters, so these are concrete methods on `*Error`.

## Building SDKs with `net/hit`

`hit` returns these same values. Every request states two schemas, response
`Out` and error `E`. A `hit.Client[E]` holds one provider's shared
configuration and error schema, and its `Classify` and `ErrorMap` map typed
error bodies and statuses onto factories. The resulting `*errs.Error` knows its
retry policy and what the caller can do, and it keeps the provider's status,
body, error type and request id server-side on `errs.Upstream`. That makes a
provider client a few declarations instead of an imported SDK. See
`net/hit/README.md`.
