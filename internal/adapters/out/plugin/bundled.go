package pluginadapter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ero/internal/ports"
)

const (
	bundledSourcePrefix     = "bundled:"
	bundledCodexName        = "ero-plugin-codex"
	bundledCodexDirName     = "codex"
	bundledCodexSource      = bundledSourcePrefix + bundledCodexName
	bundledLifecycleMessage = "shipped plugin %q is included with ero and cannot be %s"
)

type bundledPluginRoot struct {
	Source string
	Path   string
}

func bundledPluginRoots() []bundledPluginRoot {
	if dirs := bundledPluginSearchDirsFromEnv(); len(dirs) > 0 {
		return discoverBundledPluginRoots(dirs)
	}
	return discoverDefaultBundledPluginRoots()
}

func discoverBundledPluginRoots(searchDirs []string) []bundledPluginRoot {
	roots := make([]bundledPluginRoot, 0)
	seen := map[string]struct{}{}
	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pluginDir := filepath.Join(dir, entry.Name())
			manifest, err := LoadManifest(pluginDir)
			if err != nil {
				continue
			}
			pathKey := cleanPathKey(pluginDir)
			if _, ok := seen[pathKey]; ok {
				continue
			}
			seen[pathKey] = struct{}{}
			roots = append(roots, bundledPluginRoot{Source: bundledSource(manifest.Name), Path: pluginDir})
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Source < roots[j].Source })
	return roots
}

func discoverDefaultBundledPluginRoots() []bundledPluginRoot {
	return discoverBundledPluginRoots(defaultBundledPluginBaseDirs())
}

func bundledPluginSearchDirsFromEnv() []string {
	dirs := make([]string, 0, 4)
	for _, dir := range filepath.SplitList(os.Getenv("ERO_BUNDLED_PLUGIN_DIRS")) {
		if strings.TrimSpace(dir) != "" {
			dirs = append(dirs, filepath.Clean(dir))
		}
	}
	return uniqueCleanPaths(dirs)
}

func defaultBundledPluginBaseDirs() []string {
	dirs := make([]string, 0, 4)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		dirs = append(dirs,
			filepath.Join(exeDir, "plugins"),
			filepath.Join(exeDir, "..", "lib", "ero", "plugins"),
		)
	}
	if devPlugins := findDevBundledPluginDir(); devPlugins != "" {
		dirs = append(dirs, devPlugins)
	}
	return uniqueCleanPaths(dirs)
}

func findDevBundledPluginDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "plugins", bundledCodexDirName)
		if manifest, err := LoadManifest(candidate); err == nil && strings.EqualFold(strings.TrimSpace(manifest.Name), bundledCodexName) {
			return filepath.Dir(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
	}
}

func uniqueCleanPaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		key := cleanPathKey(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, path)
	}
	return out
}

func cleanPathKey(path string) string {
	path = filepath.Clean(path)
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func bundledSource(id string) string {
	return bundledSourcePrefix + id
}

func isBundledSource(source string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), bundledSourcePrefix)
}

func bundledPlugins() []ports.PluginDescriptor {
	roots := bundledPluginRoots()
	descriptors := make([]ports.PluginDescriptor, 0, len(roots))
	for _, root := range roots {
		manifest, err := LoadManifest(root.Path)
		if err != nil {
			continue
		}
		contributions := make([]ports.PluginContribution, 0, len(manifest.Contributions))
		for _, contribution := range manifest.Contributions {
			contributions = append(contributions, ports.PluginContribution{Type: contribution.Type, ID: contribution.ID, Label: contribution.Label})
		}
		descriptors = append(descriptors, ports.PluginDescriptor{
			Name:          manifest.Name,
			Version:       manifest.Version,
			Source:        root.Source,
			Path:          root.Path,
			Bundled:       true,
			Contributions: contributions,
		})
	}
	return descriptors
}

func bundledInstalledPlugins() []ports.InstalledPlugin {
	descriptors := bundledPlugins()
	installed := make([]ports.InstalledPlugin, 0, len(descriptors))
	for _, descriptor := range descriptors {
		contributions := make([]string, 0, len(descriptor.Contributions))
		for _, contribution := range descriptor.Contributions {
			contributions = append(contributions, contribution.Type+":"+contribution.ID)
		}
		installed = append(installed, ports.InstalledPlugin{
			Name:          descriptor.Name,
			Version:       descriptor.Version,
			Source:        descriptor.Source,
			Path:          descriptor.Path,
			Bundled:       true,
			Contributions: contributions,
		})
	}
	return installed
}

func bundledDescriptorForInput(input string) (ports.PluginDescriptor, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ports.PluginDescriptor{}, false
	}
	for _, descriptor := range bundledPlugins() {
		if bundledDescriptorMatchesInput(descriptor, input) {
			return descriptor, true
		}
	}
	return ports.PluginDescriptor{}, false
}

func bundledDescriptorMatchesInput(descriptor ports.PluginDescriptor, input string) bool {
	candidates := []string{descriptor.Source, descriptor.Name}
	for _, contribution := range descriptor.Contributions {
		candidates = append(candidates, contribution.ID, contribution.Label)
	}
	for _, candidate := range candidates {
		if candidate != "" && strings.EqualFold(strings.TrimSpace(candidate), input) {
			return true
		}
	}
	return false
}

func bundledLifecycleError(action, source string) error {
	return fmt.Errorf(bundledLifecycleMessage, source, action)
}

func bundledUpdateResults(source string) []ports.PluginUpdateResult {
	results := []ports.PluginUpdateResult{}
	if source != "" {
		if descriptor, ok := bundledDescriptorForInput(source); ok {
			return []ports.PluginUpdateResult{{
				Source:  descriptor.Source,
				Name:    descriptor.Name,
				Message: "shipped plugin is included with ero and cannot be updated by plugin update",
			}}
		}
		if isBundledSource(source) {
			return []ports.PluginUpdateResult{{
				Source:  source,
				Message: "shipped plugin is included with ero and cannot be updated by plugin update",
			}}
		}
		return results
	}
	for _, descriptor := range bundledPlugins() {
		results = append(results, ports.PluginUpdateResult{
			Source:  descriptor.Source,
			Name:    descriptor.Name,
			Message: "shipped plugin is included with ero and cannot be updated by plugin update",
		})
	}
	return results
}
