package encryption

import (
	"bytes"
	"strings"
	"testing"
)

var testPlain = `zygote zygote's zygotes Ångström éclair éclair's éclairs éclat
éclat's élan élan's émigré émigré's émigrés épée épée's épées étude étude's
études`

func TestEncryptAndDecrypt(t *testing.T) {
	// Generate a key.
	e := Envelope{}

	// Encrypt.
	key := e.NewKey()
	var ciphertext bytes.Buffer
	if err := e.EncryptStream(key, strings.NewReader(testPlain), &ciphertext); err != nil {
		t.Error(err)
	}

	// Decrypt.
	var plain bytes.Buffer
	if err := e.DecryptStream(key, &ciphertext, &plain); err != nil {
		t.Error(err)
	}
	if plain.String() != testPlain {
		t.Errorf("Decrypted does not match input")
	}
}
