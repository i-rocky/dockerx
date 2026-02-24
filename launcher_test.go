package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDiscoverHostConfigMounts(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, ".config")
	cacheHome := filepath.Join(home, ".cache")

	mustMkdirAll(t, filepath.Join(home, ".codex"))
	mustMkdirAll(t, filepath.Join(configHome, "gh"))
	mustMkdirAll(t, filepath.Join(home, ".ssh"))
	mustMkdirAll(t, filepath.Join(cacheHome, "huggingface"))
	mustWriteFile(t, filepath.Join(home, ".gitconfig"))

	env := map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"XDG_CACHE_HOME":  cacheHome,
	}

	mounts := discoverHostConfigMounts(home, func(key string) string {
		return env[key]
	}, pathExists)

	if len(mounts) == 0 {
		t.Fatal("expected mounts, got none")
	}

	assertMount(t, mounts, filepath.Join(home, ".codex"), containerHome+"/.codex", true)
	assertMount(t, mounts, filepath.Join(configHome, "gh"), containerHome+"/.config/gh", true)
	assertMount(t, mounts, filepath.Join(home, ".ssh"), containerHome+"/.ssh", true)
	assertMount(t, mounts, filepath.Join(cacheHome, "huggingface"), containerHome+"/.cache/huggingface", true)
	assertMount(t, mounts, filepath.Join(home, ".gitconfig"), containerHome+"/.gitconfig", true)
}

func TestBuildDockerArgsIncludesSecurityDefaults(t *testing.T) {
	args, _, err := buildDockerArgs("repo/image:latest", "/tmp/work", []string{"zsh"}, nil, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, required := range []string{
		"run",
		"--read-only",
		"--cap-drop",
		"ALL",
		"--workdir",
		"/app",
		"repo/image:latest",
		"zsh",
	} {
		if !slices.Contains(args, required) {
			t.Fatalf("missing %q in args: %v", required, args)
		}
	}

	wantWorkMount := "--mount"
	if !slices.Contains(args, wantWorkMount) {
		t.Fatalf("expected %q in args", wantWorkMount)
	}
	if containsSubstring(args, "no-new-privileges") {
		t.Fatalf("did not expect no-new-privileges in args: %v", args)
	}
	if !containsSubstring(args, "type=bind,src=/tmp/work,dst=/app") {
		t.Fatalf("missing /app mount in args: %v", args)
	}
}

func TestBuildDockerArgsPullAlwaysForDockerxImage(t *testing.T) {
	args, _, err := buildDockerArgs("wpkpda/dockerx:latest", "/tmp/work", []string{"zsh"}, nil, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsPair(args, "--pull", "always") {
		t.Fatalf("expected --pull always in args: %v", args)
	}
}

func TestBuildDockerArgsDoesNotForcePullForOtherImages(t *testing.T) {
	args, _, err := buildDockerArgs("repo/image:latest", "/tmp/work", []string{"zsh"}, nil, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containsPair(args, "--pull", "always") {
		t.Fatalf("did not expect --pull always in args: %v", args)
	}
}

func TestBuildDockerArgsNoPullSkipsAlwaysPolicy(t *testing.T) {
	args, _, err := buildDockerArgs("wpkpda/dockerx:latest", "/tmp/work", []string{"zsh"}, nil, nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containsPair(args, "--pull", "always") {
		t.Fatalf("did not expect --pull always in args with no-pull: %v", args)
	}
}

func TestBuildDockerArgsStagesConfigMounts(t *testing.T) {
	configMounts := []mountSpec{
		{src: "/host/.codex", dst: containerHome + "/.codex", readOnly: true},
	}

	args, _, err := buildDockerArgs("repo/image:latest", "/tmp/work", []string{"zsh"}, configMounts, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsSubstring(args, "type=bind,src=/host/.codex,dst=/tmp/dockerx-config/0,readonly") {
		t.Fatalf("missing staged read-only mount: %v", args)
	}
	if !slices.Contains(args, "--env") {
		t.Fatalf("expected --env entries: %v", args)
	}
	if !containsSubstring(args, "DOCKERX_CONFIG_SRC_0=/tmp/dockerx-config/0") {
		t.Fatalf("missing DOCKERX_CONFIG_SRC_0 env: %v", args)
	}
	if !containsSubstring(args, "DOCKERX_CONFIG_DST_0="+containerHome+"/.codex") {
		t.Fatalf("missing DOCKERX_CONFIG_DST_0 env: %v", args)
	}
	if !containsSubstring(args, "DOCKERX_CONFIG_COUNT=1") {
		t.Fatalf("missing DOCKERX_CONFIG_COUNT env: %v", args)
	}
}

func TestBuildDockerArgsRejectsCommaInWorkdir(t *testing.T) {
	_, _, err := buildDockerArgs("repo/image:latest", "/tmp/bad,path", []string{"zsh"}, nil, nil, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported comma") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGatherPassthroughEnvKeys(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test")
	t.Setenv("GH_TOKEN", "token")
	t.Setenv("HF_TOKEN", "")

	keys := gatherPassthroughEnvKeys()
	if !slices.Contains(keys, "OPENAI_API_KEY") {
		t.Fatalf("expected OPENAI_API_KEY in %v", keys)
	}
	if !slices.Contains(keys, "GH_TOKEN") {
		t.Fatalf("expected GH_TOKEN in %v", keys)
	}
	if slices.Contains(keys, "HF_TOKEN") {
		t.Fatalf("did not expect HF_TOKEN in %v", keys)
	}
}

func TestEnsureRuntimeIdentityAddsMissingEntries(t *testing.T) {
	passwdBase := "root:x:0:0:root:/root:/bin/bash\n"
	groupBase := "root:x:0:\n"
	shadowBase := "root:*:19793:0:99999:7:::\n"

	passwdOut, groupOut, shadowOut := ensureRuntimeIdentity(passwdBase, groupBase, shadowBase, "dev", "/home/dev", 501, 20)
	if !strings.Contains(passwdOut, "dev:x:501:20:dev user:/home/dev:/bin/zsh") {
		t.Fatalf("missing runtime passwd entry: %s", passwdOut)
	}
	if !strings.Contains(groupOut, "dev:x:20:") {
		t.Fatalf("missing runtime group entry: %s", groupOut)
	}
	if !strings.Contains(shadowOut, "dev::19793:0:99999:7:::") {
		t.Fatalf("missing runtime shadow entry: %s", shadowOut)
	}
}

func TestEnsureRuntimeIdentityUsesExistingUIDUser(t *testing.T) {
	passwdBase := "root:x:0:0:root:/root:/bin/bash\nrocky:x:501:20:Rocky:/home/rocky:/bin/zsh\n"
	groupBase := "root:x:0:\nstaff:x:20:\n"
	shadowBase := "root:*:19793:0:99999:7:::\nrocky:*:19793:0:99999:7:::\n"

	passwdOut, groupOut, shadowOut := ensureRuntimeIdentity(passwdBase, groupBase, shadowBase, "dev", "/home/dev", 501, 20)
	if strings.Contains(passwdOut, "dev:x:501:20:") {
		t.Fatalf("did not expect duplicate dev entry: %s", passwdOut)
	}
	if strings.Count(passwdOut, "rocky:x:501:20:") != 1 {
		t.Fatalf("expected existing rocky entry once: %s", passwdOut)
	}
	if strings.Count(groupOut, "staff:x:20:") != 1 {
		t.Fatalf("expected existing gid entry once: %s", groupOut)
	}
	if strings.Count(shadowOut, "rocky:*:19793:0:99999:7:::") != 1 {
		t.Fatalf("expected existing shadow entry once: %s", shadowOut)
	}
}

func assertMount(t *testing.T, mounts []mountSpec, src, dst string, readOnly bool) {
	t.Helper()
	absSrc, err := filepath.Abs(src)
	if err != nil {
		t.Fatalf("resolve abs path: %v", err)
	}

	for _, m := range mounts {
		if m.src == absSrc && m.dst == dst && m.readOnly == readOnly {
			return
		}
	}
	t.Fatalf("expected mount not found: src=%s dst=%s ro=%t mounts=%v", absSrc, dst, readOnly, mounts)
}

func containsSubstring(values []string, part string) bool {
	for _, v := range values {
		if strings.Contains(v, part) {
			return true
		}
	}
	return false
}

func containsPair(values []string, first, second string) bool {
	for i := 0; i+1 < len(values); i++ {
		if values[i] == first && values[i+1] == second {
			return true
		}
	}
	return false
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
