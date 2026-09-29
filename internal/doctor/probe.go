package doctor

import (
	"context"
	"net"
	"net/http"
	"time"

	"golang.org/x/sys/unix"
)

// Probe reads facts that do not come from running a command.
type Probe interface {
	DiskFree(path string) (uint64, error)
	OpenFilesLimit() (uint64, error)
	LookupHost(ctx context.Context, host string) error
	// Reach returns how long an HTTPS request to url took to get any response.
	Reach(ctx context.Context, url string) (time.Duration, error)
}

// SystemProbe reads the real machine.
type SystemProbe struct {
	Client *http.Client
}

func (SystemProbe) DiskFree(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func (SystemProbe) OpenFilesLimit() (uint64, error) {
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		return 0, err
	}
	return limit.Cur, nil
}

func (SystemProbe) LookupHost(ctx context.Context, host string) error {
	_, err := net.DefaultResolver.LookupHost(ctx, host)
	return err
}

func (p SystemProbe) Reach(ctx context.Context, url string) (time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := p.Client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return time.Since(start), nil
}
