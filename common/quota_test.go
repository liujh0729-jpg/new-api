package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuotaToCNY(t *testing.T) {
	assert.Equal(t, 0.073, QuotaToCNY(5000, 500000, 7.3))
	assert.Equal(t, -0.073, QuotaToCNY(-5000, 500000, 7.3))
	assert.Zero(t, QuotaToCNY(5000, 0, 7.3))
	assert.Zero(t, QuotaToCNY(5000, 500000, 0))
}
