package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"

	"github.com/JustinSornay/go_skeleton/internal/platform/apperr"
)

var (
	validateOnce sync.Once
	validate     *validator.Validate
)

// Validator returns the shared validator. Field names in validation errors
// use the `json` (or `query`) tag so clients see the names they sent.
func Validator() *validator.Validate {
	validateOnce.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())
		validate.RegisterTagNameFunc(func(f reflect.StructField) string {
			if f.Anonymous {
				return embeddedMarker
			}
			for _, tag := range []string{"json", "query"} {
				name, _, _ := strings.Cut(f.Tag.Get(tag), ",")
				if name == "-" {
					return ""
				}
				if name != "" {
					return name
				}
			}
			return f.Name
		})
	})
	return validate
}

// ErrMalformedBody is returned when the request body is not valid JSON for
// the expected type.
var ErrMalformedBody = apperr.Invalid("request.malformed_body", "request body is not valid JSON for this endpoint")

// DecodeJSON decodes a single JSON object from the body into a T.
// Unknown fields are rejected so client typos fail loudly instead of being
// silently ignored. Body size is bounded by the BodyLimit middleware.
func DecodeJSON[T any](r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return v, apperr.New(apperr.KindTooLarge, "request.too_large",
				fmt.Sprintf("request body exceeds %d bytes", maxErr.Limit))
		}
		if errors.Is(err, io.EOF) {
			return v, apperr.Invalid("request.empty_body", "request body is empty")
		}
		return v, ErrMalformedBody.Wrap(err)
	}
	if dec.More() {
		return v, apperr.Invalid("request.malformed_body", "request body must contain a single JSON value")
	}
	return v, nil
}

// Bind decodes the JSON body into a T and validates it.
func Bind[T any](r *http.Request) (T, error) {
	v, err := DecodeJSON[T](r)
	if err != nil {
		return v, err
	}
	return v, Validate(v)
}

// Validate runs struct validation and converts failures to an
// apperr.KindInvalid error listing every offending field.
func Validate(v any) error {
	err := Validator().Struct(v)
	if err == nil {
		return nil
	}
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return apperr.Internal(err)
	}
	fields := make([]apperr.FieldError, 0, len(verrs))
	for _, fe := range verrs {
		fields = append(fields, apperr.FieldError{
			Field:   fieldPath(fe),
			Rule:    fe.Tag(),
			Message: fieldMessage(fe),
		})
	}
	return apperr.Invalid("request.invalid", "request validation failed", fields...)
}

// embeddedMarker names embedded structs in validation namespaces so that
// fieldPath can drop them: clients never see Go type names.
const embeddedMarker = "~"

// fieldPath turns a validator namespace into the client-facing field path:
// "listRequest.~.limit" -> "limit", "createRequest.address.city" -> "address.city".
func fieldPath(fe validator.FieldError) string {
	parts := strings.Split(fe.Namespace(), ".")
	out := make([]string, 0, len(parts))
	for _, p := range parts[1:] { // parts[0] is the root struct
		if p != embeddedMarker {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return fe.Field()
	}
	return strings.Join(out, ".")
}

func fieldMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "min":
		return "must be at least " + fe.Param() + lengthUnit(fe)
	case "max":
		return "must be at most " + fe.Param() + lengthUnit(fe)
	case "oneof":
		return "must be one of: " + fe.Param()
	case "uuid", "uuid4", "uuid7":
		return "must be a valid UUID"
	case "email":
		return "must be a valid email address"
	default:
		return "failed rule " + fe.Tag()
	}
}

func lengthUnit(fe validator.FieldError) string {
	switch fe.Kind() {
	case reflect.String:
		return " characters"
	case reflect.Slice, reflect.Map, reflect.Array:
		return " items"
	default:
		return ""
	}
}

// BindQuery decodes URL query parameters into dst (a pointer to a struct)
// using `query:"name"` tags, then validates it.
//
// Supported field types: string, bool, signed/unsigned integers, floats,
// pointers to those (nil when the parameter is absent) and []string
// (repeated parameters or a comma-separated list).
func BindQuery(r *http.Request, dst any) error {
	if err := decodeQuery(r.URL.Query(), dst); err != nil {
		return err
	}
	return Validate(dst)
}

func decodeQuery(q map[string][]string, dst any) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return apperr.Internal(fmt.Errorf("BindQuery: dst must be a pointer to a struct, got %T", dst))
	}
	if fields := decodeStruct(q, rv.Elem()); len(fields) > 0 {
		return apperr.Invalid("request.invalid_query", "invalid query parameters", fields...)
	}
	return nil
}

// decodeStruct fills tagged fields, recursing into embedded structs (e.g.
// PageQuery) so shared parameter sets can be reused.
func decodeStruct(q map[string][]string, rv reflect.Value) []apperr.FieldError {
	var fields []apperr.FieldError
	rt := rv.Type()
	for i := range rt.NumField() {
		sf := rt.Field(i)
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
			fields = append(fields, decodeStruct(q, rv.Field(i))...)
			continue
		}
		key, _, _ := strings.Cut(sf.Tag.Get("query"), ",")
		if key == "" || key == "-" || !sf.IsExported() {
			continue
		}
		values := q[key]
		if len(values) == 0 || (len(values) == 1 && values[0] == "") {
			continue
		}
		if err := setField(rv.Field(i), values); err != nil {
			fields = append(fields, apperr.FieldError{Field: key, Rule: "type", Message: err.Error()})
		}
	}
	return fields
}

func setField(fv reflect.Value, values []string) error {
	if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.String {
		var out []string
		for _, v := range values {
			for part := range strings.SplitSeq(v, ",") {
				if part = strings.TrimSpace(part); part != "" {
					out = append(out, part)
				}
			}
		}
		fv.Set(reflect.ValueOf(out))
		return nil
	}
	if len(values) > 1 {
		return errors.New("must be provided once")
	}
	raw := values[0]
	switch fv.Kind() {
	case reflect.Pointer:
		ptr := reflect.New(fv.Type().Elem())
		if err := setField(ptr.Elem(), values); err != nil {
			return err
		}
		fv.Set(ptr)
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return errors.New("must be a boolean")
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, fv.Type().Bits())
		if err != nil {
			return errors.New("must be an integer")
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, fv.Type().Bits())
		if err != nil {
			return errors.New("must be a non-negative integer")
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, fv.Type().Bits())
		if err != nil {
			return errors.New("must be a number")
		}
		fv.SetFloat(f)
	default:
		return fmt.Errorf("unsupported field type %s", fv.Type())
	}
	return nil
}
