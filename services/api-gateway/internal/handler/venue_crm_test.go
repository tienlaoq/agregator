package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUniqueNonEmptyUserIDs(t *testing.T) {
	got := uniqueNonEmptyUserIDs([]string{
		"  a  ",
		"b",
		"",
		"a",
		"b",
		"  ",
	})
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestUniqueNonEmptyUserIDs_empty(t *testing.T) {
	assert.Empty(t, uniqueNonEmptyUserIDs(nil))
	assert.Empty(t, uniqueNonEmptyUserIDs([]string{"", "  "}))
}
