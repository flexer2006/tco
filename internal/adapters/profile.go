package adapters

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ProfileBertTokenizedMeanPooling ModelProfile = "bert_tokenized_mean_pooling"
	ProfileStringInputDirect        ModelProfile = "string_input_direct"
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
		return "", fmt.Errorf("model_profile: unsupported value %q (allowed: %s, %s)",
			trimmed, ProfileBertTokenizedMeanPooling, ProfileStringInputDirect)
	case ProfileBertTokenizedMeanPooling:
		return ProfileBertTokenizedMeanPooling, nil
	case ProfileStringInputDirect:
		return ProfileStringInputDirect, nil
	case "":
		return "", errors.New("model_profile: must not be empty")
	}
}

func ConfigFor(profile ModelProfile, vectorDimension int) (ProfileConfig, error) {
	dim := vectorDimension
	if dim <= 0 {
		dim = 384
	}
	switch profile {
	case ProfileBertTokenizedMeanPooling:
		return ProfileConfig{
			Profile:           profile,
			VectorDimension:   dim,
			RequiresTokenizer: true,
			DefaultVectorDim:  384,
		}, nil
	case ProfileStringInputDirect:
		return ProfileConfig{
			Profile:           profile,
			VectorDimension:   dim,
			RequiresTokenizer: false,
			DefaultVectorDim:  384,
		}, nil
	default:
		return ProfileConfig{}, fmt.Errorf("model_profile: unsupported value %q", profile)
	}
}
