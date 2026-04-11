package cities

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJoinSplitRoundTrip(t *testing.T) {
	in := []string{"Пенза", "Москва"}
	j := Join(in)
	assert.Contains(t, j, Sep)
	out := Split(j)
	assert.Equal(t, []string{"Пенза", "Москва"}, out)
}

func TestSplitSingle(t *testing.T) {
	assert.Equal(t, []string{"Казань"}, Split("Казань"))
}

func TestJoinDedupe(t *testing.T) {
	j := Join([]string{"Пенза", "пенза", " Москва "})
	out := Split(j)
	assert.Len(t, out, 2)
}
