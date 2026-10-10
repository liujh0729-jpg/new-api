package doubao

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedance25VideoInputUsesOfficialDiscount(t *testing.T) {
	ratio, ok := GetVideoInputRatio("doubao-seedance-2-5-260628")
	require.True(t, ok)
	require.InDelta(t, 42.0/70.0, ratio, 1e-12)
}
