package csvparser

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

// Track holds the fields needed for Navidrome matching from an Exportify CSV row.
type Track struct {
	URI           string
	Name          string
	Album         string
	Artists       []string
	PrimaryArtist string
}

// ParseFile reads an Exportify CSV file and returns all tracks.
func ParseFile(path string) ([]Track, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads Exportify CSV from r.
func Parse(r io.Reader) ([]Track, error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	idx := buildIndex(header)

	for _, col := range []string{"Track URI", "Track Name", "Artist Name(s)"} {
		if _, ok := idx[col]; !ok {
			return nil, fmt.Errorf("missing required column %q", col)
		}
	}

	var tracks []Track
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}
		if len(row) == 0 {
			continue
		}
		t := Track{
			URI:   get(row, idx, "Track URI"),
			Name:  get(row, idx, "Track Name"),
			Album: get(row, idx, "Album Name"),
		}
		t.Artists = splitArtists(get(row, idx, "Artist Name(s)"))
		if len(t.Artists) > 0 {
			t.PrimaryArtist = t.Artists[0]
		}
		if t.Name == "" {
			continue
		}
		tracks = append(tracks, t)
	}
	return tracks, nil
}

func buildIndex(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.TrimSpace(h)] = i
	}
	return m
}

func get(row []string, idx map[string]int, col string) string {
	i, ok := idx[col]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func splitArtists(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
