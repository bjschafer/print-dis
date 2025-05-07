package database

import (
	"context"
	"os"
	"testing"

	"github.com/bjschafer/print-dis/internal/models"
)

func TestSQLiteClient(t *testing.T) {
	// Create a temporary SQLite database file
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	// Create a new SQLite client
	cfg := &Config{
		Type:     "sqlite",
		Database: tmpfile.Name(),
	}
	client, err := NewDBClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create SQLite client: %v", err)
	}
	defer client.Close()

	// Run the test suite
	testDatabaseClient(t, client)
}

func TestPostgresClient(t *testing.T) {
	// Skip if not running in CI or if environment variables are not set
	if os.Getenv("CI") == "" {
		t.Skip("Skipping PostgreSQL test in non-CI environment")
	}

	// Create a new PostgreSQL client
	cfg := &Config{
		Type:     "postgres",
		Host:     os.Getenv("POSTGRES_HOST"),
		Port:     5432,
		User:     os.Getenv("POSTGRES_USER"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
		Database: os.Getenv("POSTGRES_DB"),
		SSLMode:  "disable",
	}
	client, err := NewDBClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL client: %v", err)
	}
	defer client.Close()

	// Run the test suite
	testDatabaseClient(t, client)
}

func testDatabaseClient(t *testing.T, client DBClient) {
	ctx := context.Background()

	// Test printer operations
	t.Run("Printer CRUD", func(t *testing.T) {
		// Create a printer
		printer := &models.Printer{
			Name: "Test Printer",
			Dimensions: models.Dimension{
				X: 200,
				Y: 200,
				Z: 200,
			},
			Url: "http://localhost:8080",
		}

		err := client.CreatePrinter(ctx, printer)
		if err != nil {
			t.Fatalf("Failed to create printer: %v", err)
		}

		// Get the printer
		got, err := client.GetPrinter(ctx, printer.Id)
		if err != nil {
			t.Fatalf("Failed to get printer: %v", err)
		}
		if got == nil {
			t.Fatal("Expected printer to exist")
		}
		if got.Name != printer.Name {
			t.Errorf("Expected printer name %q, got %q", printer.Name, got.Name)
		}

		// Update the printer
		printer.Name = "Updated Printer"
		err = client.UpdatePrinter(ctx, printer)
		if err != nil {
			t.Fatalf("Failed to update printer: %v", err)
		}

		// Verify the update
		got, err = client.GetPrinter(ctx, printer.Id)
		if err != nil {
			t.Fatalf("Failed to get updated printer: %v", err)
		}
		if got.Name != printer.Name {
			t.Errorf("Expected updated printer name %q, got %q", printer.Name, got.Name)
		}

		// List printers
		printers, err := client.ListPrinters(ctx)
		if err != nil {
			t.Fatalf("Failed to list printers: %v", err)
		}
		if len(printers) != 1 {
			t.Errorf("Expected 1 printer, got %d", len(printers))
		}

		// Delete the printer
		err = client.DeletePrinter(ctx, printer.Id)
		if err != nil {
			t.Fatalf("Failed to delete printer: %v", err)
		}

		// Verify deletion
		got, err = client.GetPrinter(ctx, printer.Id)
		if err != nil {
			t.Fatalf("Failed to get deleted printer: %v", err)
		}
		if got != nil {
			t.Error("Expected printer to be deleted")
		}
	})

	// Test filament operations
	t.Run("Filament CRUD", func(t *testing.T) {
		// Create a material first
		material := &models.Material{
			Name: "Test Material",
		}
		err := client.CreateMaterial(ctx, material)
		if err != nil {
			t.Fatalf("Failed to create material: %v", err)
		}

		// Create a filament
		filament := &models.Filament{
			Name:     "Test Filament",
			Material: *material,
		}

		err = client.CreateFilament(ctx, filament)
		if err != nil {
			t.Fatalf("Failed to create filament: %v", err)
		}

		// Get the filament
		got, err := client.GetFilament(ctx, filament.Id)
		if err != nil {
			t.Fatalf("Failed to get filament: %v", err)
		}
		if got == nil {
			t.Fatal("Expected filament to exist")
		}
		if got.Name != filament.Name {
			t.Errorf("Expected filament name %q, got %q", filament.Name, got.Name)
		}

		// Update the filament
		filament.Name = "Updated Filament"
		err = client.UpdateFilament(ctx, filament)
		if err != nil {
			t.Fatalf("Failed to update filament: %v", err)
		}

		// Verify the update
		got, err = client.GetFilament(ctx, filament.Id)
		if err != nil {
			t.Fatalf("Failed to get updated filament: %v", err)
		}
		if got.Name != filament.Name {
			t.Errorf("Expected updated filament name %q, got %q", filament.Name, got.Name)
		}

		// List filaments
		filaments, err := client.ListFilaments(ctx)
		if err != nil {
			t.Fatalf("Failed to list filaments: %v", err)
		}
		if len(filaments) != 1 {
			t.Errorf("Expected 1 filament, got %d", len(filaments))
		}

		// Delete the filament
		err = client.DeleteFilament(ctx, filament.Id)
		if err != nil {
			t.Fatalf("Failed to delete filament: %v", err)
		}

		// Verify deletion
		got, err = client.GetFilament(ctx, filament.Id)
		if err != nil {
			t.Fatalf("Failed to get deleted filament: %v", err)
		}
		if got != nil {
			t.Error("Expected filament to be deleted")
		}
	})

	// Test job operations
	t.Run("Job CRUD", func(t *testing.T) {
		// Create required dependencies
		printer := &models.Printer{
			Name: "Test Printer",
			Dimensions: models.Dimension{
				X: 200,
				Y: 200,
				Z: 200,
			},
			Url: "http://localhost:8080",
		}
		err := client.CreatePrinter(ctx, printer)
		if err != nil {
			t.Fatalf("Failed to create printer: %v", err)
		}

		material := &models.Material{
			Name: "Test Material",
		}
		err = client.CreateMaterial(ctx, material)
		if err != nil {
			t.Fatalf("Failed to create material: %v", err)
		}

		filament := &models.Filament{
			Name:     "Test Filament",
			Material: *material,
		}
		err = client.CreateFilament(ctx, filament)
		if err != nil {
			t.Fatalf("Failed to create filament: %v", err)
		}

		// Create a job
		job := &models.Job{
			Printer:  printer,
			Filament: filament,
			Material: material,
		}

		err = client.CreateJob(ctx, job)
		if err != nil {
			t.Fatalf("Failed to create job: %v", err)
		}

		// Get the job
		got, err := client.GetJob(ctx, job.Id)
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}
		if got == nil {
			t.Fatal("Expected job to exist")
		}
		if got.Printer.Name != printer.Name {
			t.Errorf("Expected printer name %q, got %q", printer.Name, got.Printer.Name)
		}

		// Update the job
		printer.Name = "Updated Printer"
		err = client.UpdatePrinter(ctx, printer)
		if err != nil {
			t.Fatalf("Failed to update printer: %v", err)
		}

		// Verify the update
		got, err = client.GetJob(ctx, job.Id)
		if err != nil {
			t.Fatalf("Failed to get updated job: %v", err)
		}
		if got.Printer.Name != printer.Name {
			t.Errorf("Expected updated printer name %q, got %q", printer.Name, got.Printer.Name)
		}

		// List jobs
		jobs, err := client.ListJobs(ctx)
		if err != nil {
			t.Fatalf("Failed to list jobs: %v", err)
		}
		if len(jobs) != 1 {
			t.Errorf("Expected 1 job, got %d", len(jobs))
		}

		// Delete the job
		err = client.DeleteJob(ctx, job.Id)
		if err != nil {
			t.Fatalf("Failed to delete job: %v", err)
		}

		// Verify deletion
		got, err = client.GetJob(ctx, job.Id)
		if err != nil {
			t.Fatalf("Failed to get deleted job: %v", err)
		}
		if got != nil {
			t.Error("Expected job to be deleted")
		}
	})
}
