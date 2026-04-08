class Kairos < Formula
  desc "Personal AI Runtime — local LLM platform with memory, RAG, and MCP"
  homepage "https://github.com/jxroo/kairos"
  url "https://github.com/jxroo/kairos/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  license "MIT"
  head "https://github.com/jxroo/kairos.git", branch: "main"

  depends_on "go" => :build
  depends_on "node" => :build
  depends_on "rust" => :build

  def install
    ENV["CGO_LDFLAGS_ALLOW"] = "-Wl,-rpath,.*"
    system "make", "build", "VERSION=v#{version}"
    bin.install "kairos"
    lib.install Dir["vecstore/target/release/libvecstore.{dylib,so}"]
  end

  def post_install
    (var/"kairos").mkpath
  end

  test do
    assert_match "v#{version}", shell_output("#{bin}/kairos version")
  end
end
