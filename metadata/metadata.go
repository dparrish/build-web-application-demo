package metadata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
)

type Row struct {
	ID            string    `json:"id,omitempty" spanner:"Id"`
	UserID        string    `json:"-" spanner:"UserId"`
	Name          string    `json:"name,omitempty" spanner:"Name"`
	Uploaded      time.Time `json:"uploaded,omitempty" spanner:"Uploaded"`
	MimeType      string    `json:"mime_type,omitempty" spanner:"MimeType"`
	Size          int64     `json:"size,omitempty" spanner:"Size"`
	EncryptionKey string    `json:"-" spanner:"EncryptionKey"`
}

func ListForUser(ctx context.Context, client *spanner.Client, userID string) ([]Row, error) {
	response := []Row{}

	stmt := spanner.NewStatement(`SELECT Id, UserID, Name, Uploaded, MimeType, Size FROM Metadata WHERE UserId = @userid`)
	stmt.Params["userid"] = userID

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

func Get(ctx context.Context, client *spanner.Client, userID string, objectID string) (*Row, error) {
	stmt := spanner.NewStatement(`SELECT Id, UserID, Name, Uploaded, MimeType, Size, EncryptionKey FROM Metadata WHERE UserId = @userid AND Id = @id`)
	stmt.Params["userid"] = userID
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
		if err := row.Columns(&mr.ID, &mr.UserID, &mr.Name, &mr.Uploaded, &mr.MimeType, &mr.Size, &mr.EncryptionKey); err != nil {
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
