package embedding

import (
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

func formatInputOutputInfos(values []ort.InputOutputInfo) string {
	if len(values) == 0 {
		return ""
	}

	descriptions := make([]string, 0, len(values))
	for _, value := range values {
		descriptions = append(descriptions, value.String())
	}

	return strings.Join(descriptions, "; ")
}
