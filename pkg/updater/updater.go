package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/minio/selfupdate"
	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/utils"
)

// httpClient is a shared HTTP client used for release checks and downloads.
// The Timeout value applies to the entire HTTP request: dialing, TLS
// handshake, redirects, and reading the response body. It is NOT only
// a connection (dial) timeout. To control lower-level timeouts (dial,
// TLS handshake, response header wait), supply a custom Transport with
// an appropriately configured net.Dialer.
var httpClient = &http.Client{Timeout: 2 * time.Minute}

func getWithRetry(rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return utils.DoRequestWithRetry(httpClient, req)
}

// DownloadAndExtractRelease resolves a GitHub release and downloads its verified
// archive into a temporary directory. It returns the
// extraction directory on success. If releaseURL is empty, the latest
// release of the current project is used. platform/arch can be used to
// select the correct asset (e.g. "linux", "amd64").
// Direct archive URLs are rejected because release checksum metadata is required.
func DownloadAndExtractRelease(releaseURL, platform, arch string) (string, error) {
	assetURL, checksum, err := findAssetInfo(releaseURL, platform, arch)
	if err != nil {
		return "", err
	}

	// Download asset to temp file. Use the asset URL extension so
	// extractArchive can detect the archive format (zip/tar.gz/tar).
	tmpPattern := "picoclaw-release-*"
	if u, perr := url.Parse(assetURL); perr == nil {
		base := filepath.Base(u.Path)
		lbase := strings.ToLower(base)
		switch {
		case strings.HasSuffix(lbase, ".zip"):
			tmpPattern += ".zip"
		case strings.HasSuffix(lbase, ".tar.gz") || strings.HasSuffix(lbase, ".tgz"):
			tmpPattern += ".tar.gz"
		case strings.HasSuffix(lbase, ".tar"):
			tmpPattern += ".tar"
		default:
			tmpPattern += ".archive"
		}
	} else {
		tmpPattern += ".archive"
	}

	tmpFile, err := os.CreateTemp("", tmpPattern)
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	defer tmpFile.Close()

	resp, err := getWithRetry(assetURL)
	if err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to download asset: status %d", resp.StatusCode)
	}

	// Stream download while computing SHA256 to avoid a second download.
	// Also show a simple progress line to stderr so users see activity.
	h := sha256.New()
	pw := &progressWriter{total: resp.ContentLength}
	mw := io.MultiWriter(tmpFile, h, pw)
	if _, err = io.Copy(mw, resp.Body); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	// ensure final progress line ends with newline
	pw.Finish()

	// verify checksum if available
	if checksum != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(got, checksum) {
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("checksum mismatch: got %s expected %s", got, checksum)
		}
	}

	// Extract
	destDir, err := os.MkdirTemp("", "picoclaw-extract-*")
	if err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	if err := extractArchive(tmpPath, destDir); err != nil {
		os.Remove(tmpPath)
		os.RemoveAll(destDir)
		return "", err
	}

	// cleanup archive file; keep extracted contents
	_ = os.Remove(tmpPath)
	return destDir, nil
}

// UpdateSelfFromRelease downloads the release matching the given parameters,
// extracts it and applies the binary named programName to update the
// currently running executable using minio/selfupdate.
// If releaseURL is empty, the latest release is used. If platform or arch
// is empty, runtime values are used.
func UpdateSelfFromRelease(releaseURL, platform, arch, programName string) error {
	if platform == "" {
		platform = runtime.GOOS
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	if !strings.EqualFold(platform, runtime.GOOS) || !sameArchitecture(arch, runtime.GOARCH) {
		return fmt.Errorf(
			"cannot self-update %s/%s with a %s/%s binary; use --check to inspect another target",
			runtime.GOOS,
			runtime.GOARCH,
			platform,
			arch,
		)
	}

	dir, err := DownloadAndExtractRelease(releaseURL, platform, arch)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	binPath, err := findBinaryInDir(dir, programName)
	if err != nil {
		return err
	}

	// ensure executable bit on non-windows
	if runtime.GOOS != "windows" {
		_ = os.Chmod(binPath, 0o755)
	}

	f, err := os.Open(binPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Backup current executable so we can roll back if needed.
	var opts selfupdate.Options
	if exePath, err := os.Executable(); err == nil {
		opts.OldSavePath = exePath + ".old"
	}

	if err := selfupdate.Apply(f, opts); err != nil {
		return fmt.Errorf("apply update: %w", err)
	}

	return nil
}

// UpdateSelf updates the running executable by fetching the latest release
// and applying the binary matching programName.
func UpdateSelf(programName string) error {
	// By default, select the latest stable release when no explicit
	// release URL is provided. Use --nightly or a custom URL to override.
	return UpdateSelfFromRelease("", runtime.GOOS, runtime.GOARCH, programName)
}

// DefaultRepository is the release source for this fork. Never fall back to upstream.
const DefaultRepository = "loveuer/picoclaw"

// GetReleaseAPIURL returns the latest stable release API URL for another picoclaw fork.
func GetReleaseAPIURL(owner string) string {
	return releaseAPIURL(owner+"/picoclaw", false)
}

// GetProdReleaseAPIURL returns this fork's latest stable release API URL.
func GetProdReleaseAPIURL() string {
	return releaseAPIURL(DefaultRepository, false)
}

// GetReleaseTagAPIURL returns the release API URL for a tag on another picoclaw fork.
func GetReleaseTagAPIURL(owner, tag string) string {
	return "https://api.github.com/repos/" + owner + "/picoclaw/releases/tags/" + url.PathEscape(tag)
}

// GetNightlyReleaseAPIURL returns this fork's nightly release API URL.
func GetNightlyReleaseAPIURL() string {
	return releaseAPIURL(DefaultRepository, true)
}

func releaseAPIURL(repository string, nightly bool) string {
	suffix := "/latest"
	if nightly {
		suffix = "/tags/nightly"
	}
	return "https://api.github.com/repos/" + repository + "/releases" + suffix
}

func validRepository(repository string) bool {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "." || part == ".." || !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(part) {
			return false
		}
	}
	return true
}

// findAssetInfo resolves the appropriate asset URL for the given release
// selector. An empty selector uses this fork; direct archives are rejected
// because release checksum metadata is required.
func findAssetInfo(releaseURL, platform, arch string) (string, string, error) {
	// returns (assetURL, sha256ChecksumHex, error)
	if looksLikeDirectAssetURL(releaseURL) {
		return "", "", fmt.Errorf(
			"direct archive URLs are unsupported: use the release page or API URL to obtain a verified checksum for %s",
			releaseURL,
		)
	}

	apiURL, err := buildReleaseAPIURL(releaseURL)
	if err != nil {
		return "", "", err
	}

	resp, err := getWithRetry(apiURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", "", fmt.Errorf(
			"release not found at %s: publish the selected release and matching archives first; no alternate source was queried",
			apiURL,
		)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to query releases at %s: status %d", apiURL, resp.StatusCode)
	}

	var data struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Digest             string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", err
	}

	// Selection order: platform -> arch -> extension.
	platformLower := strings.ToLower(platform)
	archLower := strings.ToLower(arch)

	isZip := func(name string) bool {
		return strings.HasSuffix(name, ".zip")
	}
	isTarGz := func(name string) bool {
		return strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz")
	}
	isTar := func(name string) bool { return strings.HasSuffix(name, ".tar") }

	// collect indices of assets that contain platform (if provided)
	var platformIdx []int
	for i, a := range data.Assets {
		n := strings.ToLower(a.Name)
		if !(isZip(n) || isTarGz(n) || isTar(n)) {
			continue
		}
		if platform == "" || assetHasToken(n, platformLower) {
			platformIdx = append(platformIdx, i)
		}
	}

	pickBest := func(idxs []int) (string, int, bool) {
		if len(idxs) == 0 {
			return "", -1, false
		}
		// prefer arch matches within idxs; if arch was specified but
		// no arch match exists among idxs, treat as no candidate.
		var archIdx []int
		if arch != "" {
			aliases := archAliases(archLower)
			for _, i := range idxs {
				n := strings.ToLower(data.Assets[i].Name)
				for _, ali := range aliases {
					if assetHasToken(n, ali) {
						archIdx = append(archIdx, i)
						break
					}
				}
			}
			if len(archIdx) == 0 {
				return "", -1, false
			}
		}
		candidates := archIdx
		if len(candidates) == 0 {
			candidates = idxs
		}

		// extension preference
		if platformLower == "windows" {
			// prefer .zip only
			for _, i := range candidates {
				if isZip(strings.ToLower(data.Assets[i].Name)) {
					return data.Assets[i].BrowserDownloadURL, i, true
				}
			}
			// if no zip found, fallthrough to first candidate
			return data.Assets[candidates[0]].BrowserDownloadURL, candidates[0], true
		}

		// non-windows: prefer tar.gz/tgz, then tar, then zip
		for _, i := range candidates {
			if isTarGz(strings.ToLower(data.Assets[i].Name)) {
				return data.Assets[i].BrowserDownloadURL, i, true
			}
		}
		for _, i := range candidates {
			if isTar(strings.ToLower(data.Assets[i].Name)) {
				return data.Assets[i].BrowserDownloadURL, i, true
			}
		}
		for _, i := range candidates {
			if isZip(strings.ToLower(data.Assets[i].Name)) {
				return data.Assets[i].BrowserDownloadURL, i, true
			}
		}
		// fallback to first candidate
		return data.Assets[candidates[0]].BrowserDownloadURL, candidates[0], true
	}

	// Try platform matches first
	if url, idx, ok := pickBest(platformIdx); ok {
		// attempt to find checksum: prefer asset digest from API if present
		if d := strings.TrimSpace(data.Assets[idx].Digest); d != "" {
			dLower := strings.ToLower(d)
			if strings.HasPrefix(dLower, "sha256:") {
				dLower = strings.TrimPrefix(dLower, "sha256:")
			}
			// If digest already looks like a 64-hex, return it
			if ok, _ := regexp.MatchString("(?i)^[a-f0-9]{64}$", dLower); ok {
				return url, dLower, nil
			}
		}
		// Look for checksum assets and verify by computing the asset's sha256.
		for j, a := range data.Assets {
			n := strings.ToLower(a.Name)
			if strings.Contains(n, "sha256") ||
				strings.Contains(n, "sha256sum") ||
				strings.Contains(n, "checksums") ||
				strings.HasSuffix(n, ".sha256") ||
				strings.HasSuffix(n, ".sha256sum") {
				resp2, err := getWithRetry(data.Assets[j].BrowserDownloadURL)
				if err != nil {
					continue
				}
				bs, err := io.ReadAll(resp2.Body)
				_ = resp2.Body.Close()
				if err != nil || resp2.StatusCode != http.StatusOK {
					continue
				}
				if h, ok := findHashInChecksumContent(bs, url); ok {
					return url, h, nil
				}
			}
		}
		// No checksum found for the selected platform asset -> error
		return "", "", fmt.Errorf("no checksum found for asset %s", url)
	}

	// No platform match — require explicit platform+arch; fail fast.
	return "", "", fmt.Errorf("no release asset matching platform %q and arch %q", platform, arch)
}

func looksLikeDirectAssetURL(u string) bool {
	if u == "" {
		return false
	}
	lower := strings.ToLower(u)
	if strings.HasSuffix(lower, ".zip") ||
		strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tgz") ||
		strings.HasSuffix(lower, ".tar") {
		return true
	}
	if strings.Contains(lower, "/releases/download/") {
		return true
	}
	return false
}

func buildReleaseAPIURL(releaseURL string) (string, error) {
	if releaseURL == "" {
		return GetProdReleaseAPIURL(), nil
	}
	invalid := func() (string, error) {
		return "", fmt.Errorf(
			"invalid release URL %q: use a GitHub repository, release page, or Releases API URL",
			releaseURL,
		)
	}
	u, err := url.Parse(releaseURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" ||
		u.Fragment != "" {
		return invalid()
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if u.Host == "github.com" {
		if len(parts) < 2 || !validRepository(parts[0]+"/"+parts[1]) {
			return invalid()
		}
		repository := parts[0] + "/" + parts[1]
		switch {
		case len(parts) == 2,
			len(parts) == 3 && parts[2] == "releases",
			len(parts) == 4 && parts[2] == "releases" && parts[3] == "latest":
			return releaseAPIURL(repository, false), nil
		case len(parts) == 5 && parts[2] == "releases" && parts[3] == "tag" && parts[4] != "":
			return "https://api.github.com/repos/" + repository + "/releases/tags/" + parts[4], nil
		default:
			return invalid()
		}
	}
	// Explicit API endpoints also support compatible proxies and local test servers.
	if (len(parts) == 5 && parts[4] == "latest" || len(parts) == 6 && parts[4] == "tags" && parts[5] != "") &&
		parts[0] == "repos" &&
		parts[3] == "releases" &&
		validRepository(parts[1]+"/"+parts[2]) {
		return u.String(), nil
	}
	return invalid()
}

// Match complete name components, so arm cannot accidentally select arm64,
// or 386 (x86) select x86_64. Normalize the compound x86_64 spelling first.
func assetHasToken(name, token string) bool {
	normalize := func(value string) string {
		return strings.NewReplacer("x86_64", "amd64", "x86-64", "amd64").Replace(strings.ToLower(value))
	}
	token = normalize(token)
	for _, part := range strings.FieldsFunc(normalize(name), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if part == token {
			return true
		}
	}
	return false
}

func sameArchitecture(a, b string) bool {
	for _, left := range archAliases(strings.ToLower(a)) {
		for _, right := range archAliases(strings.ToLower(b)) {
			if left == right {
				return true
			}
		}
	}
	return false
}

// NOTE: helper functions to compute SHA256 from URL/path were removed
// after refactoring to stream the download and verify the checksum
// during the single download to avoid double-transfer.

// findHashInChecksumContent attempts to locate a 64-hex SHA256 in the
// checksum file content that corresponds to assetURL. It returns the
// found hash (lowercase) and true, or "", false if not found.
func findHashInChecksumContent(bs []byte, assetURL string) (string, bool) {
	s := strings.ToLower(string(bs))
	var assetBase string
	if u, err := url.Parse(assetURL); err == nil {
		assetBase = strings.ToLower(filepath.Base(u.Path))
	} else {
		assetBase = strings.ToLower(filepath.Base(assetURL))
	}
	re := regexp.MustCompile(`(?i)\b([a-f0-9]{64})\b`)
	// prefer a line containing the asset filename
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, assetBase) {
			if m := re.FindString(line); m != "" {
				return m, true
			}
		}
	}
	// fallback: if there's exactly one unique 64-hex value, return it
	matches := re.FindAllString(s, -1)
	uniq := map[string]struct{}{}
	for _, m := range matches {
		uniq[m] = struct{}{}
	}
	if len(uniq) == 1 {
		for k := range uniq {
			return k, true
		}
	}
	return "", false
}

// progressWriter implements io.Writer and prints a simple progress
// line to stderr while bytes are written. It is intended to be used
// as one writer in an io.MultiWriter so we can stream-to-disk, compute
// the sha256, and update the progress display in a single pass.
type progressWriter struct {
	total   int64
	written int64
	last    time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	now := time.Now()
	if pw.last.IsZero() || now.Sub(pw.last) >= 200*time.Millisecond || (pw.total > 0 && pw.written == pw.total) {
		pw.print()
		pw.last = now
	}
	return n, nil
}

func (pw *progressWriter) print() {
	if pw.total > 0 {
		pct := float64(pw.written) * 100.0 / float64(pw.total)
		fmt.Fprintf(os.Stderr, "\rDownloading: %s / %s (%.1f%%)", humanBytes(pw.written), humanBytes(pw.total), pct)
	} else {
		fmt.Fprintf(os.Stderr, "\rDownloading: %s", humanBytes(pw.written))
	}
}

func (pw *progressWriter) Finish() {
	pw.print()
	fmt.Fprintln(os.Stderr, "")
}

func humanBytes(n int64) string {
	f := float64(n)
	const (
		KB = 1024.0
		MB = KB * 1024.0
		GB = MB * 1024.0
	)
	switch {
	case f >= GB:
		return fmt.Sprintf("%.2f GB", f/GB)
	case f >= MB:
		return fmt.Sprintf("%.2f MB", f/MB)
	case f >= KB:
		return fmt.Sprintf("%.2f KB", f/KB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// archAliases returns common release filename variants for a Go architecture.
func archAliases(arch string) []string {
	a := strings.ToLower(arch)
	if syns, ok := archSynonyms[a]; ok {
		return syns
	}
	return []string{a}
}

var archSynonyms = map[string][]string{
	"amd64":   {"amd64", "x86_64", "x64"},
	"x86_64":  {"amd64", "x86_64", "x64"},
	"x64":     {"amd64", "x86_64", "x64"},
	"386":     {"386", "x86"},
	"x86":     {"386", "x86"},
	"arm64":   {"arm64", "aarch64"},
	"aarch64": {"arm64", "aarch64"},
	"arm":     {"arm", "armv6", "armv7"},
}

func extractArchive(archivePath, destDir string) error {
	lower := strings.ToLower(archivePath)
	if strings.HasSuffix(lower, ".zip") {
		return extractZip(archivePath, destDir)
	}
	// treat .tar.gz and .tgz as gzip+tar
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return extractTarGz(archivePath, destDir)
	}
	if strings.HasSuffix(lower, ".tar") {
		return extractTar(archivePath, destDir)
	}
	// fallback: try tar.gz
	return extractTarGz(archivePath, destDir)
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	destClean := filepath.Clean(destDir)
	for _, f := range r.File {
		target := filepath.Clean(filepath.Join(destClean, f.Name))
		if !strings.HasPrefix(target, destClean+string(os.PathSeparator)) && target != destClean {
			return fmt.Errorf("path traversal detected: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.FileInfo().Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.FileInfo().Mode())
		if err != nil {
			_ = rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = rc.Close()
			_ = out.Close()
			return err
		}
		if err := rc.Close(); err != nil {
			_ = out.Close()
			return fmt.Errorf("close zip entry reader: %w", err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("close extracted file %q: %w", target, err)
		}
	}
	return nil
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	return extractTarFromReader(tr, destDir)
}

func extractTar(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	return extractTarFromReader(tr, destDir)
}

// extractTarFromReader contains logic common to extracting entries from a
// tar.Reader and is used by both extractTarGz and extractTar to avoid
// duplicated code (golangci-lint: dupl).
func extractTarFromReader(tr *tar.Reader, destDir string) error {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Clean(filepath.Join(filepath.Clean(destDir), hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) &&
			target != filepath.Clean(destDir) {
			return fmt.Errorf("path traversal detected: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("close extracted file %q: %w", target, err)
			}
		}
	}
	return nil
}

func findBinaryInDir(dir, programName string) (string, error) {
	names := []string{programName}
	if programName == "picoclaw" {
		names = append(names, "picoclaw-beta")
	}
	var wanted []string
	for _, name := range names {
		if runtime.GOOS == "windows" {
			wanted = append(wanted, name+".exe", name)
		} else {
			wanted = append(wanted, name, name+".exe")
		}
	}

	for _, wantedName := range wanted {
		var found string
		if err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || found != "" {
				return err
			}
			if d.IsDir() {
				return nil
			}
			base := filepath.Base(p)
			if base == wantedName {
				found = p
				return io.EOF // use EOF to stop walking early
			}
			return nil
		}); err != nil && err != io.EOF {
			return "", err
		}
		if found != "" {
			return found, nil
		}
	}
	return "", fmt.Errorf("binary %q not found in archive", programName)
}

// NewUpdateCommand returns a cobra command that triggers UpdateSelfFromRelease.
func NewUpdateCommand(binaryName string) *cobra.Command {
	var urlStr, repository, platform, arch string
	var nightly, check bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check and apply updates from this fork's GitHub releases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !validRepository(repository) {
				return fmt.Errorf("invalid repository %q: expected owner/repo", repository)
			}
			selector := urlStr
			if selector == "" {
				selector = releaseAPIURL(repository, nightly)
			}
			source, err := buildReleaseAPIURL(selector)
			if err != nil {
				return err
			}
			targetPlatform, targetArch := platform, arch
			if targetPlatform == "" {
				targetPlatform = runtime.GOOS
			}
			if targetArch == "" {
				targetArch = runtime.GOARCH
			}
			fmt.Fprintf(
				cmd.OutOrStdout(),
				"Current version: %s\nRelease source: %s\nTarget: %s/%s\n",
				config.FormatVersion(),
				source,
				targetPlatform,
				targetArch,
			)
			if check {
				asset, checksum, err := findAssetInfo(source, targetPlatform, targetArch)
				if err != nil {
					return err
				}
				fmt.Fprintf(
					cmd.OutOrStdout(),
					"Release asset: %s\nSHA256: %s\nCheck complete; no files changed.\n",
					asset,
					checksum,
				)
				return nil
			}
			if err := UpdateSelfFromRelease(source, targetPlatform, targetArch, binaryName); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Update applied to the current executable; restart to use the new version.")
			return nil
		},
	}
	cmd.Flags().StringVar(&repository, "repo", DefaultRepository, "GitHub release repository (owner/repo)")
	cmd.Flags().BoolVar(&nightly, "nightly", false, "Use the selected repository's nightly release")
	cmd.Flags().
		BoolVar(&check, "check", false, "Check release metadata without downloading the binary or replacing files")
	cmd.Flags().
		StringVarP(&urlStr, "url", "u", "", "GitHub repository, release page, or Releases API URL (archive URLs are unsupported)")
	cmd.Flags().StringVar(&platform, "platform", "", "Target platform (cross-platform queries require --check)")
	cmd.Flags().StringVar(&arch, "arch", "", "Target architecture (cross-architecture queries require --check)")
	cmd.MarkFlagsMutuallyExclusive("url", "repo")
	cmd.MarkFlagsMutuallyExclusive("url", "nightly")
	return cmd
}
