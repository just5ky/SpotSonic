package subsonic

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	apiVersion = "1.16.0"
	clientName = "SpotSonic"
	chunkSize  = 200
)

// Client is a Subsonic/Navidrome REST API client.
type Client struct {
	baseURL  string
	user     string
	password string
	http     *http.Client
}

// NewClient creates a Client for the given Navidrome server.
func NewClient(baseURL, user, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		user:     user,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Ping verifies connectivity and credentials.
func (c *Client) Ping() error {
	_, err := c.get("ping", nil)
	return err
}

// Search3 searches for songs matching query, returning up to count results.
func (c *Client) Search3(query string, count int) ([]Song, error) {
	params := url.Values{
		"query":       {query},
		"songCount":   {fmt.Sprint(count)},
		"artistCount": {"0"},
		"albumCount":  {"0"},
	}
	resp, err := c.get("search3", params)
	if err != nil {
		return nil, err
	}
	if resp.SearchResult3 == nil {
		return nil, nil
	}
	return resp.SearchResult3.Song, nil
}

// CreatePlaylist creates a new playlist with the given song IDs.
// Large playlists are chunked via updatePlaylist to stay within URL limits.
func (c *Client) CreatePlaylist(name string, songIDs []string) (string, error) {
	first := songIDs
	if len(first) > chunkSize {
		first = songIDs[:chunkSize]
	}

	params := url.Values{"name": {name}}
	for _, id := range first {
		params.Add("songId", id)
	}
	resp, err := c.get("createPlaylist", params)
	if err != nil {
		return "", err
	}
	if resp.Playlist == nil {
		return "", fmt.Errorf("no playlist returned")
	}
	pid := resp.Playlist.ID

	for i := chunkSize; i < len(songIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(songIDs) {
			end = len(songIDs)
		}
		if err := c.updatePlaylist(pid, songIDs[i:end]); err != nil {
			return pid, fmt.Errorf("update playlist chunk %d: %w", i/chunkSize+1, err)
		}
	}
	return pid, nil
}

// GetPlaylists returns all playlists visible to the authenticated user.
func (c *Client) GetPlaylists() ([]Playlist, error) {
	resp, err := c.get("getPlaylists", nil)
	if err != nil {
		return nil, err
	}
	if resp.Playlists == nil {
		return nil, nil
	}
	return resp.Playlists.Playlist, nil
}

// FindPlaylistByName searches Navidrome for a playlist with the given name.
// Returns ("", nil) if not found.
func (c *Client) FindPlaylistByName(name string) (string, error) {
	playlists, err := c.GetPlaylists()
	if err != nil {
		return "", err
	}
	for _, p := range playlists {
		if p.Name == name {
			return p.ID, nil
		}
	}
	return "", nil
}

// UpdatePlaylist appends song IDs to an existing playlist (exported for use by converter).
func (c *Client) UpdatePlaylist(playlistID string, songIDsToAdd []string) error {
	return c.updatePlaylist(playlistID, songIDsToAdd)
}

func (c *Client) updatePlaylist(playlistID string, songIDsToAdd []string) error {
	params := url.Values{"playlistId": {playlistID}}
	for _, id := range songIDsToAdd {
		params.Add("songIdToAdd", id)
	}
	_, err := c.get("updatePlaylist", params)
	return err
}

func (c *Client) get(method string, params url.Values) (*SubsonicResponse, error) {
	u, err := url.Parse(fmt.Sprintf("%s/rest/%s", c.baseURL, method))
	if err != nil {
		return nil, err
	}
	q := c.authParams()
	for k, vs := range params {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()

	resp, err := c.http.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var result Response
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", method, err)
	}

	sr := &result.SubsonicResponse
	if sr.Status != "ok" {
		if sr.Error != nil {
			return nil, fmt.Errorf("subsonic error %d: %s", sr.Error.Code, sr.Error.Message)
		}
		return nil, fmt.Errorf("subsonic status: %s", sr.Status)
	}
	return sr, nil
}

func (c *Client) authParams() url.Values {
	salt := randomSalt()
	return url.Values{
		"u": {c.user},
		"t": {md5hex(c.password + salt)},
		"s": {salt},
		"v": {apiVersion},
		"c": {clientName},
		"f": {"json"},
	}
}

func md5hex(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}

func randomSalt() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 12)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
