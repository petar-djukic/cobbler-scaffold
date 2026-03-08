// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package build

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Package-level variables for docker/podman operations.
var (
	BinPodman = "podman"
)

// PodmanBuildFn is the function used to run podman build. The parent
// package injects the real implementation; tests can replace it.
var PodmanBuildFn func(dockerfile string, tags ...string) error

// ReadVersionConstFn reads a Version constant from a Go source file.
// Injected by the parent package.
var ReadVersionConstFn func(filePath string) string

// GitListTagsFn returns git tags matching a pattern in a directory.
// Injected by the parent package.
var GitListTagsFn func(pattern, dir string) []string

// ---------------------------------------------------------------------------
// Docker / Podman image operations
// ---------------------------------------------------------------------------

// DockerConfig holds configuration for image build/clean operations.
type DockerConfig struct {
	Image       string
	VersionFile string
}

// BuildImage builds the container image using podman from the embedded
// Dockerfile content. It reads the version from the consuming project's
// version file (VersionFile in Config). If no version file is configured
// or it has no Version constant, it falls back to the latest v* git tag.
// Both a versioned tag and "latest" are applied to the built image.
func BuildImage(cfg DockerConfig, embeddedDockerfile string) error {
	imageName := ImageBaseName(cfg.Image)
	if imageName == "" {
		return fmt.Errorf("podman.image not set in configuration; cannot determine image name")
	}

	tag := ReadVersionConstFn(cfg.VersionFile)
	if tag == "" {
		tag = LatestVersionTag()
	}
	if tag == "" {
		return fmt.Errorf("no version found; set version_file in configuration.yaml or tag the repository (e.g., v[REL].YYYYMMDD.N)")
	}

	versionedImage := imageName + ":" + tag
	latestImage := imageName + ":latest"

	Log("buildImage: building %s", versionedImage)
	if err := BuildFromEmbeddedDockerfile(embeddedDockerfile, versionedImage, latestImage); err != nil {
		return fmt.Errorf("podman build: %w", err)
	}

	Log("buildImage: done — %s and %s", versionedImage, latestImage)
	return nil
}

// PodmanClean removes all podman containers (running or stopped) that
// were created from the given image. It resolves the image name to its
// image ID so that containers created from any alias are caught.
func PodmanClean(image string) error {
	if image == "" {
		return fmt.Errorf("podman.image not set in configuration")
	}

	imageID, err := PodmanImageID(image)
	if err != nil {
		return fmt.Errorf("resolving image ID for %s: %w", image, err)
	}
	if imageID == "" {
		Log("podmanClean: image %s not found locally, nothing to clean", image)
		return nil
	}

	out, err := exec.Command(BinPodman, "ps", "-a",
		"--filter", "ancestor="+imageID,
		"--format", "{{.ID}} {{.Status}}",
	).Output()
	if err != nil {
		return fmt.Errorf("listing containers for %s (%s): %w", image, imageID, err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		Log("podmanClean: no containers found for image %s (%s)", image, ShortID(imageID))
		return nil
	}

	var ids []string
	for _, line := range lines {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] != "" {
			ids = append(ids, fields[0])
		}
	}

	Log("podmanClean: removing %d container(s) for image %s (%s)", len(ids), image, ShortID(imageID))
	args := append([]string{"rm", "-f"}, ids...)
	cmd := exec.Command(BinPodman, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("removing containers: %w", err)
	}

	Log("podmanClean: done")
	return nil
}

// EnsureImage checks whether the given image exists locally. If missing,
// it builds it from the provided embedded Dockerfile content.
func EnsureImage(image, embeddedDockerfile string) error {
	if PodmanImageExists(image) {
		return nil
	}

	Log("ensureImage: %s not found locally, building from embedded Dockerfile", image)
	if err := BuildFromEmbeddedDockerfile(embeddedDockerfile, image); err != nil {
		return fmt.Errorf("auto-building %s: %w", image, err)
	}
	Log("ensureImage: built %s", image)
	return nil
}

// BuildFromEmbeddedDockerfile writes the given Dockerfile content to a
// temp file and runs podman build with the given image tags.
func BuildFromEmbeddedDockerfile(dockerfileContent string, tags ...string) error {
	tmp, err := os.CreateTemp("", "Dockerfile.claude-*")
	if err != nil {
		return fmt.Errorf("creating temp Dockerfile: %w", err)
	}
	defer func() {
		if err := os.Remove(tmp.Name()); err != nil && !os.IsNotExist(err) {
			Log("docker: warning: removing temp Dockerfile: %v", err)
		}
	}()

	if _, err := tmp.WriteString(dockerfileContent); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp Dockerfile: %w", err)
	}
	tmp.Close()

	return PodmanBuildFn(tmp.Name(), tags...)
}

// PodmanImageExists returns true if the given image reference exists
// in the local podman image store. A 15-second deadline prevents a
// slow or unresponsive podman socket from blocking indefinitely.
func PodmanImageExists(image string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, BinPodman, "image", "exists", image).Run(); err != nil {
		if ctx.Err() != nil {
			Log("podmanImageExists: timed out querying podman for %s", image)
		}
		return false
	}
	return true
}

// PodmanImageID resolves an image name/tag to its full image ID.
// Returns "" if the image does not exist locally.
func PodmanImageID(image string) (string, error) {
	out, err := exec.Command(BinPodman, "image", "inspect", image,
		"--format", "{{.Id}}",
	).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 125 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ShortID returns the first 12 characters of an image ID for display,
// or the full string if it is shorter than 12 characters.
func ShortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// ImageBaseName extracts the image name without a tag from a full image
// reference. For example, "cobbler-scaffold:latest" returns
// "cobbler-scaffold". If no tag is present, the input is returned as-is.
func ImageBaseName(image string) string {
	if i := strings.LastIndex(image, ":"); i > 0 {
		return image[:i]
	}
	return image
}

// LatestVersionTag returns the most recent v* git tag, or "" if none exist.
func LatestVersionTag() string {
	tags := GitListTagsFn("v*", ".")
	if len(tags) == 0 {
		return ""
	}
	return tags[len(tags)-1]
}
