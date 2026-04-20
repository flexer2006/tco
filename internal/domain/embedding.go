package domain

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

const NormalizationRuleL2Unit = "l2_unit"

type Vector struct {
	values []float32
}

func (v Vector) DotUnsafe(other Vector) float64 {
	return dotValues(v.values, other.values)
}

func (v Vector) AccumulateUnsafe(sum []float64) {
	n := min(len(v.values), len(sum))
	for i := range n {
		sum[i] += float64(v.values[i])
	}
}

func NewVector(values []float32) (Vector, error) {
	if len(values) == 0 {
		return Vector{}, errors.New("vector must not be empty")
	}
	cloned := slices.Clone(values)
	var sumSquares float64
	for i, value := range cloned {
		component := float64(value)
		if math.IsNaN(component) {
			return Vector{}, fmt.Errorf("vector[%d]: must not be NaN", i)
		}
		if math.IsInf(component, 0) {
			return Vector{}, fmt.Errorf("vector[%d]: must not be infinite", i)
		}
		sumSquares += component * component
	}
	if sumSquares == 0 {
		return Vector{}, errors.New("vector norm must not be zero")
	}
	norm := math.Sqrt(sumSquares)
	for i := range cloned {
		cloned[i] = float32(float64(cloned[i]) / norm)
	}
	return Vector{values: cloned}, nil
}

func (v Vector) Values() []float32 { return slices.Clone(v.values) }
func (v Vector) Dimension() int    { return len(v.values) }

func IsSupportedNormalizationRule(rule string) bool {
	return rule == NormalizationRuleL2Unit
}

func dotValues(left, right []float32) float64 {
	var dot float64
	for i := range left {
		dot += float64(left[i]) * float64(right[i])
	}
	return dot
}
