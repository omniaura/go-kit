package validation

import (
	"context"
	"net/http"

	"github.com/omniaura/go-kit/errs"
)

var MissingRequiredField = errs.NewFactory(http.StatusUnprocessableEntity, "missing required fields")

func CheckEmptyStringFields(ctx context.Context, pairs ...string) *errs.Error {
	if len(pairs)%2 != 0 {
		panic("CheckEmptyStringFields requires pairs of field name and value")
	}
	var fieldsMissing []string
	for i := 0; i < len(pairs); i += 2 {
		name := pairs[i]
		value := pairs[i+1]
		if value == "" {
			fieldsMissing = append(fieldsMissing, name)
		}
	}
	if len(fieldsMissing) > 0 {
		return MissingRequiredField.New(ctx).Strs(fieldsMissing)
	}
	return nil
}
