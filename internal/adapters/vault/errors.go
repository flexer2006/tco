package vault

import "errors"

var (
	ErrStemWhitespace       = errors.New("must not contain leading or trailing whitespace")
	ErrStemEmpty            = errors.New("must not be empty")
	ErrStemPathSegment      = errors.New("must not be a path segment")
	ErrStemPathSeparators   = errors.New("must not contain path separators")
	ErrStemControlChars     = errors.New("must not contain control characters")
	ErrVaultRootEmpty       = errors.New("vault root must not be empty")
	ErrProjectorNil         = errors.New("vaultfs projector must not be nil")
	ErrManifestPathEmpty    = errors.New("manifest path must not be empty")
	ErrStoreNil             = errors.New("jsonmanifest store must not be nil")
	ErrManifestTrailingData = errors.New("manifest json contains trailing data")
	ErrUnsupportedSchema    = errors.New("schema_version: unsupported value")
	ErrHashMismatch         = errors.New("hash does not match rendered body")
)
