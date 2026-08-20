package builder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Version is one published branch of a service.
type Version struct {
	Name string `json:"name"`
	// Protected marks long-lived branches (main/master/develop). They are
	// always kept and never evicted by retention.
	Protected bool      `json:"protected"`
	BuiltAt   time.Time `json:"builtAt"`
}

// VersionList is what the flyout fetches as /<service>/versions.json.
type VersionList struct {
	Service  string    `json:"service"`
	Versions []Version `json:"versions"`
}

func (b *Builder) manifestPath(service string) string {
	return filepath.Join(b.cfg.MetaDir, service+".json")
}

func (b *Builder) loadVersions(service string) []Version {
	data, err := os.ReadFile(b.manifestPath(service))
	if err != nil {
		return nil
	}
	var vl VersionList
	if err := json.Unmarshal(data, &vl); err != nil {
		return nil
	}
	return vl.Versions
}

// recordVersion adds or refreshes a version, then applies retention. It
// returns the versions that were evicted so their built output can be removed.
func (b *Builder) recordVersion(service, branch string, isProtected bool) ([]Version, error) {
	versions := b.loadVersions(service)

	found := false
	for i := range versions {
		if versions[i].Name == branch {
			// Long-lived branches update in place rather than accumulating.
			versions[i].BuiltAt = time.Now().UTC()
			versions[i].Protected = isProtected
			found = true
			break
		}
	}
	if !found {
		versions = append(versions, Version{
			Name:      branch,
			Protected: isProtected,
			BuiltAt:   time.Now().UTC(),
		})
	}

	kept, evicted := applyRetention(versions, b.cfg.MaxVersions)

	if err := os.MkdirAll(b.cfg.MetaDir, 0o755); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(VersionList{Service: service, Versions: kept}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(b.manifestPath(service), out, 0o644); err != nil {
		return nil, err
	}

	// The site copy is what the flyout reads at runtime.
	siteCopy := filepath.Join(b.cfg.SiteDir, service, "versions.json")
	if err := os.MkdirAll(filepath.Dir(siteCopy), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(siteCopy, out, 0o644); err != nil {
		return nil, err
	}

	return evicted, nil
}

// applyRetention keeps every protected version plus the most recently built
// unprotected ones, up to maxUnprotected.
func applyRetention(versions []Version, maxUnprotected int) (kept, evicted []Version) {
	var prot, unprot []Version
	for _, v := range versions {
		if v.Protected {
			prot = append(prot, v)
		} else {
			unprot = append(unprot, v)
		}
	}

	sort.Slice(prot, func(i, j int) bool { return prot[i].Name < prot[j].Name })
	// Newest first, so the tail beyond the cap is what gets evicted.
	sort.Slice(unprot, func(i, j int) bool { return unprot[i].BuiltAt.After(unprot[j].BuiltAt) })

	if maxUnprotected >= 0 && len(unprot) > maxUnprotected {
		evicted = append(evicted, unprot[maxUnprotected:]...)
		unprot = unprot[:maxUnprotected]
	}

	kept = append(kept, prot...)
	kept = append(kept, unprot...)
	return kept, evicted
}

// Services lists services that have at least one published version.
func (b *Builder) Services() []VersionList {
	entries, err := os.ReadDir(b.cfg.MetaDir)
	if err != nil {
		return nil
	}
	var out []VersionList
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		service := e.Name()[:len(e.Name())-len(".json")]
		versions := b.loadVersions(service)
		if len(versions) == 0 {
			continue
		}
		out = append(out, VersionList{Service: service, Versions: versions})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	return out
}
