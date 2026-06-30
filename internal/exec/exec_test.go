package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskSecrets(t *testing.T) {
	secrets := map[string]string{
		"API_KEY": "sk-secret-123",
		"DB_PASS": "hunter2",
	}

	output := "Response from sk-secret-123 with password hunter2 done"
	masked := MaskSecrets(output, secrets)

	assert.NotContains(t, masked, "sk-secret-123")
	assert.NotContains(t, masked, "hunter2")
	assert.Contains(t, masked, "***")
	assert.Contains(t, masked, "done")
}

func TestMaskSecrets_NoSecrets(t *testing.T) {
	output := "Hello world"
	masked := MaskSecrets(output, map[string]string{})
	assert.Equal(t, "Hello world", masked)
}

func TestMaskSecrets_PartialMatch(t *testing.T) {
	secrets := map[string]string{
		"TOKEN": "abc123xyz",
	}
	output := "prefix-abc123xyz-suffix"
	masked := MaskSecrets(output, secrets)
	assert.NotContains(t, masked, "abc123xyz")
	assert.Contains(t, masked, "prefix-")
	assert.Contains(t, masked, "-suffix")
}
