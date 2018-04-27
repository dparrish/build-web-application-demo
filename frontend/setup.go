package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/dparrish/build-web-application-demo/encryption"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/storage"

	"go.opencensus.io/exporter/stackdriver"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
)

var traceClient string

// createClients creates all the required Cloud API clients in parallel to reduce startup time.
func (s *DocumentService) createClients(ctx context.Context) {
	var wg sync.WaitGroup
	var err error

	wg.Add(1)
	go func() {
		// Create KMS client.
		defer wg.Done()
		s.encryption = encryption.New(ctx, s.config)
	}()

	wg.Add(1)
	go func() {
		// Create BigQuery client.
		defer wg.Done()
		s.bigquery, err = bigquery.NewClient(ctx, s.config.Get("project"))
		if err != nil {
			log.Fatalf("Error creating BigQuery client: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Create Cloud Storage client.
		s.storage, err = storage.NewClient(ctx)
		if err != nil {
			log.Fatalf("Error creating Cloud Storage client: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		// Create Cloud Spanner client.
		defer wg.Done()
		dbName := fmt.Sprintf("projects/%s/instances/%s/databases/%s", s.config.Get("project"), s.config.Get("spanner.instance"), s.config.Get("spanner.database"))
		s.spanner, err = spanner.NewClient(ctx, dbName)
		if err != nil {
			log.Fatalf("Error creating Cloud Spanner client: %v", err)
		}
		if *warmSpanner {
			// Perform a warming query to force the Spanner client to connect.
			func() {
				stmt := spanner.NewStatement(`SELECT 1`)
				ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				iter := s.spanner.Single().Query(ctx, stmt)
				defer iter.Stop()
			}()
		}
	}()

	wg.Add(1)
	go func() {
		// Create OpenCensus monitoring metrics that are exported to Stackdriver.
		defer wg.Done()

		// Export to Stackdriver Monitoring.
		s.exporter, err = stackdriver.NewExporter(stackdriver.Options{ProjectID: s.config.Get("project")})
		if err != nil {
			log.Fatal(err)
		}
		view.RegisterExporter(s.exporter)
		view.SetReportingPeriod(1 * time.Second)

		// Export to Stackdriver Trace.
		trace.RegisterExporter(s.exporter)
		trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

		// Create exported metrics and views.
		s.metrics.documentCount = stats.Int64("frontend/measure/document_count", "Number of documents in a list response", "docs")
		view.Register(&view.View{
			Name:        "frontend/views/list_document_count",
			Description: "list document count over time",
			Measure:     s.metrics.documentCount,
			Aggregation: view.Distribution(0, 1<<2, 1<<3, 1<<4, 1<<5, 1<<6, 1<<7),
		})

		s.metrics.retrieveAge = stats.Float64("frontend/measure/retrieve_age", "Age of document at retrieval time", "seconds")
		view.Register(&view.View{
			Name:        "frontend/views/retrieve_age",
			Description: "list document count over time",
			Measure:     s.metrics.retrieveAge,
			Aggregation: view.Distribution(0, 1<<2, 1<<3, 1<<4, 1<<5, 1<<6, 1<<7),
		})

		s.metrics.requests = stats.Int64("frontend/measure/requests", "Number of requests", "requests")
		methodKey, _ = tag.NewKey("frontend/keys/method")
		view.Register(&view.View{
			Name:        "frontend/views/requests",
			Description: "requests over time",
			TagKeys:     []tag.Key{methodKey},
			Measure:     s.metrics.requests,
			Aggregation: view.Count(),
		})
	}()

	wg.Wait()
}
