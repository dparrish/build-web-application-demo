package metadata

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/dparrish/build-web-application-demo/encryption"

	"cloud.google.com/go/spanner"
	lru "github.com/hashicorp/golang-lru"
	"google.golang.org/api/iterator"
)

type Row struct {
	ID       string    `json:"id,omitempty" spanner:"Id"`
	UserID   string    `json:"-" spanner:"UserId"`
	Name     string    `json:"name,omitempty" spanner:"Name"`
	Uploaded time.Time `json:"uploaded,omitempty" spanner:"Uploaded"`
	MimeType string    `json:"mime_type,omitempty" spanner:"MimeType"`
	Size     int64     `json:"size,omitempty" spanner:"Size"`
}

type User struct {
	ID            string `spanner:"Id"`
	EncryptionKey string `spanner:"EncryptionKey"`
}

// keyCache contains an in-memory cache of userid -> encryption key.
var keyCache *lru.Cache

func init() {
	var err error
	keyCache, err = lru.New(128)
	if err != nil {
		log.Fatal(err)
	}
}

func GetEncryptionKey(ctx context.Context, client *spanner.Client, envelope *encryption.Envelope, userid string) (encryption.Key, error) {
	// Check the cache first.
	if key, ok := keyCache.Get(userid); ok {
		return key.(encryption.Key), nil
	}

	stmt := spanner.NewStatement(`SELECT EncryptionKey FROM Users WHERE Id = @userid`)
	stmt.Params["userid"] = userid

	// Set a 10 second timeout for the metadata query.
	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	iter := client.Single().Query(rctx, stmt)
	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error fetching users row: %v", err)
		}

		var encodedKey string
		if err := row.Columns(&encodedKey); err != nil {
			return nil, fmt.Errorf("error fetching users row: %v", err)
		}

		// Decrypt the Data Encryption Key using the Key Encryption Key.
		log.Printf("Decrypting encryption key for user %q", userid)
		rctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		ek, err := envelope.DecryptKey(rctx, encodedKey)
		if err != nil {
			return nil, fmt.Errorf("error decrypting encryption key: %v", err)
		}

		// Store the retrieved key in the in-memory cache.
		keyCache.Add(userid, ek)

		return ek, nil
	}

	// No data encryption key exists for this user, create a new one.
	ek := envelope.NewKey()
	encodedKey, err := envelope.EncryptKey(ctx, ek)
	if err != nil {
		return nil, fmt.Errorf("error creating encryption key: %v", err)
	}
	if err := setEncryptionKey(ctx, client, userid, encodedKey); err != nil {
		return nil, err
	}
	// Store the retrieved key in the in-memory cache.
	keyCache.Add(userid, ek)

	return ek, nil
}

func setEncryptionKey(ctx context.Context, client *spanner.Client, userid string, key string) error {
	row := &User{
		ID:            userid,
		EncryptionKey: key,
	}
	mut, err := spanner.InsertStruct("Users", row)
	if err != nil {
		return fmt.Errorf("error creating insert mutation: %v", err)
	}
	if _, err := client.Apply(ctx, []*spanner.Mutation{mut}); err != nil {
		return fmt.Errorf("error inserting users row: %v", err)
	}
	return nil
}

func ListForUser(ctx context.Context, client *spanner.Client, userid string) ([]Row, error) {
	response := []Row{}

	stmt := spanner.NewStatement(`SELECT Id, UserID, Name, Uploaded, MimeType, Size FROM Metadata WHERE UserId = @userid`)
	stmt.Params["userid"] = userid

	// Set a 10 second timeout for the metadata query.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	iter := client.Single().Query(ctx, stmt)
	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error fetching metadata: %v", err)
		}

		var mr Row
		if err := row.Columns(&mr.ID, &mr.UserID, &mr.Name, &mr.Uploaded, &mr.MimeType, &mr.Size); err != nil {
			return nil, fmt.Errorf("error fetching metadata row: %v", err)
		}
		response = append(response, mr)
	}

	return response, nil
}

func Add(ctx context.Context, client *spanner.Client, row *Row) error {
	mut, err := spanner.InsertStruct("Metadata", row)
	if err != nil {
		return fmt.Errorf("error creating insert mutation: %v", err)
	}
	if _, err := client.Apply(ctx, []*spanner.Mutation{mut}); err != nil {
		return fmt.Errorf("error inserting metadata row: %v", err)
	}
	return nil
}

func Get(ctx context.Context, client *spanner.Client, userid string, objectID string) (*Row, error) {
	stmt := spanner.NewStatement(`SELECT Id, UserID, Name, Uploaded, MimeType, Size FROM Metadata WHERE UserId = @userid AND Id = @id`)
	stmt.Params["userid"] = userid
	stmt.Params["id"] = objectID

	// Set a 10 second timeout for the metadata query.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	iter := client.Single().Query(ctx, stmt)
	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error fetching metadata: %v", err)
		}

		var mr Row
		if err := row.Columns(&mr.ID, &mr.UserID, &mr.Name, &mr.Uploaded, &mr.MimeType, &mr.Size); err != nil {
			return nil, fmt.Errorf("error fetching metadata row: %v", err)
		}
		return &mr, nil
	}

	return nil, errors.New("no rows found")
}

func Delete(ctx context.Context, client *spanner.Client, objectID string) error {
	mut := spanner.Delete("Metadata", spanner.Key{objectID})
	if _, err := client.Apply(ctx, []*spanner.Mutation{mut}); err != nil {
		return fmt.Errorf("error deleting metadata row: %v", err)
	}
	return nil
}
