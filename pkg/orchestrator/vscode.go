// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// vscode.go delegates VS Code extension operations to the internal/vscode
// sub-package.
// prd: prd006-vscode-extension R10

package orchestrator

import (
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/vscode"
)

// VsCode manages VS Code extension packaging and installation.
type VsCode struct {
	logf func(string, ...any)
}

// NewVsCode creates a VsCode manager with explicit dependencies.
func NewVsCode(logf func(string, ...any)) *VsCode {
	return &VsCode{logf: logf}
}

// VscodePush compiles the VS Code extension from source, packages it as a
// .vsix archive, and installs it into VS Code.
func (v *VsCode) VscodePush(profile string) error {
	return vscode.Push(profile, v.logf)
}

// VscodePop uninstalls the VS Code extension.
func (v *VsCode) VscodePop(profile string) error {
	return vscode.Pop(profile, v.logf)
}
