package domain

import (
	"fmt"
	"math"
	"slices"
)

const NormalizationRuleL2Unit = "l2_unit"

type Vector struct {
	values []float32
}

func NewVector(values []float32) (Vector, error) {
	cloned, sumSquares, err := cloneAndValidateComponents(values)
	if err != nil {
		return Vector{}, err
	}

	norm := math.Sqrt(sumSquares)
	for i := range cloned {
		cloned[i] = float32(float64(cloned[i]) / norm)
	}

	return Vector{values: cloned}, nil
}

func NewVectorAlreadyNormalized(values []float32) (Vector, error) {
	cloned, _, err := cloneAndValidateComponents(values)
	if err != nil {
		return Vector{}, err
	}

	return Vector{values: cloned}, nil
}

func cloneAndValidateComponents(values []float32) ([]float32, float64, error) {
	if len(values) == 0 {
		return nil, 0, ErrVectorEmpty
	}

	cloned := slices.Clone(values)

	var sumSquares float64

	for i, value := range cloned {
		component := float64(value)
		if math.IsNaN(component) {
			return nil, 0, fmt.Errorf("vector[%d]: %w", i, ErrVectorComponentNaN)
		}

		if math.IsInf(component, 0) {
			return nil, 0, fmt.Errorf("vector[%d]: %w", i, ErrVectorComponentInf)
		}

		sumSquares += component * component
	}

	if sumSquares == 0 {
		return nil, 0, ErrVectorNormZero
	}

	return cloned, sumSquares, nil
}

func (v Vector) DotUnsafe(other Vector) float64 {
	return DotFloat32(v.values, other.values)
}

func (v Vector) AccumulateUnsafe(sum []float64) {
	n := min(len(v.values), len(sum))
	for i := range n {
		sum[i] += float64(v.values[i])
	}
}

func (v Vector) Values() []float32 { return slices.Clone(v.values) }

func (v Vector) ValuesUnsafe() []float32 { return v.values }
func (v Vector) Dimension() int          { return len(v.values) }

func IsSupportedNormalizationRule(rule string) bool {
	return rule == NormalizationRuleL2Unit
}

func DotFloat32(left, right []float32) float64 {
	n := min(len(left), len(right))

	var (
		dot0, dot1, dot2, dot3 float64
		i                      int
	)
	for ; i+3 < n; i += 4 {
		dot0 += float64(left[i]) * float64(right[i])
		dot1 += float64(left[i+1]) * float64(right[i+1])
		dot2 += float64(left[i+2]) * float64(right[i+2])
		dot3 += float64(left[i+3]) * float64(right[i+3])
	}

	dot := dot0 + dot1 + dot2 + dot3
	for ; i < n; i++ {
		dot += float64(left[i]) * float64(right[i])
	}

	return dot
}
