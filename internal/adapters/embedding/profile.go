package embedding

import (
	"fmt"
	"strings"
)

const (
	ProfileBertTokenizedMeanPooling ModelProfile = "bert_tokenized_mean_pooling" // #nosec G101
	ProfileStringInputDirect        ModelProfile = "string_input_direct"

	defaultVectorDimension = 384
)

type (
	ModelProfile  string
	ProfileConfig struct {
		Profile                           ModelProfile
		VectorDimension, DefaultVectorDim int
		RequiresTokenizer                 bool
	}
)

func (p ModelProfile) String() string { return string(p) }

func Parse(raw string) (ModelProfile, error) {
	trimmed := strings.TrimSpace(raw)
	switch ModelProfile(trimmed) {
	default:
		return "", fmt.Errorf("%w %q (allowed: %s, %s)",
			ErrUnsupportedModelProfile, trimmed, ProfileBertTokenizedMeanPooling, ProfileStringInputDirect)
	case ProfileBertTokenizedMeanPooling:
		return ProfileBertTokenizedMeanPooling, nil
	case ProfileStringInputDirect:
		return ProfileStringInputDirect, nil
	case "":
		return "", ErrModelProfileEmpty
	}
}

func ConfigFor(profile ModelProfile, vectorDimension int) (ProfileConfig, error) {
	dim := vectorDimension
	if dim <= 0 {
		dim = defaultVectorDimension
	}

	switch profile {
	case ProfileBertTokenizedMeanPooling:
		return ProfileConfig{
			Profile:           profile,
			VectorDimension:   dim,
			RequiresTokenizer: true,
			DefaultVectorDim:  defaultVectorDimension,
		}, nil
	case ProfileStringInputDirect:
		return ProfileConfig{
			Profile:           profile,
			VectorDimension:   dim,
			RequiresTokenizer: false,
			DefaultVectorDim:  defaultVectorDimension,
		}, nil
	default:
		return ProfileConfig{}, fmt.Errorf("%w %q", ErrUnsupportedModelProfile, profile)
	}
}
