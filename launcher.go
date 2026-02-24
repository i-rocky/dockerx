package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/term"
)

const containerHome = "/home/dev"
const configStageRoot = "/tmp/dockerx-config"

type cliConfig struct {
	image       string
	shell       string
	noPull      bool
	noConfig    bool
	dryRun      bool
	verbose     bool
	showVersion bool
	command     []string
}

type mountSpec struct {
	src      string
	dst      string
	readOnly bool
}

func launchDockerx(cfg cliConfig) error {
	if cfg.image == "" {
		return errors.New("image cannot be empty")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker executable not found in PATH")
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve current directory: %w", err)
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("resolve current directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory: %w", err)
	}

	configMounts := []mountSpec{}
	if !cfg.noConfig {
		configMounts = discoverHostConfigMounts(homeDir, getenv, pathExists)
	}

	identityMounts := []mountSpec{}
	cleanupIdentity := func() {}
	defer cleanupIdentity()
	if !cfg.dryRun {
		if uidGID, ok := hostUIDGID(); ok {
			mounts, cleanup, err := prepareIdentityMounts(cfg.image, "dev", containerHome, uidGID)
			if err != nil {
				if cfg.verbose {
					fmt.Fprintf(os.Stderr, "warning: identity overlay disabled: %v\n", err)
				}
			} else {
				identityMounts = mounts
				cleanupIdentity = cleanup
			}
		}
	}

	command := cfg.command
	if len(command) == 0 {
		command = []string{cfg.shell}
	}

	args, envKeys, err := buildDockerArgs(cfg.image, workDir, command, configMounts, identityMounts, cfg.noPull)
	if err != nil {
		return err
	}

	if cfg.verbose || cfg.dryRun {
		printPlan(cfg.image, workDir, command, configMounts, envKeys, args)
	}
	if cfg.dryRun {
		return nil
	}

	cmd := exec.Command("docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker run failed: %w", err)
	}

	return nil
}

func buildDockerArgs(image, workDir string, command []string, configMounts, identityMounts []mountSpec, noPull bool) ([]string, []string, error) {
	if strings.Contains(workDir, ",") {
		return nil, nil, fmt.Errorf("current directory contains an unsupported comma: %q", workDir)
	}

	uidGID, hasUIDGID := hostUIDGID()

	containerHomeTmpfs := containerHome + ":mode=755"
	if hasUIDGID {
		parts := strings.SplitN(uidGID, ":", 2)
		if len(parts) == 2 {
			containerHomeTmpfs = fmt.Sprintf("%s:mode=755,uid=%s,gid=%s", containerHome, parts[0], parts[1])
		}
	}

	args := []string{"run", "--rm", "-i"}
	if !noPull && shouldAlwaysPull(image) {
		args = append(args, "--pull", "always")
	}
	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		args = append(args, "-t")
	}

	args = append(args,
		"--read-only",
		"--cap-drop", "ALL",
		"--cap-add", "SETUID",
		"--cap-add", "SETGID",
		"--cap-add", "AUDIT_WRITE",
		"--mount", formatMount(mountSpec{src: workDir, dst: "/app", readOnly: false}),
		"--tmpfs", "/tmp:mode=1777",
		"--tmpfs", "/run:mode=755",
		"--tmpfs", "/var/tmp:mode=1777",
		"--tmpfs", "/var/lib/apt/lists:mode=755",
		"--tmpfs", "/var/cache/apt:mode=755",
		"--tmpfs", containerHomeTmpfs,
		"--workdir", "/app",
		"--env", "HOME="+containerHome,
		"--env", "USER=dev",
		"--env", "CODEX_HOME="+containerHome+"/.codex",
	)

	if hasUIDGID {
		args = append(args, "--user", uidGID)
	}

	for _, m := range identityMounts {
		if strings.Contains(m.src, ",") {
			return nil, nil, fmt.Errorf("mount source contains an unsupported comma: %q", m.src)
		}
		args = append(args, "--mount", formatMount(m))
	}

	for i, m := range configMounts {
		if strings.Contains(m.src, ",") {
			return nil, nil, fmt.Errorf("mount source contains an unsupported comma: %q", m.src)
		}
		stagePath := fmt.Sprintf("%s/%d", configStageRoot, i)
		args = append(args, "--mount", formatMount(mountSpec{src: m.src, dst: stagePath, readOnly: true}))
		args = append(args, "--env", fmt.Sprintf("DOCKERX_CONFIG_SRC_%d=%s", i, stagePath))
		args = append(args, "--env", fmt.Sprintf("DOCKERX_CONFIG_DST_%d=%s", i, m.dst))
	}
	if len(configMounts) > 0 {
		args = append(args, "--env", fmt.Sprintf("DOCKERX_CONFIG_COUNT=%d", len(configMounts)))
	}

	envKeys := gatherPassthroughEnvKeys()
	for _, key := range envKeys {
		args = append(args, "--env", key)
	}

	args = append(args, image)
	args = append(args, command...)
	return args, envKeys, nil
}

func shouldAlwaysPull(image string) bool {
	ref := strings.TrimSpace(strings.ToLower(image))
	ref = strings.TrimPrefix(ref, "docker.io/")
	ref = strings.TrimPrefix(ref, "index.docker.io/")
	return ref == "wpkpda/dockerx" || strings.HasPrefix(ref, "wpkpda/dockerx:")
}

func prepareIdentityMounts(image, username, home, uidGID string) ([]mountSpec, func(), error) {
	parts := strings.SplitN(uidGID, ":", 2)
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("invalid uid:gid: %q", uidGID)
	}
	uid, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid uid in %q: %w", uidGID, err)
	}
	gid, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid gid in %q: %w", uidGID, err)
	}
	if uid == 0 {
		return nil, func() {}, nil
	}

	passwdBase, err := readImageFile(image, "/etc/passwd")
	if err != nil {
		return nil, nil, err
	}
	groupBase, err := readImageFile(image, "/etc/group")
	if err != nil {
		return nil, nil, err
	}
	shadowBase, err := readImageFile(image, "/etc/shadow")
	if err != nil {
		return nil, nil, err
	}

	passwdContent, groupContent, shadowContent := ensureRuntimeIdentity(passwdBase, groupBase, shadowBase, username, home, uid, gid)

	tmpDir, err := os.MkdirTemp("", "dockerx-identity-")
	if err != nil {
		return nil, nil, fmt.Errorf("create identity temp dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	passwdPath := filepath.Join(tmpDir, "passwd")
	groupPath := filepath.Join(tmpDir, "group")
	shadowPath := filepath.Join(tmpDir, "shadow")
	if err := os.WriteFile(passwdPath, []byte(passwdContent), 0o644); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("write passwd overlay: %w", err)
	}
	if err := os.WriteFile(groupPath, []byte(groupContent), 0o644); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("write group overlay: %w", err)
	}
	if err := os.WriteFile(shadowPath, []byte(shadowContent), 0o400); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("write shadow overlay: %w", err)
	}

	return []mountSpec{
		{src: passwdPath, dst: "/etc/passwd", readOnly: true},
		{src: groupPath, dst: "/etc/group", readOnly: true},
		{src: shadowPath, dst: "/etc/shadow", readOnly: true},
	}, cleanup, nil
}

func readImageFile(image, path string) (string, error) {
	cmd := exec.Command("docker", "run", "--rm", "--entrypoint", "cat", image, path)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("read %s from image %s: %w (%s)", path, image, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func ensureRuntimeIdentity(passwdBase, groupBase, shadowBase, username, home string, uid, gid int) (string, string, string) {
	if strings.TrimSpace(username) == "" {
		username = "dev"
	}

	passwdLines := splitLines(passwdBase)
	groupLines := splitLines(groupBase)
	shadowLines := splitLines(shadowBase)
	uidText := strconv.Itoa(uid)
	gidText := strconv.Itoa(gid)

	runtimeUser := username
	hasUID := false
	for _, line := range passwdLines {
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		if fields[2] == uidText {
			hasUID = true
			runtimeUser = fields[0]
			break
		}
	}
	if !hasUID {
		passwdLines = append(passwdLines, fmt.Sprintf("%s:x:%s:%s:%s user:%s:/bin/zsh", runtimeUser, uidText, gidText, runtimeUser, home))
	}

	hasGID := false
	for _, line := range groupLines {
		fields := strings.Split(line, ":")
		if len(fields) < 3 {
			continue
		}
		if fields[2] == gidText {
			hasGID = true
			break
		}
	}
	if !hasGID {
		groupLines = append(groupLines, fmt.Sprintf("%s:x:%s:", runtimeUser, gidText))
	}

	hasShadowUser := false
	for _, line := range shadowLines {
		fields := strings.Split(line, ":")
		if len(fields) < 1 {
			continue
		}
		if fields[0] == runtimeUser {
			hasShadowUser = true
			break
		}
	}
	if !hasShadowUser {
		shadowLines = append(shadowLines, fmt.Sprintf("%s::19793:0:99999:7:::", runtimeUser))
	}

	return joinLines(passwdLines), joinLines(groupLines), joinLines(shadowLines)
}

func splitLines(content string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func printPlan(image, workDir string, command []string, configMounts []mountSpec, envKeys, args []string) {
	fmt.Printf("Image: %s\n", image)
	fmt.Printf("Workdir: %s -> /app (rw)\n", workDir)
	if len(configMounts) == 0 {
		fmt.Println("Host config mounts: none")
	} else {
		fmt.Println("Host config mounts:")
		for i, m := range configMounts {
			stagePath := fmt.Sprintf("%s/%d", configStageRoot, i)
			fmt.Printf("  - %s -> %s (ro), copied to %s (rw)\n", m.src, stagePath, m.dst)
		}
	}
	if len(envKeys) == 0 {
		fmt.Println("Passthrough env: none")
	} else {
		fmt.Printf("Passthrough env: %s\n", strings.Join(envKeys, ", "))
	}
	fmt.Printf("Container command: %s\n", strings.Join(command, " "))
	fmt.Printf("Docker args: %s\n", strings.Join(args, " "))
}

func discoverHostConfigMounts(homeDir string, lookupEnv func(string) string, exists func(string) bool) []mountSpec {
	configHome := lookupEnv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(homeDir, ".config")
	}

	cacheHome := lookupEnv("XDG_CACHE_HOME")
	if cacheHome == "" {
		cacheHome = filepath.Join(homeDir, ".cache")
	}

	hfHome := lookupEnv("HF_HOME")
	if hfHome == "" {
		hfHome = filepath.Join(cacheHome, "huggingface")
	}

	codexHome := lookupEnv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(homeDir, ".codex")
	}

	candidates := []mountSpec{
		{src: codexHome, dst: containerHome + "/.codex", readOnly: true},
		{src: filepath.Join(configHome, "codex"), dst: containerHome + "/.config/codex", readOnly: true},
		{src: filepath.Join(homeDir, ".openai"), dst: containerHome + "/.openai", readOnly: true},
		{src: filepath.Join(configHome, "gh"), dst: containerHome + "/.config/gh", readOnly: true},
		{src: filepath.Join(configHome, "git"), dst: containerHome + "/.config/git", readOnly: true},
		{src: filepath.Join(homeDir, ".gitconfig"), dst: containerHome + "/.gitconfig", readOnly: true},
		{src: filepath.Join(homeDir, ".git-credentials"), dst: containerHome + "/.git-credentials", readOnly: true},
		{src: filepath.Join(homeDir, ".ssh"), dst: containerHome + "/.ssh", readOnly: true},
		{src: filepath.Join(homeDir, ".huggingface"), dst: containerHome + "/.huggingface", readOnly: true},
		{src: filepath.Join(configHome, "huggingface"), dst: containerHome + "/.config/huggingface", readOnly: true},
		{src: hfHome, dst: containerHome + "/.cache/huggingface", readOnly: true},
	}

	found := make([]mountSpec, 0, len(candidates))
	seenSrc := map[string]struct{}{}
	for _, c := range candidates {
		if c.src == "" || !exists(c.src) {
			continue
		}
		abs, err := filepath.Abs(c.src)
		if err != nil {
			continue
		}
		if _, seen := seenSrc[abs]; seen {
			continue
		}
		seenSrc[abs] = struct{}{}
		c.src = abs
		found = append(found, c)
	}

	slices.SortFunc(found, func(a, b mountSpec) int {
		return strings.Compare(a.dst, b.dst)
	})
	return found
}

func gatherPassthroughEnvKeys() []string {
	keys := []string{
		"TERM",
		"COLORTERM",
		"OPENAI_API_KEY",
		"OPENAI_BASE_URL",
		"ANTHROPIC_API_KEY",
		"GEMINI_API_KEY",
		"AZURE_OPENAI_API_KEY",
		"AZURE_OPENAI_ENDPOINT",
		"GITHUB_TOKEN",
		"GH_TOKEN",
		"HF_TOKEN",
		"HUGGINGFACEHUB_API_TOKEN",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"NO_PROXY",
	}

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			continue
		}
		out = append(out, key)
	}
	return out
}

func hostUIDGID() (string, bool) {
	if runtime.GOOS == "windows" {
		return "", false
	}

	u, err := user.Current()
	if err != nil {
		return "", false
	}
	if _, err := strconv.Atoi(u.Uid); err != nil {
		return "", false
	}
	if _, err := strconv.Atoi(u.Gid); err != nil {
		return "", false
	}
	return u.Uid + ":" + u.Gid, true
}

func formatMount(m mountSpec) string {
	mode := ""
	if m.readOnly {
		mode = ",readonly"
	}
	return "type=bind,src=" + m.src + ",dst=" + m.dst + mode
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getenv(key string) string {
	return os.Getenv(key)
}
