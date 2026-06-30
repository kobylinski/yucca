package fuzzy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalize(t *testing.T) {
	assert.Equal(t, "caddypay", Normalize("Caddy-Pay"))
	assert.Equal(t, "caddypay", Normalize("caddy pay"))
	assert.Equal(t, "apikey", Normalize("API_KEY"))
	assert.Equal(t, "", Normalize("  -- "))
}

func TestMatch(t *testing.T) {
	assert.True(t, Match("caddy pay", "caddy-pay"), "spaces vs dashes")
	assert.True(t, Match("", "anything"), "empty query matches")
	assert.True(t, Match("stripe", "my-stripe-key", "other"), "matches one of many")
	assert.False(t, Match("aws", "caddy-pay"))
}
