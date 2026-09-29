package rules

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/nasserghiasi/ng-dev-tools/internal/cleanup"
)

// dockerTimeout bounds how long we wait on a Docker daemon that may still be starting.
const dockerTimeout = 15 * time.Second

type dockerTarget struct {
	title   string
	safety  cleanup.Safety
	command []string
}

var dockerTargets = map[string]dockerTarget{
	"Build Cache": {"Docker build cache", cleanup.SafetySafe, []string{"docker", "builder", "prune", "--all", "--force"}},
	"Images":      {"Docker unused images", cleanup.SafetyReview, []string{"docker", "image", "prune", "--all", "--force"}},
	"Containers":  {"Docker stopped containers", cleanup.SafetyReview, []string{"docker", "container", "prune", "--force"}},
}

// dockerRule reports reclaimable Docker space. Volumes are left alone since they hold data.
type dockerRule struct{}

func (dockerRule) Name() string               { return "Docker" }
func (dockerRule) Category() cleanup.Category { return dev }

func (dockerRule) Scan(ctx context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	if !env.Runner.Available("docker") {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, dockerTimeout)
	defer cancel()
	out, err := env.Runner.Run(ctx, "docker", "system", "df", "--format", "{{json .}}")
	if err != nil {
		// A stopped daemon simply means there is nothing to report.
		return nil, nil
	}
	return parseDockerDF(out), nil
}

func parseDockerDF(out []byte) []cleanup.Item {
	var items []cleanup.Item
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		var row struct{ Type, Reclaimable string }
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			continue
		}
		target, ok := dockerTargets[row.Type]
		size := parseDockerSize(row.Reclaimable)
		if !ok || size == 0 {
			continue
		}
		items = append(items, cleanup.Item{
			Title:         target.title,
			Detail:        strings.Join(target.command, " "),
			Category:      dev,
			Safety:        target.safety,
			RemoveCommand: target.command,
			Size:          size,
		})
	}
	return items
}

// parseDockerSize reads values like "1.2GB (40%)".
func parseDockerSize(value string) int64 {
	amount, _, _ := strings.Cut(value, " ")
	size, err := humanize.ParseBytes(amount)
	if err != nil {
		return 0
	}
	return int64(size)
}
