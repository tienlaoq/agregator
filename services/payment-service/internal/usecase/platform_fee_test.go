package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlatformFeeKopecks(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int64(1500), PlatformFeeKopecks(10000, 1500))
	assert.Equal(t, int64(4), PlatformFeeKopecks(33, 1500)) // 33*1500/10000 = 4 (integer division)
	assert.Equal(t, int64(8500), CounterpartyNetKopecks(10000, 1500))
}
