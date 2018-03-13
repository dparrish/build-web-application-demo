package autoconfig

import (
	"context"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

const validConfig = `
{
	"var1": "value1",
	"hash1": {
	  "hash1var1": "blah",
		"hash2": {
			"hash2var1": ["foo", "bar"]
		}
	}
}
`

func loadTestConfig() *Config {
	Fs = afero.NewMemMapFs()
	afero.WriteFile(Fs, "test.config", []byte(validConfig), 0644)
	c, _ := Load(context.Background(), "test.config")
	return c
}

func TestInvalidConfig(t *testing.T) {
	Fs = afero.NewMemMapFs()
	afero.WriteFile(Fs, "test.config", []byte("blahblah"), 0644)
	c, err := Load(context.Background(), "test.config")
	assert.Nil(t, c)
	assert.NotNil(t, err)
}

func TestLoad(t *testing.T) {
	Fs = afero.NewMemMapFs()
	afero.WriteFile(Fs, "test.config", []byte(validConfig), 0644)
	c, err := Load(context.Background(), "test.config")
	assert.Nil(t, err)
	assert.NotNil(t, c)
}

func TestGet(t *testing.T) {
	c := loadTestConfig()
	assert.Equal(t, c.Get("var1"), "value1")
	assert.Equal(t, c.Get("hash1.hash1var1"), "blah")
}

func TestGetAll(t *testing.T) {
	c := loadTestConfig()
	assert.Equal(t, c.GetAll("hash1.hash2.hash2var1"), []string{"foo", "bar"})
}

func TestValidator(t *testing.T) {
}
