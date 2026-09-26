package validation

import (
	"context"
	"net/http"

	"github.com/omniaura/go-kit/errs"
)

var MissingRequiredField = errs.NewFactory(http.StatusUnprocessableEntity, "missing required fields",
	errs.WithCode("missing_required_fields"), errs.WithAction(errs.ActionFixInput))

func CheckEmptyStringFields(ctx context.Context, pairs ...string) *errs.Error {
	if len(pairs)%2 != 0 {
		panic("CheckEmptyStringFields requires pairs of field name and value")
	}
	var e *errs.Error
	for i := 0; i < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			continue
		}
		if e == nil {
			e = MissingRequiredField.New(ctx)
		}
		e.Field(pairs[i], "is required")
	}
	return e
}
