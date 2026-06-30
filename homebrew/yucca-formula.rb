# typed: false
# frozen_string_literal: true

# Source template for the Homebrew formula (the `yucca` CLI). The release
# workflow fills __VERSION__ and the per-platform __SHA_*__ from the release
# checksums, then commits the result to kobylinski/homebrew-tap as
# Formula/yucca.rb.
class Yucca < Formula
  desc "Local secret management for AI coding assistants"
  homepage "https://github.com/kobylinski/yucca"
  version "__VERSION__"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/kobylinski/yucca/releases/download/v#{version}/yucca_#{version}_darwin_arm64.tar.gz"
      sha256 "__SHA_DARWIN_ARM64__"
    end
    on_intel do
      url "https://github.com/kobylinski/yucca/releases/download/v#{version}/yucca_#{version}_darwin_amd64.tar.gz"
      sha256 "__SHA_DARWIN_AMD64__"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/kobylinski/yucca/releases/download/v#{version}/yucca_#{version}_linux_arm64.tar.gz"
      sha256 "__SHA_LINUX_ARM64__"
    end
    on_intel do
      url "https://github.com/kobylinski/yucca/releases/download/v#{version}/yucca_#{version}_linux_amd64.tar.gz"
      sha256 "__SHA_LINUX_AMD64__"
    end
  end

  def install
    bin.install "yucca"
  end

  service do
    run [opt_bin/"yucca", "daemon", "--idle-timeout", "0"]
    keep_alive true
    log_path var/"log/yucca.log"
    error_log_path var/"log/yucca.log"
  end

  def caveats
    <<~EOS
      Start the Yucca daemon (launchd-managed — auto-start at login, auto-restart):
        brew services start yucca

      Then register a project:
        cd your-project && yucca init
    EOS
  end

  test do
    system "#{bin}/yucca", "version"
  end
end
