// Package errs is errors as values for HTTP services and the clients they call.
//
// An error answers five questions, and every one of them is declared on the
// error rather than re-derived by whoever catches it:
//
//  1. When can you try again?      RetryPolicy, RetryAfter
//  2. What can you do about it?    Action
//  3. How serious is it?           Severity
//  4. What does the client see?    Message, Code, Ref, Fields, Params
//  5. What do we keep?             AddError, AddAttrs, AddMessage, Upstream → log + Sinks
//
// The builder keeps the last two strictly apart. Client-facing methods (Action,
// Field, Param, RetryAfter) shape the response body; server-only methods
// (AddError, AddErrors, AddAttrs, AddMessage, Upstream, Log) go to the log line
// and to every registered Sink (a database table, an audit log), never to the
// wire. There is no method that copies an error's text into the response: the
// factory message is fixed at declaration, so an internal detail can only reach
// a client if someone writes it into a factory message on purpose.
//
//	var ErrKeyUnreadable = errs.NewFactory(http.StatusUnprocessableEntity,
//		"Your saved provider key could not be read. Re-enter it to continue.",
//		errs.WithCode("provider_key_unreadable"),
//		errs.WithAction(errs.ActionUpdateSettings),
//	)
//
//	if err != nil {
//		ErrKeyUnreadable.New(ctx).AddError(err).AddAttrs(errs.String("provider", p)).Abort(w)
//		return
//	}
//
// Abort writes the client view with the configured Encoder, logs everything at
// the level the Severity implies, and hands a Record to the Sinks when the
// error is serious enough to keep. Every step carries the same Ref — by
// default the request id from zerolog's hlog — so the string a user copies out
// of the app finds the log line and the stored row.
//
// Transports that are not an HTTP response (a websocket frame, an SSE event, a
// queue message) call Emit instead of Abort: it does the logging and recording
// and returns the Public view to put in the frame.
//
// The same values flow the other way through net/hit: a request can map an
// upstream's statuses and typed error bodies onto factories, so an SDK built
// on hit returns *Error values that already know whether to retry, what the
// caller can do, and which upstream detail to keep server-side.
package errs
