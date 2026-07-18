// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxLogicalExecutableBytes = 128

type LogicalStdioPolicyOptions struct {
	AllowedCommands  []string
	NpxPackages      []string
	UvxPackages      []string
	NodeScriptRoots  []string
	CommandRules     []StdioCommandRule
	AllowedEnvKeys   []string
	MaxArgs          int
	MaxArgBytes      int
	MaxEnvVars       int
	MaxEnvValueBytes int
}

type logicalStdioCommand struct {
	kind          stdioCommandKind
	argumentRules []compiledStdioArgumentRule
}

type LogicalStdioPolicy struct {
	commands         map[string]logicalStdioCommand
	npxPackages      map[string]struct{}
	uvxPackages      map[string]struct{}
	nodeScriptRoots  []string
	allowedEnvKeys   map[string]struct{}
	maxArgs          int
	maxArgBytes      int
	maxEnvVars       int
	maxEnvValueBytes int
}

func NewLogicalStdioPolicy(options LogicalStdioPolicyOptions) (*LogicalStdioPolicy, error) {
	if len(options.AllowedCommands) == 0 || options.MaxArgs < 0 ||
		options.MaxArgBytes <= 0 || options.MaxEnvVars < 0 ||
		options.MaxEnvValueBytes <= 0 {
		return nil, ErrStdioCommandDenied
	}
	policy := &LogicalStdioPolicy{
		commands:         make(map[string]logicalStdioCommand, len(options.AllowedCommands)),
		npxPackages:      exactPackageSet(options.NpxPackages),
		uvxPackages:      exactPackageSet(options.UvxPackages),
		allowedEnvKeys:   make(map[string]struct{}, len(options.AllowedEnvKeys)),
		maxArgs:          options.MaxArgs,
		maxArgBytes:      options.MaxArgBytes,
		maxEnvVars:       options.MaxEnvVars,
		maxEnvValueBytes: options.MaxEnvValueBytes,
	}
	for _, raw := range options.AllowedEnvKeys {
		key := strings.TrimSpace(raw)
		if !validEnvName(key) {
			return nil, ErrStdioEnvDenied
		}
		policy.allowedEnvKeys[key] = struct{}{}
	}
	for _, raw := range options.NodeScriptRoots {
		root, ok := normalizeLogicalStdioPath(raw)
		if !ok {
			return nil, ErrStdioCommandDenied
		}
		policy.nodeScriptRoots = append(policy.nodeScriptRoots, root)
	}
	sort.Strings(policy.nodeScriptRoots)
	for _, raw := range options.AllowedCommands {
		command := strings.TrimSpace(raw)
		if err := ValidateLogicalExecutable(command); err != nil {
			return nil, err
		}
		kind, err := classifyStdioCommand(command)
		if err != nil {
			return nil, ErrStdioCommandDenied
		}
		policy.commands[command] = logicalStdioCommand{kind: kind}
	}
	for _, rule := range options.CommandRules {
		command := strings.TrimSpace(rule.Command)
		if err := ValidateLogicalExecutable(command); err != nil {
			return nil, err
		}
		compiled, ok := policy.commands[command]
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
			if !validLogicalStdioArgument(argument) {
				return nil, ErrStdioCommandDenied
			}
		}
		compiled.argumentRules = append(compiled.argumentRules, compiledStdioArgumentRule{
			prefix: prefix, exact: exact, allowAny: rule.AllowAnyArgs,
		})
		policy.commands[command] = compiled
	}
	return policy, nil
}

func ValidateLogicalExecutable(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" || value != raw || len(value) > maxLogicalExecutableBytes ||
		!utf8.ValidString(value) || path.IsAbs(value) ||
		strings.ContainsAny(value, `/\:`) {
		return ErrStdioCommandDenied
	}
	switch strings.ToLower(value) {
	case "sh", "bash", "dash", "zsh", "fish", "ksh", "csh", "tcsh":
		return ErrStdioCommandDenied
	}
	for index := range value {
		character := value[index]
		if index == 0 && !logicalExecutableAlphaNumeric(character) {
			return ErrStdioCommandDenied
		}
		if !logicalExecutableAlphaNumeric(character) && character != '_' &&
			character != '-' && character != '.' && character != '+' {
			return ErrStdioCommandDenied
		}
	}
	return nil
}

func (p *LogicalStdioPolicy) Validate(config StdioConfig) error {
	if p == nil {
		return ErrStdioCommandDenied
	}
	command := strings.TrimSpace(config.Command)
	if command != config.Command || ValidateLogicalExecutable(command) != nil {
		return ErrStdioCommandDenied
	}
	compiled, ok := p.commands[command]
	if !ok {
		return ErrStdioCommandDenied
	}
	if len(config.Args) > p.maxArgs {
		return ErrStdioArgsLimitExceeded
	}
	argBytes := 0
	args := append([]string(nil), config.Args...)
	for _, argument := range args {
		if !validLogicalStdioArgument(argument) {
			return ErrStdioArgsLimitExceeded
		}
		argBytes += len([]byte(argument))
		if argBytes > p.maxArgBytes {
			return ErrStdioArgsLimitExceeded
		}
	}
	if len(config.Env) > p.maxEnvVars {
		return ErrStdioEnvLimitExceeded
	}
	for key, value := range config.Env {
		if !validEnvName(key) {
			return ErrStdioEnvDenied
		}
		if _, allowed := p.allowedEnvKeys[key]; !allowed {
			return ErrStdioEnvDenied
		}
		if len([]byte(value)) > p.maxEnvValueBytes || !validLogicalStdioArgument(value) {
			return ErrStdioEnvLimitExceeded
		}
	}
	if config.WorkingDir != "" {
		if _, ok := normalizeLogicalStdioPath(config.WorkingDir); !ok {
			return ErrStdioWorkingDirDenied
		}
	}
	switch compiled.kind {
	case stdioCommandNPX:
		if !allowedNPXInvocation(args, p.npxPackages) {
			return ErrStdioCommandDenied
		}
	case stdioCommandUVX:
		if !allowedUVXInvocation(args, p.uvxPackages) {
			return ErrStdioCommandDenied
		}
	case stdioCommandNode:
		if !p.allowedLogicalNodeInvocation(args, config.WorkingDir) {
			return ErrStdioCommandDenied
		}
	default:
		if !matchesArgumentRule(args, compiled.argumentRules) {
			return ErrStdioCommandDenied
		}
	}
	return nil
}

func (p *LogicalStdioPolicy) allowedLogicalNodeInvocation(args []string, workingDir string) bool {
	if len(args) == 0 || len(p.nodeScriptRoots) == 0 || strings.HasPrefix(args[0], "-") {
		return false
	}
	script, ok := normalizeLogicalStdioPath(args[0])
	if !ok {
		return false
	}
	if workingDir != "" && !strings.HasPrefix(script, "workspace/") {
		base, baseOK := normalizeLogicalStdioPath(workingDir)
		if !baseOK {
			return false
		}
		script, ok = normalizeLogicalStdioPath(path.Join(base, script))
		if !ok {
			return false
		}
	}
	for _, root := range p.nodeScriptRoots {
		if logicalStdioPathWithin(script, root) {
			return true
		}
	}
	return false
}

func normalizeLogicalStdioPath(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || value != raw || !utf8.ValidString(value) || path.IsAbs(value) ||
		strings.ContainsAny(value, `\:`) || path.Clean(value) != value || value == "." {
		return "", false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", false
		}
		for _, character := range segment {
			if character == 0 || character == 0x7f || character < 0x20 {
				return "", false
			}
		}
	}
	return value, true
}

func logicalStdioPathWithin(target, prefix string) bool {
	return target == prefix || strings.HasPrefix(target, prefix+"/")
}

func validLogicalStdioArgument(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character == 0 || character == 0x7f || character < 0x20 {
			return false
		}
	}
	return true
}

func logicalExecutableAlphaNumeric(character byte) bool {
	return character >= 'A' && character <= 'Z' ||
		character >= 'a' && character <= 'z' ||
		character >= '0' && character <= '9'
}
