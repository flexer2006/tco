package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/flexer2006/tco/internal/application"
	"github.com/flexer2006/tco/internal/domain"
)

type (
	store struct {
		atomic manifestAtomicWriter
		path   string
	}
)

func NewStore(path string) (application.ManifestStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("manifest path must not be empty")
	}
	return &store{path: path, atomic: newManifestAtomicWriter()}, nil
}

func (s *store) Load() (domain.Manifest, error) {
	if s == nil {
		return domain.Manifest{}, errors.New("jsonmanifest store must not be nil")
	}
	raw, err := s.atomic.readFile(s.path)
	if err != nil {
		return domain.Manifest{}, fmt.Errorf("load manifest %q: %w", s.path, err)
	}
	loaded, err := manifestFromBytes(raw)
	if err != nil {
		return domain.Manifest{}, fmt.Errorf("load manifest %q: %w", s.path, err)
	}
	return loaded, nil
}

func (s *store) Save(value domain.Manifest) (bool, error) {
	if s == nil {
		return false, errors.New("jsonmanifest store must not be nil")
	}
	if err := domain.Validate(value); err != nil {
		return false, err
	}
	raw, err := marshalManifest(value)
	if err != nil {
		return false, err
	}
	changed, err := s.atomic.write(s.path, raw)
	if err != nil {
		return false, fmt.Errorf("save manifest %q: %w", s.path, err)
	}
	return changed, nil
}

func marshalManifest(value domain.Manifest) ([]byte, error) {
	raw, err := json.MarshalIndent(manifestToWire(value), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func manifestFromBytes(raw []byte) (domain.Manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var wire manifestWire
	if err := dec.Decode(&wire); err != nil {
		return domain.Manifest{}, err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return domain.Manifest{}, errors.New("manifest json contains trailing data")
		}
		return domain.Manifest{}, err
	}
	return wireToManifest(wire)
}
