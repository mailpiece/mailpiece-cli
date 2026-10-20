package mailpiece

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// ValidateNumber holds a record's number to the app's form: a whole number from 1.
func ValidateNumber(number int64) error {
	if number <= 0 {
		return ErrUsage(fmt.Sprintf("%d is not a number: numbers start at 1", number))
	}
	return nil
}

// segment is a path segment the SDK supplies itself, such as a kind's plural.
type segment string

// Path builds an API path from format and args: a string is an id, escaped; an
// int64 is a number. It is how a list's own path is built to ask for another of
// its pages.
func Path(format string, args ...any) (string, error) {
	return buildPath(format, args...)
}

// buildPath fills format's verbs with args, each held to its form: a string is an
// id, escaped; an int64 is a number; a segment is escaped as it is.
func buildPath(format string, args ...any) (string, error) {
	filled := make([]any, len(args))
	for i, arg := range args {
		switch v := arg.(type) {
		case string:
			filled[i] = url.PathEscape(v)
		case int64:
			if err := ValidateNumber(v); err != nil {
				return "", err
			}
			filled[i] = v
		case segment:
			filled[i] = url.PathEscape(string(v))
		default:
			panic(fmt.Sprintf("mailpiece: buildPath takes ids, numbers and segments, got %T", arg))
		}
	}
	return fmt.Sprintf(format, filled...), nil
}

// fetch GETs the record at the path format and args build, into a T.
func fetch[T any](ctx context.Context, ac *AccountClient, format string, args ...any) (*T, *Response, error) {
	path, err := buildPath(format, args...)
	if err != nil {
		return nil, nil, err
	}
	return decode[T](ac.Get(ctx, path))
}

// send makes a write at the path format and args build, and reads what it
// answers into a T.
func send[T any](ctx context.Context, ac *AccountClient, method string, body any, format string, args ...any) (*T, *Response, error) {
	resp, err := sendBare(ctx, ac, method, body, format, args...)
	return decode[T](resp, err)
}

// sendBare makes a write at the path format and args build, for an endpoint
// that answers no body worth reading.
func sendBare(ctx context.Context, ac *AccountClient, method string, body any, format string, args ...any) (*Response, error) {
	path, err := buildPath(format, args...)
	if err != nil {
		return nil, err
	}
	switch method {
	case "POST":
		return ac.Post(ctx, path, body)
	case "PATCH":
		return ac.Patch(ctx, path, body)
	case "PUT":
		return ac.Put(ctx, path, body)
	case "DELETE":
		return ac.Delete(ctx, path)
	}
	panic("mailpiece: sendBare takes a write method, got " + method)
}

func decode[T any](resp *Response, err error) (*T, *Response, error) {
	if err != nil {
		return nil, nil, err
	}
	var v T
	if err := resp.UnmarshalData(&v); err != nil {
		return nil, resp, err
	}
	return &v, resp, nil
}

func decodeList[T any](resp *Response, err error) ([]T, *Response, error) {
	if err != nil {
		return nil, nil, err
	}
	var v []T
	if err := json.Unmarshal(resp.Data, &v); err != nil {
		return nil, resp, err
	}
	return v, resp, nil
}
