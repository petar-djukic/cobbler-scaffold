// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package generate

import "os/exec"

// ---------------------------------------------------------------------------
// Mock implementations for role-based git interfaces used in tests.
// Each mock holds function fields so individual tests can wire only
// the methods they exercise.
// ---------------------------------------------------------------------------

// mockRepoReader implements gitops.RepoReader for tests.
type mockRepoReader struct {
	CurrentBranchFn func(dir string) (string, error)
	BranchExistsFn  func(name, dir string) bool
	ListBranchesFn  func(pattern, dir string) []string
	ListTagsFn      func(pattern, dir string) []string
	LsFilesFn       func(dir string) []string
	RevParseHEADFn  func(dir string) (string, error)
}

func (m *mockRepoReader) CurrentBranch(dir string) (string, error) {
	if m.CurrentBranchFn != nil {
		return m.CurrentBranchFn(dir)
	}
	return "", nil
}
func (m *mockRepoReader) BranchExists(name, dir string) bool {
	if m.BranchExistsFn != nil {
		return m.BranchExistsFn(name, dir)
	}
	return false
}
func (m *mockRepoReader) ListBranches(pattern, dir string) []string {
	if m.ListBranchesFn != nil {
		return m.ListBranchesFn(pattern, dir)
	}
	return nil
}
func (m *mockRepoReader) ListTags(pattern, dir string) []string {
	if m.ListTagsFn != nil {
		return m.ListTagsFn(pattern, dir)
	}
	return nil
}
func (m *mockRepoReader) LsFiles(dir string) []string {
	if m.LsFilesFn != nil {
		return m.LsFilesFn(dir)
	}
	return nil
}
func (m *mockRepoReader) RevParseHEAD(dir string) (string, error) {
	if m.RevParseHEADFn != nil {
		return m.RevParseHEADFn(dir)
	}
	return "", nil
}

// mockBranchManager implements gitops.BranchManager for tests.
type mockBranchManager struct {
	CheckoutFn          func(branch, dir string) error
	CheckoutNewFn       func(branch, dir string) error
	CreateBranchFn      func(name, dir string) error
	DeleteBranchFn      func(name, dir string) error
	ForceDeleteBranchFn func(name, dir string) error
	MergeCmdFn          func(branch, dir string) *exec.Cmd
}

func (m *mockBranchManager) Checkout(branch, dir string) error {
	if m.CheckoutFn != nil {
		return m.CheckoutFn(branch, dir)
	}
	return nil
}
func (m *mockBranchManager) CheckoutNew(branch, dir string) error {
	if m.CheckoutNewFn != nil {
		return m.CheckoutNewFn(branch, dir)
	}
	return nil
}
func (m *mockBranchManager) CreateBranch(name, dir string) error {
	if m.CreateBranchFn != nil {
		return m.CreateBranchFn(name, dir)
	}
	return nil
}
func (m *mockBranchManager) DeleteBranch(name, dir string) error {
	if m.DeleteBranchFn != nil {
		return m.DeleteBranchFn(name, dir)
	}
	return nil
}
func (m *mockBranchManager) ForceDeleteBranch(name, dir string) error {
	if m.ForceDeleteBranchFn != nil {
		return m.ForceDeleteBranchFn(name, dir)
	}
	return nil
}
func (m *mockBranchManager) MergeCmd(branch, dir string) *exec.Cmd {
	if m.MergeCmdFn != nil {
		return m.MergeCmdFn(branch, dir)
	}
	return exec.Command("true")
}

// mockCommitWriter implements gitops.CommitWriter for tests.
type mockCommitWriter struct {
	StageAllFn         func(dir string) error
	StageDirFn         func(path, dir string) error
	UnstageAllFn       func(dir string) error
	HasChangesFn       func(dir string) bool
	StashFn            func(msg, dir string) error
	CommitFn           func(msg, dir string) error
	CommitAllowEmptyFn func(msg, dir string) error
	ResetSoftFn        func(ref, dir string) error
}

func (m *mockCommitWriter) StageAll(dir string) error {
	if m.StageAllFn != nil {
		return m.StageAllFn(dir)
	}
	return nil
}
func (m *mockCommitWriter) StageDir(path, dir string) error {
	if m.StageDirFn != nil {
		return m.StageDirFn(path, dir)
	}
	return nil
}
func (m *mockCommitWriter) UnstageAll(dir string) error {
	if m.UnstageAllFn != nil {
		return m.UnstageAllFn(dir)
	}
	return nil
}
func (m *mockCommitWriter) HasChanges(dir string) bool {
	if m.HasChangesFn != nil {
		return m.HasChangesFn(dir)
	}
	return false
}
func (m *mockCommitWriter) Stash(msg, dir string) error {
	if m.StashFn != nil {
		return m.StashFn(msg, dir)
	}
	return nil
}
func (m *mockCommitWriter) Commit(msg, dir string) error {
	if m.CommitFn != nil {
		return m.CommitFn(msg, dir)
	}
	return nil
}
func (m *mockCommitWriter) CommitAllowEmpty(msg, dir string) error {
	if m.CommitAllowEmptyFn != nil {
		return m.CommitAllowEmptyFn(msg, dir)
	}
	return nil
}
func (m *mockCommitWriter) ResetSoft(ref, dir string) error {
	if m.ResetSoftFn != nil {
		return m.ResetSoftFn(ref, dir)
	}
	return nil
}

// mockWorktreeManager implements gitops.WorktreeManager for tests.
type mockWorktreeManager struct {
	WorktreeAddFn    func(worktreeDir, branch, dir string) *exec.Cmd
	WorktreeRemoveFn func(worktreeDir, dir string) error
	WorktreePruneFn  func(dir string) error
}

func (m *mockWorktreeManager) WorktreeAdd(worktreeDir, branch, dir string) *exec.Cmd {
	if m.WorktreeAddFn != nil {
		return m.WorktreeAddFn(worktreeDir, branch, dir)
	}
	return exec.Command("true")
}
func (m *mockWorktreeManager) WorktreeRemove(worktreeDir, dir string) error {
	if m.WorktreeRemoveFn != nil {
		return m.WorktreeRemoveFn(worktreeDir, dir)
	}
	return nil
}
func (m *mockWorktreeManager) WorktreePrune(dir string) error {
	if m.WorktreePruneFn != nil {
		return m.WorktreePruneFn(dir)
	}
	return nil
}
