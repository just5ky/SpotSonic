package converter

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justsky/spotsonic/internal/csvparser"
	"github.com/justsky/spotsonic/internal/matcher"
	"github.com/justsky/spotsonic/internal/state"
	"github.com/justsky/spotsonic/internal/subsonic"
)

// Config holds converter options.
type Config struct {
	Threshold  float64 // min similarity score 0.0–1.0 to accept a match
	DryRun     bool    // preview only, do not create or update playlists
	ReportFile string  // path to write unmatched tracks CSV; empty = skip
	StateFile  string  // path to persistent state JSON
}

// Converter orchestrates CSV parsing, Navidrome API searching, and playlist management.
type Converter struct {
	client     *subsonic.Client
	cfg        Config
	st         *state.State
	report     *csv.Writer
	reportFile *os.File
}

// New creates a Converter with the given Subsonic client and config.
func New(client *subsonic.Client, cfg Config) *Converter {
	return &Converter{client: client, cfg: cfg}
}

// ConvertPath converts a single CSV file or every CSV in a directory.
// State is loaded before processing and saved after all files complete.
func (c *Converter) ConvertPath(path string) error {
	if err := c.client.Ping(); err != nil {
		return fmt.Errorf("cannot reach Navidrome: %w", err)
	}
	if err := c.loadState(); err != nil {
		return err
	}
	if err := c.openReport(); err != nil {
		return err
	}
	defer c.closeReport()

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		err = c.convertDir(path)
	} else {
		err = c.convertFile(path)
	}

	// always save state even if some files errored
	if saveErr := c.saveState(); saveErr != nil {
		log.Printf("ERROR: could not save state: %v", saveErr)
	}
	return err
}

func (c *Converter) convertDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	for i, path := range files {
		log.Printf("[%d/%d] %s", i+1, len(files), filepath.Base(path))
		if err := c.convertFile(path); err != nil {
			log.Printf("  ERROR: %v", err)
		}
	}
	return nil
}

func (c *Converter) convertFile(path string) error {
	tracks, err := csvparser.ParseFile(path)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if len(tracks) == 0 {
		log.Println("  empty file, skipping")
		return nil
	}

	name := playlistName(filepath.Base(path))
	ps := c.st.Playlists[name]

	if ps == nil {
		return c.createPlaylist(name, tracks)
	}
	return c.updatePlaylist(name, tracks, ps)
}

// createPlaylist handles a playlist seen for the first time.
func (c *Converter) createPlaylist(name string, tracks []csvparser.Track) error {
	log.Printf("  [new] %q — %d tracks", name, len(tracks))

	matchedIDs, matchedURIs, unmatched := c.matchAll(name, tracks)

	log.Printf("  matched %d/%d (%.0f%%)", len(matchedIDs), len(tracks), pct(len(matchedIDs), len(tracks)))

	ps := &state.PlaylistState{
		MatchedURIs: matchedURIs,
		Unmatched:   unmatched,
		LastUpdated: time.Now(),
	}

	if !c.cfg.DryRun && len(matchedIDs) > 0 {
		pid, err := c.client.CreatePlaylist(name, matchedIDs)
		if err != nil {
			return fmt.Errorf("create playlist %q: %w", name, err)
		}
		ps.NavidromeID = pid
		log.Printf("  created playlist id=%s", pid)
	}

	c.st.Playlists[name] = ps
	c.writeUnmatchedReport(name, unmatched)
	return nil
}

// updatePlaylist handles a playlist that was processed in a previous run.
// It retries previously unmatched tracks and adds any new tracks from the CSV.
func (c *Converter) updatePlaylist(name string, tracks []csvparser.Track, ps *state.PlaylistState) error {
	unmatchedSet := ps.UnmatchedURISet()

	// separate tracks into: previously unmatched (retry), unseen (new in CSV), already matched (skip)
	var toRetry []csvparser.Track
	var newTracks []csvparser.Track
	for _, t := range tracks {
		if _, alreadyMatched := ps.MatchedURIs[t.URI]; alreadyMatched {
			continue
		}
		if _, wasUnmatched := unmatchedSet[t.URI]; wasUnmatched {
			toRetry = append(toRetry, t)
		} else {
			newTracks = append(newTracks, t)
		}
	}

	log.Printf("  [update] %q — retry %d unmatched, %d new tracks", name, len(toRetry), len(newTracks))

	if len(toRetry) == 0 && len(newTracks) == 0 {
		log.Println("  nothing to do")
		return nil
	}

	// match retries
	var retriedIDs []string
	var stillUnmatched []state.Track
	for _, t := range toRetry {
		id, score, ok := c.findTrack(t)
		if ok {
			retriedIDs = append(retriedIDs, id)
			ps.MatchedURIs[t.URI] = id
			log.Printf("    ✓ (retry) %s — %s (%.0f%%)", t.Name, t.PrimaryArtist, score*100)
		} else {
			stillUnmatched = append(stillUnmatched, state.Track{
				URI: t.URI, Name: t.Name, PrimaryArtist: t.PrimaryArtist, Album: t.Album,
			})
			log.Printf("    ✗ (retry) %s — %s (best %.0f%%)", t.Name, t.PrimaryArtist, score*100)
		}
	}

	// match new tracks from CSV
	newMatchedIDs, newMatchedURIs, newUnmatched := c.matchAll(name, newTracks)
	for uri, id := range newMatchedURIs {
		ps.MatchedURIs[uri] = id
	}
	stillUnmatched = append(stillUnmatched, newUnmatched...)

	allNewIDs := append(retriedIDs, newMatchedIDs...)
	log.Printf("  newly matched: %d (retry: %d, new: %d)", len(allNewIDs), len(retriedIDs), len(newMatchedIDs))

	if !c.cfg.DryRun && len(allNewIDs) > 0 {
		pid, err := c.ensurePlaylistExists(name, ps)
		if err != nil {
			return err
		}
		if err := c.client.UpdatePlaylist(pid, allNewIDs); err != nil {
			return fmt.Errorf("update playlist %q: %w", name, err)
		}
		log.Printf("  added %d songs to playlist id=%s", len(allNewIDs), pid)
	}

	ps.Unmatched = stillUnmatched
	ps.LastUpdated = time.Now()
	c.writeUnmatchedReport(name, stillUnmatched)
	return nil
}

// ensurePlaylistExists returns the Navidrome playlist ID, recovering if it was deleted.
func (c *Converter) ensurePlaylistExists(name string, ps *state.PlaylistState) (string, error) {
	if ps.NavidromeID != "" {
		return ps.NavidromeID, nil
	}
	// state lost NavidromeID — find by name
	pid, err := c.client.FindPlaylistByName(name)
	if err != nil {
		return "", fmt.Errorf("find playlist %q: %w", name, err)
	}
	if pid != "" {
		ps.NavidromeID = pid
		return pid, nil
	}
	// playlist deleted — recreate with all matched songs so far
	ids := make([]string, 0, len(ps.MatchedURIs))
	for _, id := range ps.MatchedURIs {
		ids = append(ids, id)
	}
	pid, err = c.client.CreatePlaylist(name, ids)
	if err != nil {
		return "", fmt.Errorf("recreate playlist %q: %w", name, err)
	}
	ps.NavidromeID = pid
	log.Printf("  recreated playlist id=%s", pid)
	return pid, nil
}

// matchAll searches Navidrome for each track and returns matched IDs, URI→ID map, and unmatched.
func (c *Converter) matchAll(playlistName string, tracks []csvparser.Track) ([]string, map[string]string, []state.Track) {
	var matchedIDs []string
	matchedURIs := make(map[string]string)
	var unmatched []state.Track

	for _, t := range tracks {
		id, score, ok := c.findTrack(t)
		if ok {
			matchedIDs = append(matchedIDs, id)
			matchedURIs[t.URI] = id
			log.Printf("    ✓ %s — %s (%.0f%%)", t.Name, t.PrimaryArtist, score*100)
		} else {
			unmatched = append(unmatched, state.Track{
				URI: t.URI, Name: t.Name, PrimaryArtist: t.PrimaryArtist, Album: t.Album,
			})
			log.Printf("    ✗ %s — %s (best %.0f%%)", t.Name, t.PrimaryArtist, score*100)
		}
	}
	return matchedIDs, matchedURIs, unmatched
}

// findTrack searches Navidrome for a matching song using two strategies:
// 1. search by title alone, score by artist
// 2. search by "artist title" combined
func (c *Converter) findTrack(t csvparser.Track) (id string, score float64, ok bool) {
	songs, err := c.client.Search3(t.Name, 20)
	if err == nil && len(songs) > 0 {
		if s, sc, found := matcher.BestMatch(songs, t.Name, t.PrimaryArtist, c.cfg.Threshold); found {
			return s.ID, sc, true
		}
	}

	query := t.PrimaryArtist + " " + t.Name
	songs2, err2 := c.client.Search3(query, 20)
	if err2 == nil && len(songs2) > 0 {
		s, sc, found := matcher.BestMatch(songs2, t.Name, t.PrimaryArtist, c.cfg.Threshold)
		if found {
			return s.ID, sc, true
		}
		_, sc2, _ := matcher.BestMatch(songs2, t.Name, t.PrimaryArtist, 0)
		return "", sc2, false
	}

	if err != nil {
		log.Printf("    search error: %v", err)
	}
	return "", 0, false
}

func (c *Converter) writeUnmatchedReport(playlist string, unmatched []state.Track) {
	if c.report == nil || len(unmatched) == 0 {
		return
	}
	for _, u := range unmatched {
		if err := c.report.Write([]string{playlist, u.Name, u.PrimaryArtist, u.Album}); err != nil {
			log.Printf("warning: write unmatched report: %v", err)
		}
	}
	c.report.Flush()
	if err := c.report.Error(); err != nil {
		log.Printf("warning: flush unmatched report: %v", err)
	}
}

func (c *Converter) loadState() error {
	if c.cfg.StateFile == "" {
		c.st = state.New()
		return nil
	}
	var err error
	c.st, err = state.Load(c.cfg.StateFile)
	return err
}

func (c *Converter) saveState() error {
	if c.cfg.StateFile == "" || c.cfg.DryRun {
		return nil
	}
	return c.st.Save(c.cfg.StateFile)
}

func (c *Converter) openReport() error {
	if c.cfg.ReportFile == "" {
		return nil
	}
	f, err := os.Create(c.cfg.ReportFile)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	c.reportFile = f
	c.report = csv.NewWriter(f)
	_ = c.report.Write([]string{"Playlist", "Track Name", "Artist", "Album"})
	return nil
}

func (c *Converter) closeReport() {
	if c.report != nil {
		c.report.Flush()
	}
	if c.reportFile != nil {
		c.reportFile.Close()
	}
}

func playlistName(filename string) string {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	return strings.ReplaceAll(name, "_", " ")
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
