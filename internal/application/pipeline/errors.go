package pipeline

import "errors"

var (
	ErrSourceNil                     = errors.New("source must not be nil")
	ErrEncoderNil                    = errors.New("encoder must not be nil")
	ErrManifestStoreNil              = errors.New("manifest store must not be nil")
	ErrVaultProjectorNil             = errors.New("vault projector must not be nil")
	ErrOrchestratorNil               = errors.New("orchestrator must not be nil")
	ErrContextNil                    = errors.New("context must not be nil")
	ErrSourceChatEmpty               = errors.New("source chat must not be empty")
	ErrThresholdOutOfRange           = errors.New("must be greater than 0 and at most 1")
	ErrDuplicateNoteID               = errors.New("duplicate note id")
	ErrEmbeddingEmpty                = errors.New("embedding must not be empty")
	ErrEmbeddingDimensionMismatch    = errors.New("embedding dimension mismatch")
	ErrIncrementalMetadataMismatch   = errors.New("incremental run metadata mismatch with existing Manifest")
	ErrEncoderModelIDEmpty           = errors.New("encoder metadata model_id must not be empty")
	ErrEncoderModelHashEmpty         = errors.New("encoder metadata model_hash must not be empty")
	ErrEncoderVectorDimensionInvalid = errors.New(
		"encoder metadata vector_dimension must be greater than 0",
	)
	ErrUnsupportedNormalizationRule = errors.New(
		"encoder metadata normalization_rule: unsupported value",
	)
)
