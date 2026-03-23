package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kustomization represents a minimal kustomization.yaml.
type Kustomization struct {
	Resources []string         `yaml:"resources"`
	Images    []KustomizeImage `yaml:"images,omitempty"`
}

// KustomizeImage represents an image override in kustomization.yaml.
type KustomizeImage struct {
	Name    string `yaml:"name"`
	NewName string `yaml:"newName,omitempty"`
	NewTag  string `yaml:"newTag,omitempty"`
	Digest  string `yaml:"digest,omitempty"`
}

// parseKustomization parses a kustomization.yaml file.
func parseKustomization(data []byte) (*Kustomization, error) {
	var k Kustomization
	if err := yaml.Unmarshal(data, &k); err != nil {
		return nil, fmt.Errorf("parsing kustomization.yaml: %w", err)
	}
	return &k, nil
}

// resolveKustomize reads a kustomization.yaml and all referenced resources,
// following nested kustomization directories recursively. Returns concatenated YAML.
func resolveKustomize(basePath string) ([]byte, *Kustomization, error) {
	kustomizationPath := filepath.Join(basePath, "kustomization.yaml")
	data, err := os.ReadFile(kustomizationPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", kustomizationPath, err)
	}

	k, err := parseKustomization(data)
	if err != nil {
		return nil, nil, err
	}

	var allManifests []byte
	for _, res := range k.Resources {
		resPath := filepath.Join(basePath, res)
		info, err := os.Stat(resPath)
		if err != nil {
			return nil, nil, fmt.Errorf("resource %q: %w", res, err)
		}

		if info.IsDir() {
			// Nested kustomization directory
			nested, _, err := resolveKustomize(resPath)
			if err != nil {
				return nil, nil, fmt.Errorf("nested kustomization %q: %w", res, err)
			}
			allManifests = append(allManifests, nested...)
		} else {
			content, err := os.ReadFile(resPath)
			if err != nil {
				return nil, nil, fmt.Errorf("reading resource %q: %w", res, err)
			}
			if len(allManifests) > 0 && !strings.HasSuffix(string(allManifests), "---\n") {
				allManifests = append(allManifests, []byte("---\n")...)
			}
			allManifests = append(allManifests, content...)
		}
	}

	return allManifests, k, nil
}

// applyImageOverrides replaces image references in YAML manifests.
// Each override maps an old image name to a new name:tag.
func applyImageOverrides(manifests []byte, overrides []KustomizeImage) []byte {
	result := string(manifests)
	for _, img := range overrides {
		oldRef := img.Name
		var newRef string
		if img.NewName != "" {
			newRef = img.NewName
		} else {
			newRef = img.Name
		}
		if img.Digest != "" {
			newRef += "@" + img.Digest
		} else if img.NewTag != "" {
			newRef += ":" + img.NewTag
		}

		// Replace "image: oldRef" and "image: oldRef:anything" patterns
		// Handle both quoted and unquoted values
		lines := strings.Split(result, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "image:") && !strings.HasPrefix(trimmed, "- image:") {
				continue
			}
			// Extract the image value
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) < 2 {
				continue
			}
			imgVal := strings.TrimSpace(parts[1])
			// Handle "image: name" in a "- image:" context
			if strings.HasPrefix(trimmed, "- image:") {
				parts = strings.SplitN(trimmed, "image:", 2)
				imgVal = strings.TrimSpace(parts[1])
			}
			imgVal = strings.Trim(imgVal, "\"'")

			// Match if the image value starts with the old reference
			if imgVal == oldRef || strings.HasPrefix(imgVal, oldRef+":") || strings.HasPrefix(imgVal, oldRef+"@") {
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				if strings.HasPrefix(trimmed, "- image:") {
					lines[i] = indent + "- image: " + newRef
				} else {
					lines[i] = indent + "image: " + newRef
				}
			}
		}
		result = strings.Join(lines, "\n")
	}
	return []byte(result)
}

// parseImageLines parses "NAME=REGISTRY/IMAGE:TAG" lines into KustomizeImage overrides.
func parseImageLines(input string) []KustomizeImage {
	var images []KustomizeImage
	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		oldName := strings.TrimSpace(parts[0])
		newRef := strings.TrimSpace(parts[1])

		img := KustomizeImage{Name: oldName}
		// Split newRef into name:tag or name@digest
		if idx := strings.LastIndex(newRef, "@"); idx > 0 {
			img.NewName = newRef[:idx]
			img.Digest = newRef[idx+1:]
		} else if idx := strings.LastIndex(newRef, ":"); idx > 0 {
			img.NewName = newRef[:idx]
			img.NewTag = newRef[idx+1:]
		} else {
			img.NewName = newRef
		}
		images = append(images, img)
	}
	return images
}
