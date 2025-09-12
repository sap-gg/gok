package internal

import (
	"io"

	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
)

// NewYAMLDecoder creates a new YAML decoder with strict mode and validation enabled.
func NewYAMLDecoder(reader io.Reader) *yaml.Decoder {
	validate := validator.New()
	return yaml.NewDecoder(reader,
		yaml.Strict(),
		yaml.Validator(validate))
}

func NewYAMLEncoder(writer io.Writer) *yaml.Encoder {
	return yaml.NewEncoder(writer, yaml.Indent(2))
}
