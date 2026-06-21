package state

import (
	"encoding/json"
	"os"
	"time"
)

const version = 1

// Track is a minimal record of an unmatched Spotify track kept between runs.
type Track struct {
	URI           string `json:"uri"`
	Name          string `json:"name"`
	PrimaryArtist string `json:"primary_artist"`
	Album         string `json:"album"`
}

// PlaylistState tracks what has been matched and what is still pending for one playlist.
type PlaylistState struct {
	NavidromeID string            `json:"navidrome_id"`
	MatchedURIs map[string]string `json:"matched_uris"` // Spotify URI → Navidrome song ID
	Unmatched   []Track           `json:"unmatched"`
	LastUpdated time.Time         `json:"last_updated"`
}

// State is the top-level state file structure.
type State struct {
	Version   int                       `json:"version"`
	Playlists map[string]*PlaylistState `json:"playlists"`
}

// New returns an empty State.
func New() *State {
	return &State{
		Version:   version,
		Playlists: make(map[string]*PlaylistState),
	}
}

// Load reads a State from path. Returns a fresh State if the file does not exist.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Playlists == nil {
		s.Playlists = make(map[string]*PlaylistState)
	}
	return &s, nil
}

// Save writes s to path atomically (write to temp, rename).
func (s *State) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup
		return err
	}
	return nil
}

// NewPlaylist creates a fresh PlaylistState.
func NewPlaylist(navidromeID string) *PlaylistState {
	return &PlaylistState{
		NavidromeID: navidromeID,
		MatchedURIs: make(map[string]string),
		LastUpdated: time.Now(),
	}
}

// UnmatchedURISet returns the set of Spotify URIs in Unmatched for fast lookup.
func (ps *PlaylistState) UnmatchedURISet() map[string]struct{} {
	m := make(map[string]struct{}, len(ps.Unmatched))
	for _, t := range ps.Unmatched {
		m[t.URI] = struct{}{}
	}
	return m
}
