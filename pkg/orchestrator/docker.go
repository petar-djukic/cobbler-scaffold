// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	_ "embed"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/build"
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
	return build.BuildImage(build.DockerConfig{
		Image:       o.cfg.Podman.Image,
		VersionFile: o.cfg.Project.VersionFile,
	}, embeddedDockerfile)
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
	return build.PodmanClean(o.cfg.Podman.Image)
}

// ensureImage checks whether the configured PodmanImage exists locally.
// If missing, it builds it from the embedded Dockerfile.
func (o *Orchestrator) ensureImage() error {
	return build.EnsureImage(o.cfg.Podman.Image, embeddedDockerfile)
}

// buildFromEmbeddedDockerfile writes the embedded Dockerfile to a temp
// file and runs podman build with the given image tags.
func buildFromEmbeddedDockerfile(tags ...string) error {
	return build.BuildFromEmbeddedDockerfile(embeddedDockerfile, tags...)
}

// podmanImageExists returns true if the given image reference exists
// in the local podman image store.
func podmanImageExists(image string) bool {
	return build.PodmanImageExists(image)
}

// podmanImageID resolves an image name/tag to its full image ID.
// Returns "" if the image does not exist locally.
func podmanImageID(image string) (string, error) {
	return build.PodmanImageID(image)
}

// shortID returns the first 12 characters of an image ID for display,
// or the full string if it is shorter than 12 characters.
func shortID(id string) string {
	return build.ShortID(id)
}

// imageBaseName extracts the image name without a tag from a full image
// reference. For example, "cobbler-scaffold:latest" returns
// "cobbler-scaffold". If no tag is present, the input is returned
// as-is.
func imageBaseName(image string) string {
	return build.ImageBaseName(image)
}

// latestVersionTag returns the most recent v* git tag, or "" if none exist.
func latestVersionTag() string {
	return build.LatestVersionTag()
}

