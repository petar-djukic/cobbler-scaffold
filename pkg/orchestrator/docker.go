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
// Dockerfile. It derives the version tag from the latest v* git tag and
// applies both the versioned tag and "latest" to the built image. The
// image name is taken from PodmanImage (stripped of any existing tag).
//
// Exposed as a mage target (e.g., mage buildImage).
func (o *Orchestrator) BuildImage() error {
	imageName := imageBaseName(o.cfg.PodmanImage)
	if imageName == "" {
		return fmt.Errorf("podman_image not set in configuration; cannot determine image name")
	}

	tag := latestVersionTag()
	if tag == "" {
		return fmt.Errorf("no v* git tag found; tag the repository first (e.g., v0.YYYYMMDD.N)")
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

// ensureImage checks whether the configured PodmanImage exists locally.
// If missing, it builds it from the embedded Dockerfile.
func (o *Orchestrator) ensureImage() error {
	if podmanImageExists(o.cfg.PodmanImage) {
		return nil
	}

	logf("ensureImage: %s not found locally, building from embedded Dockerfile", o.cfg.PodmanImage)
	if err := buildFromEmbeddedDockerfile(o.cfg.PodmanImage); err != nil {
		return fmt.Errorf("auto-building %s: %w", o.cfg.PodmanImage, err)
	}
	logf("ensureImage: built %s", o.cfg.PodmanImage)
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

// imageBaseName extracts the image name without a tag from a full image
// reference. For example, "mage-claude-orchestrator:latest" returns
// "mage-claude-orchestrator". If no tag is present, the input is returned
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
