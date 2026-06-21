# SpotSonic Code Audit Report

## Executive Summary

This audit examined the SpotSonic Go project, a tool that converts Exportify CSV playlists into Navidrome playlists via the Subsonic API. The codebase is relatively small (~600 lines of Go) and follows a clean separation of concerns across 7 internal packages.

**Overall Assessment**: The code is well-structured with good separation of concerns. However, several issues were identified related to error handling, resource management, security, and edge cases that should be addressed before production use.

---

## Table of Contents

1. [Code Quality Issues](#code-quality-issues)
2. [Security Concerns](#security-concerns)
3. [Error Handling & Edge Cases](#error-handling--edge-cases)
4. [Resource Management](#resource-management)
5. [Logging Best Practices](#logging-best-practices)
6. [Documentation & Maintainability](#documentation--maintainability)
7. [Recommendations](#recommendations)

---

## Code Quality Issues

### 1. Inconsistent Error Handling in `main.go`

**Location**: `main.go:48-52`

```go
if *server == "" || *user == "" || *pass == "" {
    fmt.Fprintln(os.Stderr, "error: -server, -user, and -password are required")
    flag.Usage()
    os.Exit(1)
}
```

**Issue**: The error message is hardcoded and doesn't list the actual missing flags. While this is a simple validation check, it would be better to use `flag.VisitErr()` or collect all invalid values before exiting.

### 2. Silent Error Swallowing in `converter.go`

**Location**: `converter.go:65-67`

```go
// always save state even if some files errored
if saveErr := c.saveState(); saveErr != nil {
    log.Printf("warning: could not save state: %v", saveErr)
}
return err
```

**Issue**: The warning is logged but the error from `saveState()` is completely ignored. This could lead to data loss if the state file cannot be written (e.g., disk full, permission denied). Consider either:
- Returning an error up the call stack
- Logging at ERROR level instead of WARNING
- Attempting a fallback mechanism

### 3. Unnecessary Error Suppression in `converter.go`

**Location**: `converter.go:291-293`

```go
func (c *Converter) writeUnmatchedReport(playlist string, unmatched []state.Track) {
    if c.report == nil || len(unmatched) == 0 {
        return
    }
    for _, u := range unmatched {
        _ = c.report.Write([]string{playlist, u.Name, u.PrimaryArtist, u.Album})
    }
    c.report.Flush()
}
```

**Issue**: The error from `c.report.Write()` is ignored with `_`. If CSV writing fails (e.g., special characters in track names), the report becomes corrupted without any indication. Should check and handle errors properly.

### 4. No Timeout on State File Operations

**Location**: `state/state.go:61-70`

```go
func (s *State) Save(path string) error {
    data, err := json.MarshalIndent(s, "", "  ")
    if err != nil {
        return err
    }
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, data, 0o644); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}
```

**Issue**: No timeout or cancellation mechanism. If the process is killed during a state save (e.g., Ctrl+C), the temporary file could be left behind, causing issues on subsequent runs. Consider adding:
- A cleanup function in `defer` to remove `.tmp` files
- Context-based cancellation support

### 5. No Validation of CSV Column Names

**Location**: `csvparser/parser.go:42-46`

```go
for _, col := range []string{"Track URI", "Track Name", "Artist Name(s)"} {
    if _, ok := idx[col]; !ok {
        return nil, fmt.Errorf("missing required column %q", col)
    }
}
```

**Issue**: The code expects exact column names including spaces. Exportify CSV exports may have slightly different formatting (e.g., "Track URI" vs "track uri"). Consider:
- Normalizing column names to lowercase before comparison
- Providing a helpful error message with the actual header found

### 6. No Handling of Empty/Whitespace-Only Track Names

**Location**: `csvparser/parser.go:69-71`

```go
if t.Name == "" {
    continue
}
tracks = append(tracks, t)
```

**Issue**: Only checks for empty strings but not whitespace-only names. A track named "   " would be included and could cause matching issues. Consider trimming:

```go
t.Name = strings.TrimSpace(t.Name)
if t.Name == "" {
    continue
}
```

---

## Security Concerns

### 1. Password in Command Line Arguments

**Location**: `main.go:20-50`

```go
pass := flag.String("password", "", "Navidrome password")
...
if *server == "" || *user == "" || *pass == "" {
    fmt.Fprintln(os.Stderr, "error: -server, -user, and -password are required")
    flag.Usage()
    os.Exit(1)
}
```

**Issue**: Passwords passed via command-line arguments can be:
- Visible in process listings (`ps aux`)
- Stored in shell history
- Passed to other processes (e.g., `grep`, `kill -SIGUSR1`)
- Logged by some systems

**Recommendation**: 
- Use environment variables as an alternative or default
- Add a `-password-file` flag that reads from a file with restricted permissions
- Document this security limitation in the README

### 2. No Rate Limiting on API Calls

**Location**: `subsonic/client.go:137-174`

```go
func (c *Client) get(method string, params url.Values) (*SubsonicResponse, error) {
    u, err := url.Parse(fmt.Sprintf("%s/rest/%s", c.baseURL, method))
    ...
    resp, err := c.http.Get(u.String())
    ...
}
```

**Issue**: The HTTP client has no rate limiting. Navidrome's Subsonic API may have rate limits (typically 10-20 requests per minute). Rapid failures could:
- Trigger IP blocking
- Cause the server to throttle responses
- Waste bandwidth on failed retries

**Recommendation**: Add a `rate_limiter` or use a library like `golang.org/x/time/rate`. Consider adding retry logic with exponential backoff for transient errors.

### 3. No TLS Verification Configuration

**Location**: `subsonic/client.go:35`

```go
http: &http.Client{Timeout: 30 * time.Second},
```

**Issue**: The HTTP client uses the default transport which verifies TLS certificates. While this is correct behavior, consider adding a comment explaining that users should not disable TLS verification in production. Some users might be tempted to add `InsecureSkipVerify: true` for self-signed certificates, which is a security risk.

### 4. No Input Validation on CSV Data

**Location**: Throughout the codebase

**Issue**: The code trusts all data from CSV files without validation:
- Track names could contain path traversal sequences (`../`)
- Artist names could be used in file operations (though none currently exist)
- Playlist names derived from filenames could contain shell metacharacters

While most of these are mitigated by Go's string handling, it's good practice to sanitize inputs when they're used in user-facing contexts.

---

## Error Handling & Edge Cases

### 1. No Retry Logic for Network Failures

**Location**: `subsonic/client.go:150-164`

```go
resp, err := c.http.Get(u.String())
if err != nil {
    return nil, fmt.Errorf("%s: %w", method, err)
}
...
var result Response
if err := json.Unmarshal(body, &result); err != nil {
    return nil, fmt.Errorf("decode %s response: %w", method, err)
}
```

**Issue**: Network failures (DNS resolution, connection refused, timeout) are not retried. A single transient failure could cause the entire conversion to fail. Consider adding retry logic with exponential backoff for network errors only.

### 2. No Handling of Partial CSV Reads

**Location**: `csvparser/parser.go:50-56`

```go
row, err := cr.Read()
if err == io.EOF {
    break
}
if err != nil {
    return nil, fmt.Errorf("read row: %w", err)
}
```

**Issue**: If `cr.Read()` fails partway through a large file (e.g., disk full during write), the error is returned but no partial results are saved. Consider:
- Saving matched tracks to a temporary file before processing
- Using streaming processing for very large CSV files (>1GB)

### 3. No Validation of State File Format on Load

**Location**: `state/state.go:50-53`

```go
var s State
if err := json.Unmarshal(data, &s); err != nil {
    return nil, err
}
```

**Issue**: If the state file is corrupted or from a future version, it will fail to load. Consider:
- Adding a `version` field check and migration logic
- Providing a helpful error message suggesting manual cleanup
- Using a schema validation library like `go-playground/validator`

### 4. No Handling of Duplicate Playlist Names

**Location**: `converter.go:101-108`

```go
name := playlistName(filepath.Base(path))
ps := c.st.Playlists[name]
if ps == nil {
    return c.createPlaylist(name, tracks)
}
return c.updatePlaylist(name, tracks, ps)
```

**Issue**: If two CSV files have the same base name (e.g., `playlist.csv` and `playlist_backup.csv`), they will be treated as the same playlist. This could lead to:
- Overwriting state from one file with another
- Incorrect song matching between different playlists

Consider adding a `-prefix` flag or requiring unique filenames.

### 5. No Handling of API Rate Limits

**Location**: `subsonic/client.go:167-172`

```go
sr := &result.SubsonicResponse
if sr.Status != "ok" {
    if sr.Error != nil {
        return nil, fmt.Errorf("subsonic error %d: %s", sr.Error.Code, sr.Error.Message)
    }
    return nil, fmt.Errorf("subsonic status: %s", sr.Status)
}
```

**Issue**: If Navidrome returns a 429 (Too Many Requests), the code will fail immediately. Consider treating rate limit errors specially and implementing backoff retry logic.

---

## Resource Management

### 1. No Cleanup of Temporary Files

**Location**: `state/state.go:66-70`

```go
tmp := path + ".tmp"
if err := os.WriteFile(tmp, data, 0o644); err != nil {
    return err
}
return os.Rename(tmp, path)
```

**Issue**: If `os.Rename()` fails (e.g., cross-filesystem rename on some systems), the `.tmp` file is left behind. Consider:
- Using `defer os.Remove(tmp)` to clean up on any exit path
- Checking if the temp file exists before cleanup

### 2. No File Size Limits

**Location**: Throughout CSV parsing

**Issue**: The code reads entire CSV files into memory. For very large playlists (10,000+ tracks), this could:
- Exhaust RAM
- Cause OOM kills on constrained systems

Consider using streaming processing or chunked reading for large files.

### 3. No Connection Pool Configuration

**Location**: `subsonic/client.go:35`

```go
http: &http.Client{Timeout: 30 * time.Second},
```

**Issue**: The default HTTP client uses a connection pool with no custom configuration. For high-frequency API calls, consider:
- Setting `MaxIdleConns` and `MaxIdleConnsPerHost`
- Configuring keep-alive timeouts
- Using a dedicated connection pool for the Subsonic client

---

## Logging Best Practices

### 1. Inconsistent Log Levels

**Location**: Throughout the codebase

```go
log.Printf("  [new] %q — %d tracks", name, len(tracks))
log.Printf("  matched %d/%d (%.0f%%)", len(matchedIDs), len(tracks), pct(len(matchedIDs), len(tracks)))
log.Printf("    ✓ %s — %s (%.0f%%)", t.Name, t.PrimaryArtist, score*100)
log.Printf("    ✗ %s — %s (best %.0f%%)", t.Name, t.PrimaryArtist, score*100)
```

**Issue**: All logs use `log.Printf()` which defaults to INFO level. Consider:
- Using structured logging with a library like `go.uber.org/zap` or `log/slog` (Go 1.21+)
- Adding log levels for DEBUG (matching details), INFO (progress), WARN (recoverable issues), ERROR (failures)
- Including timestamps and request IDs for debugging

### 2. No Structured Logging

**Location**: All logging calls

```go
log.Printf("error: %v", err)
```

**Issue**: Logs are unstructured, making it difficult to parse with tools like ELK or Splunk. Consider migrating to structured logging.

### 3. Sensitive Data in Logs

**Location**: `converter.go:172-178`

```go
log.Printf("    ✓ (retry) %s — %s (%.0f%%)", t.Name, t.PrimaryArtist, score*100)
log.Printf("    ✗ (retry) %s — %s (best %.0f%%)", t.Name, t.PrimaryArtist, score*100)
```

**Issue**: While track names and artists are generally safe, consider redacting any user-provided data that could be used for fingerprinting or tracking. This is a minor concern but good practice.

---

## Documentation & Maintainability

### 1. Missing Code Comments

**Location**: Throughout the codebase

```go
func (c *Converter) convertDir(dir string) error {
    entries, err := os.ReadDir(dir)
    ...
}
```

**Issue**: Most functions lack comments explaining their purpose and behavior. Consider adding:
- Function-level doc comments following Go conventions
- Comments for non-obvious logic (e.g., the two-pass matching strategy in `findTrack`)
- Examples of usage in the README

### 2. No Unit Tests

**Location**: Not present in repository

**Issue**: The codebase has no tests. This makes it difficult to:
- Verify bug fixes
- Refactor with confidence
- Document expected behavior

Consider adding unit tests for:
- CSV parsing edge cases (quoted fields, empty rows)
- Matching algorithm correctness
- State file serialization/deserialization
- API client error handling

### 3. No CHANGELOG or Version History

**Location**: Not present in repository

**Issue**: There's no record of what changed between versions. Consider adding:
- A `CHANGELOG.md` following [Keep a Changelog](https://keepachangelog.com/) format
- Semantic versioning with clear release notes
- Git tags for releases

### 4. Incomplete README Examples

**Location**: `README.md`

**Issue**: The examples show basic usage but don't cover:
- Error handling in scripts
- CI/CD integration
- Troubleshooting common issues
- Performance tuning (threshold values, batch sizes)

---

## Recommendations

### High Priority

1. **Add proper error handling for state file saves** - Don't silently ignore errors; either return them or log at ERROR level with a suggestion to check disk space/permissions.

2. **Implement retry logic with exponential backoff** - Add retry attempts for network failures and API rate limits, especially in the `get()` method of the Subsonic client.

3. **Add password file support** - Implement `-password-file` flag that reads from a file with restricted permissions (0600). Document this as a more secure alternative to command-line arguments.

4. **Migrate to structured logging** - Use `log/slog` (Go 1.21+) or `zap` for better log parsing and filtering capabilities.

5. **Add unit tests** - Start with critical paths: CSV parsing, matching algorithm, state serialization. Aim for >80% coverage of core logic.

### Medium Priority

6. **Improve error messages** - Make them more helpful by including context (e.g., which file failed, what operation was in progress).

7. **Add input sanitization** - Validate and sanitize CSV data before using it in any user-facing contexts.

8. **Implement cleanup of temporary files** - Use `defer os.Remove()` to ensure temp files are cleaned up on any exit path.

9. **Document the matching algorithm** - Add a section to the README explaining how the fuzzy matching works, with examples of edge cases and tuning tips.

10. **Add CI/CD pipeline** - Set up GitHub Actions for:
    - Linting (golangci-lint)
    - Testing
    - Building binaries
    - Releasing via goreleaser

### Low Priority

11. **Refactor the matching algorithm** - Consider extracting it into a separate package or using an existing library like `github.com/knqyf263/go-fuzzyfinder` for better performance and maintainability.

12. **Add benchmark tests** - Measure performance of CSV parsing, matching, and API calls to identify bottlenecks.

13. **Create troubleshooting guide** - Document common issues (e.g., "playlists not updating", "matching failing") with solutions.

---

## Conclusion

SpotSonic is a well-architected project with clean separation of concerns and good use of Go idioms. The main areas for improvement are:

1. **Error handling** - More robust error propagation and recovery
2. **Security** - Better password handling, rate limiting, input validation
3. **Testing** - Add unit tests to prevent regressions
4. **Documentation** - Improve README with examples and troubleshooting

Addressing these issues will make SpotSonic more production-ready and easier to maintain long-term. The codebase is small enough that all recommendations can be implemented without significant refactoring.

---

*Audit performed on: 2026-05-07*  
*Auditor: Cline (AI Code Review Assistant)*