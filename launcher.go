package main

import (
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

type cliConfig struct {
	image       string
	shell       string
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

	command := cfg.command
	if len(command) == 0 {
		command = []string{cfg.shell}
	}

	args, envKeys, err := buildDockerArgs(cfg.image, workDir, command, configMounts)
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

func buildDockerArgs(image, workDir string, command []string, configMounts []mountSpec) ([]string, []string, error) {
	if strings.Contains(workDir, ",") {
		return nil, nil, fmt.Errorf("current directory contains an unsupported comma: %q", workDir)
	}

	args := []string{"run", "--rm", "-i"}
	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		args = append(args, "-t")
	}

	args = append(args,
		"--read-only",
		"--security-opt", "no-new-privileges",
		"--cap-drop", "ALL",
		"--mount", formatMount(mountSpec{src: workDir, dst: "/w", readOnly: false}),
		"--tmpfs", "/tmp:mode=1777",
		"--tmpfs", "/run:mode=755",
		"--tmpfs", "/var/tmp:mode=1777",
		"--tmpfs", containerHome+":mode=755",
		"--workdir", "/w",
		"--env", "HOME="+containerHome,
		"--env", "USER=dev",
		"--env", "CODEX_HOME="+containerHome+"/.codex",
	)

	if uidGID, ok := hostUIDGID(); ok {
		args = append(args, "--user", uidGID)
	}

	for _, m := range configMounts {
		if strings.Contains(m.src, ",") {
			return nil, nil, fmt.Errorf("mount source contains an unsupported comma: %q", m.src)
		}
		args = append(args, "--mount", formatMount(m))
	}

	envKeys := gatherPassthroughEnvKeys()
	for _, key := range envKeys {
		args = append(args, "--env", key)
	}

	args = append(args, image)
	args = append(args, command...)
	return args, envKeys, nil
}

func printPlan(image, workDir string, command []string, configMounts []mountSpec, envKeys, args []string) {
	fmt.Printf("Image: %s\n", image)
	fmt.Printf("Workdir: %s -> /w (rw)\n", workDir)
	if len(configMounts) == 0 {
		fmt.Println("Host config mounts: none")
	} else {
		fmt.Println("Host config mounts:")
		for _, m := range configMounts {
			mode := "rw"
			if m.readOnly {
				mode = "ro"
			}
			fmt.Printf("  - %s -> %s (%s)\n", m.src, m.dst, mode)
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
