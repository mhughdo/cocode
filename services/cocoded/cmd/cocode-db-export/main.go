package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/devexport"
)

func main() {
	config, err := app.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	dbPath := flag.String("db", config.DBPath, "path to cocoded SQLite database")
	outPath := flag.String("out", "", "optional output file; stdout when empty")
	pretty := flag.Bool("pretty", true, "pretty-print JSON output")
	flag.Parse()

	ctx := context.Background()
	database, err := devexport.OpenReadOnly(ctx, *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	dump, err := devexport.Export(ctx, database, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "export db: %v\n", err)
		os.Exit(1)
	}

	var data []byte
	if *pretty {
		data, err = json.MarshalIndent(dump, "", "  ")
	} else {
		data, err = json.Marshal(dump)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode export: %v\n", err)
		os.Exit(1)
	}
	data = append(data, '\n')

	if *outPath == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			fmt.Fprintf(os.Stderr, "write stdout: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(*outPath, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write output: %v\n", err)
		os.Exit(1)
	}
}
