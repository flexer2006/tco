package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/flexer2006/tco/internal/domain"
	"github.com/flexer2006/tco/internal/ports"
)

type (
	store struct {
		atomic manifestAtomicWriter
		path   string
	}
)

func NewStore(path string) (ports.ManifestStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrManifestPathEmpty
	}

	return new(store{path: path, atomic: newManifestAtomicWriter()}), nil
}

func (s *store) Load() (domain.Manifest, error) {
	if s == nil {
		return domain.Manifest{}, ErrStoreNil
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
		return false, ErrStoreNil
	}

	err := domain.Validate(value)
	if err != nil {
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
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	return append(raw, '\n'), nil
}

func manifestFromBytes(raw []byte) (domain.Manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var wire manifestWire

	err := dec.Decode(&wire)
	if err != nil {
		return domain.Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}

	var extra any

	err = dec.Decode(&extra)
	if !errors.Is(err, io.EOF) {
		if err == nil {
			return domain.Manifest{}, ErrManifestTrailingData
		}

		return domain.Manifest{}, fmt.Errorf("decode manifest trailing data: %w", err)
	}

	return wireToManifest(wire)
}
