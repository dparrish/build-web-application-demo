package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dparrish/build-web-application-demo/authentication"
	"github.com/dparrish/build-web-application-demo/autoconfig"
	"github.com/dparrish/build-web-application-demo/encryption"
	"github.com/dparrish/build-web-application-demo/logging"
	"github.com/dparrish/build-web-application-demo/metadata"
	"github.com/dparrish/build-web-application-demo/middleware"
	"github.com/dparrish/build-web-application-demo/swagger"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/storage"

	"go.opencensus.io/exporter/stackdriver"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	health "github.com/docker/go-healthcheck"
	"github.com/google/uuid"
	gcontext "github.com/gorilla/context"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

var (
	configFile  = flag.String("config", "", "Configuration file location")
	warmSpanner = flag.Bool("warm_spanner", false, "Send a warming query to spanner on startup")
)

type DocumentService struct {
	// Cloud API Clients
	config   *autoconfig.Config
	spanner  *spanner.Client
	storage  *storage.Client
	bigquery *bigquery.Client

	metrics struct {
		requests      *stats.Int64Measure
		documentCount *stats.Int64Measure
		retrieveAge   *stats.Float64Measure
	}
	exporter *stackdriver.Exporter

	Handler    *mux.Router // The http.Handler responsible for all the requests.
	encryption *encryption.Envelope
}

var (
	methodKey tag.Key
)

// NewDocumentService creates a new DocumentService object.
// It is responsible for creating connections to all the required backends, and setting up the HTTP routing.
func NewDocumentService(config *autoconfig.Config, ctx context.Context) (*DocumentService, error) {
	s := &DocumentService{
		config:  config,
		Handler: mux.NewRouter(),
	}
	s.createClients(ctx)

	// These is the un-authenticated endpoint that handles authentication with Auth0.
	s.Handler.HandleFunc("/debug/health", health.StatusHandler).Methods("GET")
	s.Handler.Handle("/login", middleware.JSON(authentication.Handler(config))).Methods("POST")

	// These requests all require authentication.
	logMiddleware := logging.LogMiddleware{
		Table: s.bigquery.Dataset(config.Get("bigquery.dataset")).Table(config.Get("bigquery.log_table")),
	}
	authRouter := s.Handler.PathPrefix("/document").Subrouter()
	authRouter.Handle("/", middleware.JSON(authentication.Middleware(config, s.ListDocuments))).Methods("GET")
	authRouter.Handle("/", middleware.JSON(authentication.Middleware(config, s.UploadDocument))).Methods("POST")
	authRouter.Handle("/{id}", authentication.Middleware(config, s.GetDocument)).Methods("GET")
	authRouter.Handle("/{id}", middleware.JSON(authentication.Middleware(config, s.DeleteDocument))).Methods("DELETE")
	authRouter.Use(logMiddleware.Middleware)
	authRouter.Use(handlers.CompressHandler)
	return s, nil
}

func (s *DocumentService) ListDocuments(w http.ResponseWriter, r *http.Request) {
	ctx, _ := tag.New(r.Context(), tag.Insert(methodKey, "list"))
	stats.Record(ctx, s.metrics.requests.M(1))

	userid := gcontext.Get(r, "userid").(string)

	rows, err := metadata.ListForUser(r.Context(), s.spanner, userid)
	if err != nil {
		log.Print(err)
		swagger.Errorf(w, http.StatusInternalServerError, err.Error())
		return
	}
	stats.Record(ctx, s.metrics.documentCount.M(int64(len(rows))))
	json.NewEncoder(w).Encode(rows)
}

func (s *DocumentService) GetDocument(w http.ResponseWriter, r *http.Request) {
	ctx, _ := tag.New(r.Context(), tag.Insert(methodKey, "get"))
	stats.Record(ctx, s.metrics.requests.M(1))

	userid := gcontext.Get(r, "userid").(string)
	vars := mux.Vars(r)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	mr, err := metadata.Get(ctx, s.spanner, userid, vars["id"])
	if err != nil {
		swagger.Errorf(w, http.StatusNotFound, "Invalid object ID")
		return
	}

	bucket := s.storage.Bucket(s.config.Get("storage.bucket"))
	obj := bucket.Object(mr.ID)
	reader, err := obj.NewReader(r.Context())
	if err != nil {
		log.Print(err)
		swagger.Errorf(w, http.StatusInternalServerError, "Error reading blob")
		return
	}
	defer reader.Close()

	// Decrypt the Data Encryption Key using the Key Encryption Key (KMS).
	ctx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ek, err := metadata.GetEncryptionKey(ctx, s.spanner, s.encryption, userid)
	if err != nil {
		log.Print(err)
		swagger.Errorf(w, http.StatusInternalServerError, "Error getting encryption key")
		return
	}

	// Send the file to the client.
	w.Header().Set("Content-Type", mr.MimeType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", mr.Size))
	w.WriteHeader(http.StatusOK)

	// Record the document age in the retrieval age distribution.
	stats.Record(ctx, s.metrics.retrieveAge.M(time.Since(mr.Uploaded).Seconds()))
	log.Printf("Recoring retrievel age of %s (%f seconds)", time.Since(mr.Uploaded), time.Since(mr.Uploaded).Seconds())

	// Create an io.MultWriter to split off data as it streams. This is not necessary in this case but this can be used to
	// do other operations on the streaming data in parallel with the decryption, such as hash verification.
	mw := io.MultiWriter(w)

	// Decrypt the Data using the Data Encryption Key. This uses streaming decryption so the whole body doesn't have to be
	// read into RAM first.
	if err := s.encryption.Decrypt(ek, reader, mw); err != nil {
		log.Printf("Error reading body: %v", err)
		swagger.Errorf(w, http.StatusInternalServerError, "Error reading body")
		return
	}
}

func (s *DocumentService) UploadDocument(w http.ResponseWriter, r *http.Request) {
	ctx, _ := tag.New(r.Context(), tag.Insert(methodKey, "upload"))
	stats.Record(ctx, s.metrics.requests.M(1))

	userid := gcontext.Get(r, "userid").(string)

	// Create the bucket handle before parsing the request, just in case it doesn't work.
	filename, err := uuid.NewRandom()
	if err != nil {
		log.Printf("Error creating UUID: %v", err)
		swagger.Errorf(w, http.StatusInternalServerError, "Error writing to backend storage")
		return
	}
	bucket := s.storage.Bucket(s.config.Get("storage.bucket"))
	obj := bucket.Object(filename.String())

	// JSON decode the request.
	// This is very inefficient because the entire body will be kept in RAM.
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		swagger.Errorf(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if req["name"] == "" || req["body"] == "" {
		swagger.Errorf(w, http.StatusBadRequest, "Invalid request, missing field")
		return
	}

	// Get the data encryption key for the user. One will be created if none exist.
	ek, err := metadata.GetEncryptionKey(ctx, s.spanner, s.encryption, userid)
	if err != nil {
		log.Print(err)
		swagger.Errorf(w, http.StatusInternalServerError, "Error getting encryption key")
		return
	}

	// The following operations are all streaming so that each step doesn't have to take up RAM proportional to the size
	// of the body.

	// Create a blob writer.
	blobWriter := obj.NewWriter(r.Context())
	if req["mime_type"] != "" {
		// Set the MIME type to whatever the caller specifies, if it's set.
		blobWriter.ObjectAttrs.ContentType = req["mime_type"]
	}

	// Create an io.Pipe object as an intermediate so that the size of the decode output can be obtained.
	var size int64
	pr, pw := io.Pipe()

	// Create an io.MultWriter to split off data as it streams. This is not necessary in this case but this can be used to
	// do other operations on the streaming data in parallel with the encryption, such as hashing.
	mw := io.MultiWriter(pw)

	// Base64 decode the body.
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(req["body"]))
	go func() {
		// Copy from the decoder to the pipe, which will injected straight into the encryption.
		// This must be done in a goroutine because there is not yet anything ready to receive the data.
		size, err = io.Copy(mw, decoder)
		if err != nil {
			log.Printf("Error base64 decoding body: %v", err)
		}
		pw.Close()
	}()

	// Encrypt the Data using the Data Encryption Key. The data is streamed from the io.Pipe created above.
	if err := s.encryption.Encrypt(ek, pr, blobWriter); err != nil {
		log.Printf("Error encrypting body: %v", err)
		swagger.Errorf(w, http.StatusInternalServerError, "Error writing to backend storage")
		return
	}

	if err := blobWriter.Close(); err != nil {
		log.Printf("Error writing to storage file: %v", err)
		swagger.Errorf(w, http.StatusInternalServerError, "Error writing to backend storage")
		return
	}

	mr := &metadata.Row{
		ID:       filename.String(),
		UserID:   userid,
		Name:     req["name"],
		MimeType: req["mime_type"],
		Uploaded: time.Now(),
		Size:     size,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := metadata.Add(ctx, s.spanner, mr); err != nil {
		log.Printf("Error writing metadata: %v", err)
		swagger.Errorf(w, http.StatusInternalServerError, "Error writing to backend storage")
		return
	}

	// Complete, give the user something to look at.
	json.NewEncoder(w).Encode(*mr)
}

func (s *DocumentService) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	ctx, _ := tag.New(r.Context(), tag.Insert(methodKey, "delete"))
	stats.Record(ctx, s.metrics.requests.M(1))

	userid := gcontext.Get(r, "userid").(string)
	vars := mux.Vars(r)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	mr, err := metadata.Get(ctx, s.spanner, userid, vars["id"])
	if err != nil {
		log.Print(err)
		swagger.Errorf(w, http.StatusNotFound, "Invalid object ID")
		return
	}

	if err := metadata.Delete(ctx, s.spanner, vars["id"]); err != nil {
		log.Print(err)
		swagger.Errorf(w, http.StatusNotFound, "Error deleting metadata")
		return
	}

	bucket := s.storage.Bucket(s.config.Get("storage.bucket"))
	obj := bucket.Object(mr.ID)
	ctx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := obj.Delete(r.Context()); err != nil {
		log.Print(err)
		swagger.Errorf(w, http.StatusNotFound, "Error deleting metadata")
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	flag.Parse()
	ctx := context.Background()

	log.Printf("Starting Document Storage API service v1.0.0")

	config, err := autoconfig.Load(ctx, *configFile)
	if err != nil {
		log.Fatalf("Could not load config file %q: %v", *configFile, err)
	}
	config.AddValidator(func(old, new *autoconfig.Config) error {
		for _, key := range []string{"project", "spanner.instance", "spanner.database"} {
			if old.Get(key) != new.Get(key) {
				return fmt.Errorf("%q cannot be changed while the service is running", key)
			}
		}
		return nil
	})

	// Check that required environment variables are set.
	if os.Getenv("PORT") == "" {
		log.Fatalf("Missing required environment variable \"PORT\"")
	}

	// Check that required configuration options are set.
	for _, v := range []string{"project", "spanner.instance", "spanner.database"} {
		if config.Get(v) == "" {
			log.Fatalf("Missing required configuration option %q", v)
		}
	}

	s, err := NewDocumentService(config, ctx)
	if err != nil {
		log.Fatal(err)
	}

	listenAddr := fmt.Sprintf("[::]:%s", os.Getenv("PORT"))
	log.Printf("Listening on %s", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, s.Handler))
}
