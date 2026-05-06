package subsonic

// Response is the outer Subsonic JSON envelope.
type Response struct {
	SubsonicResponse SubsonicResponse `json:"subsonic-response"`
}

// SubsonicResponse holds the status and optional payload fields.
type SubsonicResponse struct {
	Status        string         `json:"status"`
	Version       string         `json:"version"`
	Error         *APIError      `json:"error,omitempty"`
	SearchResult3 *SearchResult3 `json:"searchResult3,omitempty"`
	Playlist      *Playlist      `json:"playlist,omitempty"`
	Playlists     *Playlists     `json:"playlists,omitempty"`
}

// APIError is a Subsonic error payload.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// SearchResult3 holds songs returned by search3.
type SearchResult3 struct {
	Song []Song `json:"song"`
}

// Song is a Navidrome song entry.
type Song struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
}

// Playlist is a Navidrome playlist entry.
type Playlist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Playlists is the container returned by getPlaylists.
type Playlists struct {
	Playlist []Playlist `json:"playlist"`
}
