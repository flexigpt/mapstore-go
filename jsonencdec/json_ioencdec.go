package jsonencdec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type JSONEncoderDecoder struct{}

// Encode encodes the given value into JSON format and writes it to the writer.
func (d JSONEncoderDecoder) Encode(w io.Writer, value any) error {
	if w == nil {
		return errors.New("writer cannot be nil")
	}

	encoder := json.NewEncoder(w)
	// For pretty output.
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("failed to encode value: %w", err)
	}

	return nil
}

// Decode decodes JSON data from the reader into the given value.
func (d JSONEncoderDecoder) Decode(r io.Reader, value any) error {
	if r == nil {
		return errors.New("reader cannot be nil")
	}

	if _, err := requireNonNilPointer(value, "value"); err != nil {
		return err
	}

	decoder := json.NewDecoder(r)

	decoder.DisallowUnknownFields()

	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}

	return nil
}
