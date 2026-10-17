package domain_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// The cosine is the one place two vectors are compared, so every boundary that
// would otherwise become a silent zero is pinned here.

func TestCosine_IdenticalVectorsAreOne(t *testing.T) {
	similarity, err := domain.Cosine([]float32{0.1, -0.2, 0.3}, []float32{0.1, -0.2, 0.3})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, similarity, 1e-6)
}

func TestCosine_IsScaleInvariant(t *testing.T) {
	// A model change or a normalisation step must not change which chunk is the
	// nearest: only the angle matters.
	similarity, err := domain.Cosine([]float32{1, 2, 3}, []float32{2, 4, 6})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, similarity, 1e-6)

	opposite, err := domain.Cosine([]float32{1, 2, 3}, []float32{-2, -4, -6})
	require.NoError(t, err)
	assert.InDelta(t, -1.0, opposite, 1e-6)
}

func TestCosine_OrthogonalVectorsAreZero(t *testing.T) {
	similarity, err := domain.Cosine([]float32{1, 0}, []float32{0, 1})
	require.NoError(t, err)
	assert.InDelta(t, 0.0, similarity, 1e-6)
}

func TestCosine_IsSymmetric(t *testing.T) {
	a := []float32{0.2, 0.5, -0.1, 0.7}
	b := []float32{-0.3, 0.4, 0.9, 0.1}

	ab, err := domain.Cosine(a, b)
	require.NoError(t, err)
	ba, err := domain.Cosine(b, a)
	require.NoError(t, err)
	assert.InDelta(t, ab, ba, 1e-9)
}

func TestCosine_RejectsVectorsOfDifferentLengths(t *testing.T) {
	_, err := domain.Cosine([]float32{1, 2, 3}, []float32{1, 2})
	require.Error(t, err, "a 1024-dimension vector must never be compared with a 3072-dimension one")
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)
}

func TestCosine_RejectsEmptyVectors(t *testing.T) {
	for name, pair := range map[string][2][]float32{
		"both empty":     {nil, nil},
		"first empty":    {{}, {1, 2}},
		"second empty":   {{1, 2}, {}},
		"empty vs empty": {[]float32{}, []float32{}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := domain.Cosine(pair[0], pair[1])
			require.Error(t, err, "cosine is undefined for an empty vector")
			assert.ErrorIs(t, err, apperror.ErrInvalidInput)
		})
	}
}

// A zero vector has no direction, so it is similar to nothing. That is a defined
// answer, not a failure: refusing would turn one degenerate stored vector into a
// search that cannot run at all.
func TestCosine_AZeroVectorIsNoSimilarity(t *testing.T) {
	similarity, err := domain.Cosine([]float32{0, 0, 0}, []float32{1, 2, 3})
	require.NoError(t, err)
	assert.InDelta(t, 0.0, similarity, 1e-9, "a zero vector must not divide by zero")

	similarity, err = domain.Cosine([]float32{1, 2, 3}, []float32{0, 0, 0})
	require.NoError(t, err)
	assert.InDelta(t, 0.0, similarity, 1e-9)
}

func TestCosine_HandlesTheMagnitudeOfRealVectors(t *testing.T) {
	// 3072 dimensions of order one: the dot product and the norms are large
	// enough that a float32 accumulator would lose the small terms, which is why
	// the sum is accumulated in float64.
	a := make([]float32, 3072)
	b := make([]float32, 3072)
	for i := range a {
		a[i] = float32(math.Sin(float64(i)))
		b[i] = float32(math.Cos(float64(i)))
	}

	similarity, err := domain.Cosine(a, b)
	require.NoError(t, err)
	assert.False(t, math.IsNaN(similarity), "a real embedding must not produce NaN")
	assert.GreaterOrEqual(t, similarity, -1.0)
	assert.LessOrEqual(t, similarity, 1.0)
}
