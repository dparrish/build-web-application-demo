// Package encryption manages envelope encryption for blobs stored on Cloud Storage, where the data is encrypted using
// envelope encryption.
// The data is encrypted with a Data Encryption Key (supplied), which is itself encrypted/decrypted by Google Key
// Management Service (KMS).
package encryption

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log"
	"path"

	"github.com/dparrish/build-web-application-demo/autoconfig"

	"golang.org/x/oauth2/google"
	cloudkms "google.golang.org/api/cloudkms/v1"

	"github.com/gtank/cryptopasta"
)

type Key *[32]byte

type Envelope struct {
	config *autoconfig.Config
	svc    *cloudkms.Service // Google KMS Service client.
}

func New(ctx context.Context, config *autoconfig.Config) *Envelope {
	client, err := google.DefaultClient(ctx, cloudkms.CloudPlatformScope)
	if err != nil {
		log.Fatal(err)
	}

	kmsService, err := cloudkms.New(client)
	if err != nil {
		log.Fatal(err)
	}

	return &Envelope{
		config: config,
		svc:    kmsService,
	}
}

// Encrypt encrypts data using enveloe encryption.
func (e *Envelope) Encrypt(key Key, reader io.Reader, writer io.Writer) error {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	var iv [aes.BlockSize]byte
	if _, err := io.ReadFull(rand.Reader, iv[:]); err != nil {
		return err
	}
	// Write the IV as the first [aes.BlockSize] bytes of the stream.
	writer.Write(iv[:])
	stream := cipher.NewOFB(block, iv[:])
	out := &cipher.StreamWriter{S: stream, W: writer}
	if _, err := io.Copy(out, reader); err != nil {
		log.Printf("Error writing encrypted data: %v", err)
		return err
	}
	return nil
}

// Decrypt decrypts data using enveloe encryption.
func (e *Envelope) Decrypt(key Key, reader io.Reader, writer io.Writer) error {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	var iv [aes.BlockSize]byte
	reader.Read(iv[:])
	stream := cipher.NewOFB(block, iv[:])
	out := &cipher.StreamWriter{S: stream, W: writer}
	if _, err := io.Copy(out, reader); err != nil {
		return err
	}
	return nil
}

func (e *Envelope) NewKey() Key {
	return cryptopasta.NewEncryptionKey()
}

func (e *Envelope) kmsKey() string {
	return path.Join("projects", e.config.Get("project"), "locations", e.config.Get("encryption.location"), "keyRings",
		e.config.Get("encryption.keyring"), "cryptoKeys", e.config.Get("encryption.key"))
}

func (e *Envelope) DecryptKey(ctx context.Context, key string) (Key, error) {
	req := &cloudkms.DecryptRequest{Ciphertext: key}
	resp, err := e.svc.Projects.Locations.KeyRings.CryptoKeys.Decrypt(e.kmsKey(), req).Do()
	if err != nil {
		return nil, err
	}
	var ek [32]byte
	k, err := base64.StdEncoding.DecodeString(resp.Plaintext)
	copy(ek[:], k)
	return &ek, nil
}

func (e *Envelope) EncryptKey(ctx context.Context, key Key) (string, error) {
	req := &cloudkms.EncryptRequest{Plaintext: base64.StdEncoding.EncodeToString(key[:])}
	resp, err := e.svc.Projects.Locations.KeyRings.CryptoKeys.Encrypt(e.kmsKey(), req).Do()
	if err != nil {
		return "", err
	}
	return resp.Ciphertext, nil
}
