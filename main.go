package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	cfg := parseCLI()
	if cfg.showVersion {
		fmt.Println(version)
		return 0
	}

	if err := launchDockerx(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "dockerx: %v\n", err)
		return 1
	}
	return 0
}

func parseCLI() cliConfig {
	imageDefault := os.Getenv("DOCKERX_IMAGE")
	if imageDefault == "" {
		imageDefault = "wpkpda/dockerx:latest"
	}

	cfg := cliConfig{
		image: imageDefault,
		shell: "zsh",
	}

	flag.StringVar(&cfg.image, "image", cfg.image, "Docker image to run")
	flag.StringVar(&cfg.shell, "shell", cfg.shell, "Shell to launch when no command is provided")
	flag.BoolVar(&cfg.noPull, "no-pull", false, "Disable forced pull policy for dockerx images")
	flag.BoolVar(&cfg.noConfig, "no-config", false, "Disable automatic host config mounts")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "Print docker command without executing it")
	flag.BoolVar(&cfg.verbose, "verbose", false, "Print resolved mounts and environment passthrough")
	flag.BoolVar(&cfg.showVersion, "version", false, "Print dockerx version")
	flag.Parse()

	cfg.command = flag.Args()
	return cfg
}
