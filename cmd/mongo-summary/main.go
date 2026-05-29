package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jerichorivera/mongo-summary/internal/collector"
	"github.com/jerichorivera/mongo-summary/internal/report"
)

func main() {
	input := flag.String("input", ".", "input folder containing MongoDB collection text files")
	output := flag.String("output", "mongodb-summary.html", "output HTML file")
	title := flag.String("title", "", "report title; defaults to the input folder name")
	flag.Parse()

	if *title == "" {
		*title = filepath.Base(filepath.Clean(*input))
	}

	summary, err := collector.Collect(*input, *title)
	if err != nil {
		fmt.Fprintf(os.Stderr, "collect: %v\n", err)
		os.Exit(1)
	}

	if err := report.WriteHTML(*output, summary); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s\n", *output)
}
