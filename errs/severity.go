package errs

import (
	"net/http"

	"github.com/rs/zerolog"
)

// Severity is how serious an error is for the service that raised it. It picks
// the log level and whether the error is kept by the Sinks.
type Severity uint8

const (
	// SeverityUnset derives the severity from the status: 5xx is Error, 499
	// (client closed) is Info, other 4xx are Warn.
	SeverityUnset Severity = iota
	// SeverityDebug is noise worth having while debugging only.
	SeverityDebug
	// SeverityInfo is an expected outcome, e.g. the client went away.
	SeverityInfo
	// SeverityWarn is an expected client-side failure: validation, auth, not found.
	SeverityWarn
	// SeverityError is a server fault someone should be able to look up.
	SeverityError
	// SeverityCritical is a server fault someone should look at now.
	SeverityCritical
)

// String returns the lower-case name used in logs and records.
func (s Severity) String() string {
	switch s {
	case SeverityDebug:
		return "debug"
	case SeverityInfo:
		return "info"
	case SeverityWarn:
		return "warn"
	case SeverityError:
		return "error"
	case SeverityCritical:
		return "critical"
	default:
		return "unset"
	}
}

// SeverityForStatus is the default severity of an error with status.
func SeverityForStatus(status int) Severity {
	switch {
	case status == 499:
		return SeverityInfo
	case status >= http.StatusInternalServerError:
		return SeverityError
	case status >= http.StatusBadRequest:
		return SeverityWarn
	default:
		return SeverityInfo
	}
}

func (s Severity) level() zerolog.Level {
	switch s {
	case SeverityDebug:
		return zerolog.DebugLevel
	case SeverityInfo:
		return zerolog.InfoLevel
	case SeverityWarn:
		return zerolog.WarnLevel
	default:
		return zerolog.ErrorLevel
	}
}

func severityForLevel(level zerolog.Level) Severity {
	switch {
	case level <= zerolog.DebugLevel:
		return SeverityDebug
	case level == zerolog.InfoLevel:
		return SeverityInfo
	case level == zerolog.WarnLevel:
		return SeverityWarn
	case level == zerolog.ErrorLevel:
		return SeverityError
	default:
		return SeverityCritical
	}
}
