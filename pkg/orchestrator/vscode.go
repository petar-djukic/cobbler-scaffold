// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// vscode.go delegates VS Code extension operations to the internal/vscode
// sub-package.
// prd: prd006-vscode-extension R10

package orchestrator

import (
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/vscode"
)

// VscodeManager manages VS Code extension packaging and installation.
type VscodeManager interface {
	VscodePush(profile string) error
	VscodePop(profile string) error
}

// VscodePush compiles the VS Code extension from source, packages it as a
// .vsix archive, and installs it into VS Code.
func (o *Orchestrator) VscodePush(profile string) error {
	return vscode.Push(profile, logf)
}

// VscodePop uninstalls the VS Code extension.
func (o *Orchestrator) VscodePop(profile string) error {
	return vscode.Pop(profile, logf)
}
