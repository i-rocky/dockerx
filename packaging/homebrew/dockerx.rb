class Dockerx < Formula
  desc "Hardened Docker dev environment launcher"
  homepage "https://github.com/__REPO__"
  version "__VERSION__"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/__REPO__/releases/download/v__VERSION__/dockerx-darwin-aarch64-v__VERSION__.tar.gz"
      sha256 "__DARWIN_ARM64_SHA__"
    else
      url "https://github.com/__REPO__/releases/download/v__VERSION__/dockerx-darwin-x86_64-v__VERSION__.tar.gz"
      sha256 "__DARWIN_AMD64_SHA__"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/__REPO__/releases/download/v__VERSION__/dockerx-linux-aarch64-v__VERSION__.tar.gz"
      sha256 "__LINUX_ARM64_SHA__"
    else
      url "https://github.com/__REPO__/releases/download/v__VERSION__/dockerx-linux-x86_64-v__VERSION__.tar.gz"
      sha256 "__LINUX_AMD64_SHA__"
    end
  end

  def install
    bin.install "dockerx"
  end
end
