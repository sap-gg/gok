package internal

import (
	"errors"
	"fmt"
	"io"

	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
)

// NewYAMLDecoder creates a new YAML decoder with strict mode and validation enabled.
func NewYAMLDecoder(reader io.Reader, opts ...yaml.DecodeOption) *yaml.Decoder {
	validate := validator.New()
	return yaml.NewDecoder(reader,
		append(opts,
			yaml.Strict(),
			yaml.Validator(validate))...)
}

// NewYAMLEncoder creates a new YAML encoder with an indentation of 2 spaces.
func NewYAMLEncoder(writer io.Writer, opts ...yaml.EncodeOption) *yaml.Encoder {
	return yaml.NewEncoder(writer,
		append(opts, yaml.Indent(2))...)
}

// IsDecodeErrorAndPrint checks if the error is a YAML decoding error.
// If it is, it prints the formatted error and returns true.
func IsDecodeErrorAndPrint(err error) bool {
	var yamlError yaml.Error
	if errors.As(err, &yamlError) {
		fmt.Println(yamlError.FormatError(true, true))
		return true
	}
	return false
}
