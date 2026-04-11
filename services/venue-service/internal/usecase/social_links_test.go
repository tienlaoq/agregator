package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSocialLinksJSON_Empty(t *testing.T) {
	s, err := NormalizeSocialLinksJSON("")
	require.NoError(t, err)
	assert.Equal(t, "{}", s)
}

func TestNormalizeSocialLinksJSON_Object(t *testing.T) {
	s, err := NormalizeSocialLinksJSON(`{"vk":"https://vk.com/x"}`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"vk":"https://vk.com/x"}`, s)
}

func TestNormalizeSocialLinksJSON_NotObject(t *testing.T) {
	_, err := NormalizeSocialLinksJSON(`["a"]`)
	require.Error(t, err)
}
