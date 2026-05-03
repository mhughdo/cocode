package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/db"
	"github.com/hughdo/cocode/services/cocoded/internal/devseed"
)

func main() {
	config, err := app.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	dbPath := flag.String("db", config.DBPath, "path to cocoded SQLite database")
	artifactDir := flag.String("artifacts", config.ArtifactDir, "path to cocoded artifact directory")
	workspaceRoot := flag.String("workspace-root", "", "root path to store on the seeded workspace")
	quiet := flag.Bool("quiet", false, "suppress summary output")
	flag.Parse()

	ctx := context.Background()
	database, err := db.Open(ctx, *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.Apply(ctx, database, db.Migrations); err != nil {
		fmt.Fprintf(os.Stderr, "apply migrations: %v\n", err)
		os.Exit(1)
	}

	result, err := devseed.Seed(ctx, database, devseed.Options{
		ArtifactDir:   *artifactDir,
		WorkspaceRoot: *workspaceRoot,
		Now:           time.Now().UTC(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed db: %v\n", err)
		os.Exit(1)
	}

	if *quiet {
		return
	}
	fmt.Fprintf(os.Stdout, "Seeded cocode dev data\n")
	fmt.Fprintf(os.Stdout, "Workspace: %s\n", result.WorkspaceID)
	fmt.Fprintf(os.Stdout, "Repository: %s\n", result.RepositoryID)
	fmt.Fprintf(os.Stdout, "Snapshot: %s\n", result.SnapshotID)
	fmt.Fprintf(os.Stdout, "Review sessions: %d\n", len(result.ReviewSessionIDs))
	fmt.Fprintf(os.Stdout, "Findings: %d\n", len(result.FindingIDs))
	fmt.Fprintf(os.Stdout, "Artifacts: %d\n", len(result.ArtifactIDs))
}
