// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"os"
	"path/filepath"
	"strings"
)

type StdioCommandRule struct {
	Command      string   `json:"command"`
	ArgvPrefix   []string `json:"argv_prefix,omitempty"`
	ExactArgs    []string `json:"exact_args,omitempty"`
	AllowAnyArgs bool     `json:"allow_any_args,omitempty"`
}

type StdioCommandPolicyOptions struct {
	AllowedCommands []string
	NpxPackages     []string
	UvxPackages     []string
	NodeScriptRoots []string
	CommandRules    []StdioCommandRule
}

type stdioCommandKind uint8

const (
	stdioCommandOther stdioCommandKind = iota
	stdioCommandNPX
	stdioCommandUVX
	stdioCommandNode
)

type executableSnapshot struct {
	path    string
	info    os.FileInfo
	size    int64
	mode    os.FileMode
	modTime int64
}

type compiledStdioCommand struct {
	identity      executableSnapshot
	kind          stdioCommandKind
	argumentRules []compiledStdioArgumentRule
}

type compiledStdioArgumentRule struct {
	prefix   []string
	exact    []string
	allowAny bool
}

type ResolvedStdioInvocation struct {
	Command string
	Args    []string

	executable executableSnapshot
	nodeScript *executableSnapshot
}

type StdioCommandPolicy struct {
	commands        map[string]compiledStdioCommand
	npxPackages     map[string]struct{}
	uvxPackages     map[string]struct{}
	nodeScriptRoots []string
}

func NewStdioCommandPolicy(options StdioCommandPolicyOptions) (*StdioCommandPolicy, error) {
	if len(options.AllowedCommands) == 0 {
		return nil, ErrStdioCommandDenied
	}
	policy := &StdioCommandPolicy{
		commands:    make(map[string]compiledStdioCommand, len(options.AllowedCommands)),
		npxPackages: exactPackageSet(options.NpxPackages),
		uvxPackages: exactPackageSet(options.UvxPackages),
	}
	for _, root := range options.NodeScriptRoots {
		resolved, err := resolveRealDirectory(root)
		if err != nil {
			return nil, ErrStdioCommandDenied
		}
		policy.nodeScriptRoots = append(policy.nodeScriptRoots, resolved)
	}
	for _, command := range options.AllowedCommands {
		identity, err := inspectExecutable(command)
		if err != nil {
			return nil, ErrStdioCommandDenied
		}
		kind, err := classifyStdioCommand(command)
		if err != nil {
			return nil, err
		}
		if existing, ok := policy.commands[identity.path]; ok && existing.kind != kind {
			return nil, ErrStdioCommandDenied
		}
		policy.commands[identity.path] = compiledStdioCommand{identity: identity, kind: kind}
	}
	for _, rule := range options.CommandRules {
		identity, err := inspectExecutable(rule.Command)
		if err != nil {
			return nil, ErrStdioCommandDenied
		}
		compiled, ok := policy.commands[identity.path]
		if !ok || compiled.kind != stdioCommandOther {
			return nil, ErrStdioCommandDenied
		}
		modeCount := 0
		if len(rule.ArgvPrefix) > 0 {
			modeCount++
		}
		if rule.ExactArgs != nil {
			modeCount++
		}
		if rule.AllowAnyArgs {
			modeCount++
		}
		if modeCount != 1 {
			return nil, ErrStdioCommandDenied
		}
		prefix := append([]string(nil), rule.ArgvPrefix...)
		exact := append([]string(nil), rule.ExactArgs...)
		for _, argument := range append(append([]string(nil), prefix...), exact...) {
			if strings.ContainsRune(argument, 0) {
				return nil, ErrStdioCommandDenied
			}
		}
		compiled.argumentRules = append(compiled.argumentRules, compiledStdioArgumentRule{
			prefix: prefix, exact: exact, allowAny: rule.AllowAnyArgs,
		})
		policy.commands[identity.path] = compiled
	}
	return policy, nil
}

func (p *StdioCommandPolicy) Resolve(command string, args []string, workingDir string) (string, error) {
	invocation, err := p.ResolveInvocation(command, args, workingDir)
	if err != nil {
		return "", err
	}
	return invocation.Command, nil
}

func (p *StdioCommandPolicy) ResolveInvocation(
	command string,
	args []string,
	workingDir string,
) (ResolvedStdioInvocation, error) {
	if p == nil {
		return ResolvedStdioInvocation{}, ErrStdioCommandDenied
	}
	current, err := inspectExecutable(command)
	if err != nil {
		return ResolvedStdioInvocation{}, ErrStdioCommandDenied
	}
	compiled, ok := p.commands[current.path]
	if !ok || !sameExecutableSnapshot(compiled.identity, current) {
		return ResolvedStdioInvocation{}, ErrStdioCommandDenied
	}
	resolvedArgs := append([]string(nil), args...)
	for _, argument := range resolvedArgs {
		if strings.ContainsRune(argument, 0) {
			return ResolvedStdioInvocation{}, ErrStdioCommandDenied
		}
	}
	invocation := ResolvedStdioInvocation{
		Command: current.path, Args: resolvedArgs, executable: current,
	}
	switch compiled.kind {
	case stdioCommandNPX:
		if !allowedNPXInvocation(resolvedArgs, p.npxPackages) {
			return ResolvedStdioInvocation{}, ErrStdioCommandDenied
		}
	case stdioCommandUVX:
		if !allowedUVXInvocation(resolvedArgs, p.uvxPackages) {
			return ResolvedStdioInvocation{}, ErrStdioCommandDenied
		}
	case stdioCommandNode:
		script, err := resolveNodeScript(resolvedArgs, workingDir, p.nodeScriptRoots)
		if err != nil {
			return ResolvedStdioInvocation{}, ErrStdioCommandDenied
		}
		invocation.Args[0] = script.path
		invocation.nodeScript = &script
	default:
		if !matchesArgumentRule(resolvedArgs, compiled.argumentRules) {
			return ResolvedStdioInvocation{}, ErrStdioCommandDenied
		}
	}
	return invocation, nil
}

func sameResolvedStdioInvocation(expected, current ResolvedStdioInvocation) bool {
	if expected.Command != current.Command || !sameExecutableSnapshot(expected.executable, current.executable) ||
		len(expected.Args) != len(current.Args) {
		return false
	}
	for index := range expected.Args {
		if expected.Args[index] != current.Args[index] {
			return false
		}
	}
	if expected.nodeScript == nil || current.nodeScript == nil {
		return expected.nodeScript == nil && current.nodeScript == nil
	}
	return sameExecutableSnapshot(*expected.nodeScript, *current.nodeScript)
}

func inspectExecutable(command string) (executableSnapshot, error) {
	path, err := resolveExecutable(strings.TrimSpace(command))
	if err != nil {
		return executableSnapshot{}, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return executableSnapshot{}, ErrStdioCommandDenied
	}
	return executableSnapshot{
		path:    path,
		info:    info,
		size:    info.Size(),
		mode:    info.Mode(),
		modTime: info.ModTime().UnixNano(),
	}, nil
}

func sameExecutableSnapshot(expected, current executableSnapshot) bool {
	return expected.path == current.path && os.SameFile(expected.info, current.info) &&
		expected.size == current.size && expected.mode == current.mode &&
		expected.modTime == current.modTime
}

func classifyStdioCommand(command string) (stdioCommandKind, error) {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(command))) {
	case "npx":
		return stdioCommandNPX, nil
	case "uvx":
		return stdioCommandUVX, nil
	case "node":
		return stdioCommandNode, nil
	case "sh", "bash", "dash", "zsh", "fish", "ksh", "csh", "tcsh":
		return stdioCommandOther, ErrStdioCommandDenied
	default:
		return stdioCommandOther, nil
	}
}

func exactPackageSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n\t ") {
			continue
		}
		result[value] = struct{}{}
	}
	return result
}

func allowedNPXInvocation(args []string, packages map[string]struct{}) bool {
	if len(packages) == 0 || len(args) == 0 {
		return false
	}
	for _, argument := range args {
		if forbiddenNPXArgument(argument) {
			return false
		}
	}
	index := 0
	if args[index] == "-y" || args[index] == "--yes" {
		index++
	}
	if index >= len(args) || strings.HasPrefix(args[index], "-") {
		return false
	}
	_, ok := packages[args[index]]
	return ok
}

func forbiddenNPXArgument(argument string) bool {
	name := argument
	if separator := strings.IndexByte(name, '='); separator >= 0 {
		name = name[:separator]
	}
	switch name {
	case "-c", "--call", "-p", "--package", "--shell", "--script-shell", "--eval", "--node-options":
		return true
	}
	return strings.HasPrefix(name, "-p") && name != "-"
}

func allowedUVXInvocation(args []string, packages map[string]struct{}) bool {
	if len(packages) == 0 || len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return false
	}
	for _, argument := range args {
		name := argument
		if separator := strings.IndexByte(name, '='); separator >= 0 {
			name = name[:separator]
		}
		switch name {
		case "--from", "--with", "--python", "--index", "--index-url":
			return false
		}
	}
	_, ok := packages[args[0]]
	return ok
}

func resolveNodeScript(args []string, workingDir string, roots []string) (executableSnapshot, error) {
	if len(args) == 0 || len(roots) == 0 || strings.HasPrefix(args[0], "-") {
		return executableSnapshot{}, ErrStdioCommandDenied
	}
	script := strings.TrimSpace(args[0])
	if script == "" || strings.ContainsRune(script, 0) {
		return executableSnapshot{}, ErrStdioCommandDenied
	}
	if !filepath.IsAbs(script) {
		if !filepath.IsAbs(strings.TrimSpace(workingDir)) {
			return executableSnapshot{}, ErrStdioCommandDenied
		}
		script = filepath.Join(workingDir, script)
	}
	realScript, err := filepath.EvalSymlinks(filepath.Clean(script))
	if err != nil || !filepath.IsAbs(realScript) {
		return executableSnapshot{}, ErrStdioCommandDenied
	}
	info, err := os.Stat(realScript)
	if err != nil || !info.Mode().IsRegular() {
		return executableSnapshot{}, ErrStdioCommandDenied
	}
	for _, root := range roots {
		if pathWithin(realScript, root) {
			return executableSnapshot{
				path: realScript, info: info, size: info.Size(), mode: info.Mode(), modTime: info.ModTime().UnixNano(),
			}, nil
		}
	}
	return executableSnapshot{}, ErrStdioCommandDenied
}

func matchesArgumentRule(args []string, rules []compiledStdioArgumentRule) bool {
	for _, rule := range rules {
		if rule.allowAny {
			return true
		}
		if rule.exact != nil {
			if len(args) != len(rule.exact) {
				continue
			}
			matched := true
			for index := range rule.exact {
				if args[index] != rule.exact[index] {
					matched = false
					break
				}
			}
			if matched {
				return true
			}
			continue
		}
		prefix := rule.prefix
		if len(args) < len(prefix) {
			continue
		}
		matched := true
		for index := range prefix {
			if args[index] != prefix[index] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func resolveRealDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		return "", ErrStdioCommandDenied
	}
	realPath, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil || !filepath.IsAbs(realPath) {
		return "", ErrStdioCommandDenied
	}
	info, err := os.Stat(realPath)
	if err != nil || !info.IsDir() {
		return "", ErrStdioCommandDenied
	}
	return filepath.Clean(realPath), nil
}
