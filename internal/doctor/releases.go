package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

const (
	nodeReleasesURL = "https://nodejs.org/dist/index.json"
	goReleasesURL   = "https://go.dev/dl/?mode=json"
	npmRegistryURL  = "https://registry.npmjs.org"
)

// ErrOffline is returned by release lookups when doctor runs with --offline.
var ErrOffline = errors.New("offline")

// Releases looks up the newest published versions of tools.
type Releases interface {
	NodeLTS(ctx context.Context) (Version, error)
	Go(ctx context.Context) (Version, error)
	Npm(ctx context.Context, pkg string) (Version, error)
}

// WebReleases asks the projects' own release feeds.
type WebReleases struct {
	Client *http.Client
}

func (w WebReleases) NodeLTS(ctx context.Context) (Version, error) {
	var releases []struct {
		Version string          `json:"version"`
		LTS     json.RawMessage `json:"lts"`
	}
	if err := w.getJSON(ctx, nodeReleasesURL, &releases); err != nil {
		return Version{}, err
	}
	for _, r := range releases {
		if string(r.LTS) != "false" {
			return parseRelease(r.Version)
		}
	}
	return Version{}, errors.New("no LTS release found")
}

func (w WebReleases) Go(ctx context.Context) (Version, error) {
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := w.getJSON(ctx, goReleasesURL, &releases); err != nil {
		return Version{}, err
	}
	for _, r := range releases {
		if r.Stable {
			return parseRelease(r.Version)
		}
	}
	return Version{}, errors.New("no stable Go release found")
}

func (w WebReleases) Npm(ctx context.Context, pkg string) (Version, error) {
	var latest struct {
		Version string `json:"version"`
	}
	if err := w.getJSON(ctx, npmRegistryURL+"/"+pkg+"/latest", &latest); err != nil {
		return Version{}, err
	}
	return parseRelease(latest.Version)
}

func (w WebReleases) getJSON(ctx context.Context, url string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := w.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

func parseRelease(text string) (Version, error) {
	v, ok := ParseVersion(text)
	if !ok {
		return Version{}, fmt.Errorf("unrecognised release %q", text)
	}
	return v, nil
}

// OfflineReleases answers every lookup with ErrOffline.
type OfflineReleases struct{}

func (OfflineReleases) NodeLTS(context.Context) (Version, error) { return Version{}, ErrOffline }

func (OfflineReleases) Go(context.Context) (Version, error) { return Version{}, ErrOffline }

func (OfflineReleases) Npm(context.Context, string) (Version, error) { return Version{}, ErrOffline }
