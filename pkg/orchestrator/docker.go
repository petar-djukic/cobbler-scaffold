// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

//go:embed Dockerfile.claude
var embeddedDockerfile string

// BuildImage builds the container image using podman from the embedded
// Dockerfile. It reads the version from the consuming project's version
// file (VersionFile in Config). If no version file is configured or it
// has no Version constant, it falls back to the latest v* git tag.
// Both a versioned tag and "latest" are applied to the built image.
// The image name is taken from PodmanImage (stripped of any existing tag).
//
// Exposed as a mage target (e.g., mage buildImage).
func (o *Orchestrator) BuildImage() error {
	imageName := imageBaseName(o.cfg.Podman.Image)
	if imageName == "" {
		return fmt.Errorf("podman.image not set in configuration; cannot determine image name")
	}

	// Prefer version from the project's version file; fall back to git tags.
	tag := readVersionConst(o.cfg.Project.VersionFile)
	if tag == "" {
		tag = latestVersionTag()
	}
	if tag == "" {
		return fmt.Errorf("no version found; set version_file in configuration.yaml or tag the repository (e.g., v[REL].YYYYMMDD.N)")
	}

	versionedImage := imageName + ":" + tag
	latestImage := imageName + ":latest"

	logf("buildImage: building %s", versionedImage)
	if err := buildFromEmbeddedDockerfile(versionedImage, latestImage); err != nil {
		return fmt.Errorf("podman build: %w", err)
	}

	logf("buildImage: done — %s and %s", versionedImage, latestImage)
	return nil
}

// PodmanClean removes all podman containers (running or stopped) that
// were created from the configured PodmanImage. It resolves the
// configured image name to its image ID so that containers created
// from any alias of the same image are caught (e.g. claude-cli,
// cobbler-scaffold, and mage-claude-orchestrator may all share the
// same image ID).
//
// Exposed as a mage target (e.g., mage podman:clean).
func (o *Orchestrator) PodmanClean() error {
	image := o.cfg.Podman.Image
	if image == "" {
		return fmt.Errorf("podman.image not set in configuration")
	}

	// Resolve to image ID so we catch containers from any name alias.
	imageID, err := podmanImageID(image)
	if err != nil {
		return fmt.Errorf("resolving image ID for %s: %w", image, err)
	}
	if imageID == "" {
		logf("podmanClean: image %s not found locally, nothing to clean", image)
		return nil
	}

	// List all containers (running + stopped) created from this image ID.
	out, err := exec.Command(binPodman, "ps", "-a",
		"--filter", "ancestor="+imageID,
		"--format", "{{.ID}} {{.Status}}",
	).Output()
	if err != nil {
		return fmt.Errorf("listing containers for %s (%s): %w", image, imageID, err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		logf("podmanClean: no containers found for image %s (%s)", image, imageID[:12])
		return nil
	}

	var ids []string
	for _, line := range lines {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] != "" {
			ids = append(ids, fields[0])
		}
	}

	logf("podmanClean: removing %d container(s) for image %s (%s)", len(ids), image, imageID[:12])
	args := append([]string{"rm", "-f"}, ids...)
	cmd := exec.Command(binPodman, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("removing containers: %w", err)
	}

	logf("podmanClean: done")
	return nil
}

// ensureImage checks whether the configured PodmanImage exists locally.
// If missing, it builds it from the embedded Dockerfile.
func (o *Orchestrator) ensureImage() error {
	if podmanImageExists(o.cfg.Podman.Image) {
		return nil
	}

	logf("ensureImage: %s not found locally, building from embedded Dockerfile", o.cfg.Podman.Image)
	if err := buildFromEmbeddedDockerfile(o.cfg.Podman.Image); err != nil {
		return fmt.Errorf("auto-building %s: %w", o.cfg.Podman.Image, err)
	}
	logf("ensureImage: built %s", o.cfg.Podman.Image)
	return nil
}

// buildFromEmbeddedDockerfile writes the embedded Dockerfile to a temp
// file and runs podman build with the given image tags.
func buildFromEmbeddedDockerfile(tags ...string) error {
	tmp, err := os.CreateTemp("", "Dockerfile.claude-*")
	if err != nil {
		return fmt.Errorf("creating temp Dockerfile: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(embeddedDockerfile); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp Dockerfile: %w", err)
	}
	tmp.Close()

	return podmanBuild(tmp.Name(), tags...)
}

// podmanImageExists returns true if the given image reference exists
// in the local podman image store.
func podmanImageExists(image string) bool {
	return exec.Command(binPodman, "image", "exists", image).Run() == nil
}

// podmanImageID resolves an image name/tag to its full image ID.
// Returns "" if the image does not exist locally.
func podmanImageID(image string) (string, error) {
	out, err := exec.Command(binPodman, "image", "inspect", image,
		"--format", "{{.Id}}",
	).Output()
	if err != nil {
		// image not found is not an error for our purposes
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 125 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// imageBaseName extracts the image name without a tag from a full image
// reference. For example, "cobbler-scaffold:latest" returns
// "cobbler-scaffold". If no tag is present, the input is returned
// as-is.
func imageBaseName(image string) string {
	if i := strings.LastIndex(image, ":"); i > 0 {
		return image[:i]
	}
	return image
}

// latestVersionTag returns the most recent v* git tag, or "" if none exist.
func latestVersionTag() string {
	tags := gitListTags("v*")
	if len(tags) == 0 {
		return ""
	}
	// gitListTags returns tags sorted by name; the last one is the highest
	// version when using the v[REL].[DATE].[REVISION] convention.
	return tags[len(tags)-1]
}
