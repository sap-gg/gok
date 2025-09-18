package artifact

import "fmt"

const SpecVersion = 1

// Spec defines the structure of the artifact specification file.
type Spec struct {
	Version   int    `yaml:"version"`
	Algorithm string `yaml:"algorithm"`
	Checksum  string `yaml:"checksum"`
	Source    Source `yaml:"source"`
}

// Source defines where to fetch the artifact from.
type Source struct {
	HTTP *HTTPSource `yaml:"http,omitempty"`
}

// HTTPSource defines the HTTP source details for fetching the artifact.
type HTTPSource struct {
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

// Validate checks if the Spec is valid.
func (s *Spec) Validate() error {
	if s.Version != SpecVersion {
		return fmt.Errorf("unsupported artifact spec version: %d", s.Version)
	}
	if s.Checksum == "" {
		return fmt.Errorf("checksum is required")
	}
	if s.Algorithm != "sha256" {
		return fmt.Errorf("unsupported checksum algorithm: %s", s.Algorithm)
	}
	if s.Source.HTTP == nil {
		return fmt.Errorf("unsupported source type: only HTTP is supported")
	}
	if s.Source.HTTP.URL == "" {
		return fmt.Errorf("http source url is required")
	}
	return nil
}
