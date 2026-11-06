package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/justsky/spotsonic/internal/converter"
	"github.com/justsky/spotsonic/internal/subsonic"
)

var version = "dev"

func main() {
	log.SetFlags(0)

	server := flag.String("server", "", "Navidrome URL (e.g. http://localhost:4533)")
	user := flag.String("user", "", "Navidrome username")
	pass := flag.String("password", "", "Navidrome password (env: SPOTSONIC_PASSWORD)")
	input := flag.String("input", ".", "Input CSV file or directory of CSV files")
	threshold := flag.Float64("threshold", 0.80, "Fuzzy match threshold 0.0–1.0")
	dryRun := flag.Bool("dry-run", false, "Preview matches without creating or updating playlists")
	quiet := flag.Bool("quiet", false, "Suppress per-track match/no-match log lines (summary lines still shown)")
	reportFile := flag.String("report", "", "Write currently unmatched tracks to this CSV file")
	stateFile := flag.String("state", "spotsonic-state.json", "State file for incremental updates (tracks matched/unmatched between runs)")
	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "SpotSonic %s — Exportify CSV → Navidrome playlist converter\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage:\n  spotsonic [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # First run — creates playlists\n")
		fmt.Fprintf(os.Stderr, "  spotsonic -server http://localhost:4533 -user admin -password secret -input ./playlists/\n\n")
		fmt.Fprintf(os.Stderr, "  # Subsequent runs — retries unmatched, adds newly found songs\n")
		fmt.Fprintf(os.Stderr, "  spotsonic -server http://localhost:4533 -user admin -password secret -input ./playlists/ -state spotsonic-state.json\n\n")
		fmt.Fprintf(os.Stderr, "  # Dry-run preview\n")
		fmt.Fprintf(os.Stderr, "  spotsonic -server http://localhost:4533 -user admin -password secret -input playlist.csv -dry-run\n")
	}

	flag.Parse()

	if *pass == "" {
		*pass = os.Getenv("SPOTSONIC_PASSWORD")
	}

	if *showVersion {
		fmt.Printf("SpotSonic %s\n", version)
		return
	}

	if *server == "" || *user == "" || *pass == "" {
		fmt.Fprintln(os.Stderr, "error: -server, -user, and -password (or SPOTSONIC_PASSWORD env var) are required")
		flag.Usage()
		os.Exit(1)
	}

	client := subsonic.NewClient(*server, *user, *pass)
	conv := converter.New(client, converter.Config{
		Threshold:  *threshold,
		DryRun:     *dryRun,
		ReportFile: *reportFile,
		StateFile:  *stateFile,
		Quiet:      *quiet,
	})

	if *dryRun {
		log.Println("dry-run mode: no playlists will be created or modified")
	}

	if err := conv.ConvertPath(*input); err != nil {
		log.Fatalf("error: %v", err)
	}
}
