package updater

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForkReleaseURLs(t *testing.T) {
	require.Equal(t, "https://api.github.com/repos/loveuer/picoclaw/releases/latest", GetProdReleaseAPIURL())
	require.Equal(t, "https://api.github.com/repos/loveuer/picoclaw/releases/tags/nightly", GetNightlyReleaseAPIURL())
	tests := []struct{ input, want string }{
		{"", GetProdReleaseAPIURL()},
		{"https://github.com/another/fork", "https://api.github.com/repos/another/fork/releases/latest"},
		{
			"https://github.com/another/fork/releases/latest",
			"https://api.github.com/repos/another/fork/releases/latest",
		},
		{
			"https://github.com/another/fork/releases/tag/v1.2.3",
			"https://api.github.com/repos/another/fork/releases/tags/v1.2.3",
		},
		{
			"https://github.com/another/fork/releases/tag/beta%2Fv1",
			"https://api.github.com/repos/another/fork/releases/tags/beta%2Fv1",
		},
		{
			"https://api.github.com/repos/another/fork/releases/tags/nightly",
			"https://api.github.com/repos/another/fork/releases/tags/nightly",
		},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := buildReleaseAPIURL(test.input)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
	for _, input := range []string{"not-a-url", "https://api.github.com.evil.example/invalid", "https://example.com/not-a-release", "https://github.com/another", "https://github.com/another/fork/issues", "file:///repos/a/b/releases/latest", "https://github.com/a/b?x=1"} {
		_, err := buildReleaseAPIURL(input)
		require.Error(t, err, input)
	}
}

type releaseRoundTripper func(*http.Request) (*http.Response, error)

func (f releaseRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func routeReleaseRequests(t *testing.T, server *httptest.Server) {
	t.Helper()
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	transport := server.Client().Transport
	withTestHTTPClient(t, &http.Client{Transport: releaseRoundTripper(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		clone.URL.Scheme = endpoint.Scheme
		clone.URL.Host = endpoint.Host
		return transport.RoundTrip(clone)
	})})
}

func TestUpdateCommandSelectsSourceWithoutDownloading(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
	}{
		{"default fork", nil, "/repos/loveuer/picoclaw/releases/latest"},
		{"nightly", []string{"--nightly"}, "/repos/loveuer/picoclaw/releases/tags/nightly"},
		{"other fork", []string{"--repo", "another/fork", "--nightly"}, "/repos/another/fork/releases/tags/nightly"},
		{
			"pinned release",
			[]string{"--url", "https://github.com/another/fork/releases/tag/v1.2.3"},
			"/repos/another/fork/releases/tags/v1.2.3",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				if r.URL.Path != test.path {
					http.NotFound(w, r)
					return
				}
				writeReleasePayload(
					w,
					testReleasePayload{
						TagName: "v1.2.3",
						Assets: []testReleaseAsset{
							{
								Name:               "picoclaw_Linux_x86_64.tar.gz",
								BrowserDownloadURL: "https://example.com/picoclaw_Linux_x86_64.tar.gz",
								Digest:             "sha256:" + strings.Repeat("a", 64),
							},
						},
					},
				)
			}))
			defer server.Close()
			routeReleaseRequests(t, server)
			command := NewUpdateCommand("picoclaw")
			var out bytes.Buffer
			command.SetOut(&out)
			command.SetErr(&out)
			command.SetArgs(append(test.args, "--check", "--platform", "linux", "--arch", "amd64"))
			require.NoError(t, command.Execute())
			require.Equal(t, []string{test.path}, requests)
			require.Contains(t, out.String(), "no files changed")
			require.Contains(t, out.String(), "SHA256: "+strings.Repeat("a", 64))
		})
	}
}

func TestUpdateCommandRejectsInvalidSourceBeforeNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected request:", r.URL)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	routeReleaseRequests(t, server)
	for _, args := range [][]string{
		{"--check", "--repo", "../bad"},
		{"--check", "--url", "https://example.com/wrong"},
		{"--check", "--url", "https://github.com/a/b", "--repo", "a/b"},
		{"--check", "--url", "https://github.com/a/b", "--nightly"},
		{"--check", "unexpected"},
	} {
		command := NewUpdateCommand("picoclaw")
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs(args)
		require.Error(t, command.Execute(), args)
	}
}

func TestMissingForkReleaseDoesNotFallback(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()
	routeReleaseRequests(t, server)
	_, _, err := findAssetInfo("", "linux", "amd64")
	require.ErrorContains(t, err, "release not found")
	require.ErrorContains(t, err, "loveuer/picoclaw")
	require.Equal(t, []string{"/repos/loveuer/picoclaw/releases/latest"}, requests)
}

func TestAssetSelectionRequiresExactArchitectureAndArchive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assets := make([]testReleaseAsset, 0, 5)
		for i, name := range []string{"picoclaw_linux_arm64.tar.gz", "picoclaw_linux_amd64.tar.gz", "picoclaw_linux_amd64.tar.gz.sha256", "picoclaw_linux_armv7.tar.gz", "picoclaw_linux_386.zip"} {
			assets = append(
				assets,
				testReleaseAsset{
					Name:               name,
					BrowserDownloadURL: "https://example.com/" + name,
					Digest:             "sha256:" + strings.Repeat(fmt.Sprint(i+1), 64),
				},
			)
		}
		writeReleasePayload(w, testReleasePayload{Assets: assets})
	}))
	defer server.Close()
	for _, test := range []struct{ arch, filename string }{
		{"arm", "picoclaw_linux_armv7.tar.gz"},
		{"amd64", "picoclaw_linux_amd64.tar.gz"},
		{"386", "picoclaw_linux_386.zip"},
	} {
		asset, _, err := findAssetInfo(server.URL+testReleaseAPIPath, "linux", test.arch)
		require.NoError(t, err)
		require.Equal(t, "https://example.com/"+test.filename, asset)
	}
}

func TestSelfUpdateRejectsDifferentTarget(t *testing.T) {
	other := "windows"
	if runtime.GOOS == other {
		other = "linux"
	}
	require.ErrorContains(t, UpdateSelfFromRelease("", other, runtime.GOARCH, "picoclaw"), "cannot self-update")
	require.ErrorContains(
		t,
		UpdateSelfFromRelease("", runtime.GOOS, "not-the-current-arch", "picoclaw"),
		"cannot self-update",
	)
}

func TestFindBinaryPrefersCanonicalAndAcceptsBeta(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "picoclaw-beta"), []byte("beta"), 0o755))
	path, err := findBinaryInDir(dir, "picoclaw")
	require.NoError(t, err)
	require.Equal(t, "picoclaw-beta", filepath.Base(path))
	nested := filepath.Join(dir, "nested")
	require.NoError(t, os.Mkdir(nested, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "picoclaw"), []byte("canonical"), 0o755))
	path, err = findBinaryInDir(dir, "picoclaw")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(nested, "picoclaw"), path)
}

// Only the isolated copy of this test executable can enter the replacement path.
func TestSelfUpdateChild(t *testing.T) {
	if os.Getenv("PICOCLAW_UPDATER_CHILD") != "1" {
		return
	}
	if err := UpdateSelfFromRelease(
		os.Getenv("PICOCLAW_UPDATER_TEST_RELEASE"),
		runtime.GOOS,
		runtime.GOARCH,
		"picoclaw",
	); err != nil {
		t.Fatal(err)
	}
}

func TestSelfUpdatePreservesInstalledNameAndChecksChecksum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a running executable cannot be replaced using the Unix test-copy approach")
	}
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			archive := buildTestTarGz(t, map[string]string{"picoclaw": "new fork binary payload"})
			sum := sha256.Sum256(archive)
			digest := hex.EncodeToString(sum[:])
			if !valid {
				digest = strings.Repeat("0", 64)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/asset.tar.gz" {
					_, _ = w.Write(archive)
					return
				}
				writeReleasePayload(
					w,
					testReleasePayload{
						Assets: []testReleaseAsset{
							{
								Name:               "picoclaw_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz",
								BrowserDownloadURL: "http://" + r.Host + "/asset.tar.gz",
								Digest:             "sha256:" + digest,
							},
						},
					},
				)
			}))
			defer server.Close()
			executable, err := os.Executable()
			require.NoError(t, err)
			original, err := os.ReadFile(executable)
			require.NoError(t, err)
			target := filepath.Join(t.TempDir(), "picoclaw-beta")
			require.NoError(t, os.WriteFile(target, original, 0o755))
			command := exec.Command(target, "-test.run=^TestSelfUpdateChild$")
			command.Env = append(
				os.Environ(),
				"PICOCLAW_UPDATER_CHILD=1",
				"PICOCLAW_UPDATER_TEST_RELEASE="+server.URL+testReleaseAPIPath,
			)
			output, err := command.CombinedOutput()
			content, readErr := os.ReadFile(target)
			require.NoError(t, readErr)
			if valid {
				require.NoError(t, err, string(output))
				require.Equal(t, "new fork binary payload", string(content))
				_, err = os.Stat(filepath.Join(filepath.Dir(target), "picoclaw"))
				require.True(t, os.IsNotExist(err))
			} else {
				require.Error(t, err)
				require.Contains(t, string(output), "checksum mismatch")
				require.Equal(t, original, content)
			}
		})
	}
}

func TestCheckRejectsMalformedDigestAndFailedChecksumResponse(t *testing.T) {
	for _, failedManifest := range []bool{false, true} {
		t.Run(fmt.Sprint(failedManifest), func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/checksums.txt" {
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, strings.Repeat("a", 64)+"  picoclaw_linux_amd64.tar.gz")
					return
				}
				assets := []testReleaseAsset{
					{
						Name:               "picoclaw_linux_amd64.tar.gz",
						BrowserDownloadURL: server.URL + "/picoclaw_linux_amd64.tar.gz",
						Digest:             "sha256:not-a-valid-digest",
					},
				}
				if failedManifest {
					assets = append(
						assets,
						testReleaseAsset{Name: "checksums.txt", BrowserDownloadURL: server.URL + "/checksums.txt"},
					)
				}
				writeReleasePayload(w, testReleasePayload{Assets: assets})
			}))
			defer server.Close()
			_, _, err := findAssetInfo(server.URL+testReleaseAPIPath, "linux", "amd64")
			require.ErrorContains(t, err, "no checksum found")
		})
	}
}
