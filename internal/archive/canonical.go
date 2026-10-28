// Package archive stores original captures and their accumulated observed blocks.
package archive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Canonical encodes deterministic, compact JSON using the standard encoder.
// Number tokens retain their original precision through json.Number.
func Canonical(v any) (string, error) {
	if err := validateJSON(v); err != nil {
		return "", err
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}

// encoding/json replaces invalid UTF-8. Reject it instead so stored source
// values cannot silently change. The encoder checks number syntax and floats;
// additionally reject decimal/exponent tokens outside the finite float range.
func validateJSON(v any) error {
	switch value := v.(type) {
	case string:
		if !utf8.ValidString(value) {
			return fmt.Errorf("text must contain valid Unicode")
		}
	case json.Number:
		if value == "" {
			return fmt.Errorf("invalid JSON number")
		}
		if strings.ContainsAny(string(value), ".eE") {
			if _, err := strconv.ParseFloat(string(value), 64); err != nil {
				return fmt.Errorf("JSON number must be finite: %w", err)
			}
		}
	case map[string]any:
		for key, item := range value {
			if err := validateJSON(key); err != nil {
				return err
			}
			if err := validateJSON(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range value {
			if err := validateJSON(item); err != nil {
				return err
			}
		}
	case []map[string]any:
		for _, item := range value {
			if err := validateJSON(item); err != nil {
				return err
			}
		}
	case []string:
		for _, item := range value {
			if err := validateJSON(item); err != nil {
				return err
			}
		}
	case nil, bool, int, int64, float64:
	default:
		return fmt.Errorf("unsupported JSON value %T", v)
	}
	return nil
}

func Decode(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	err := decoder.Decode(&value)
	return value, err
}
