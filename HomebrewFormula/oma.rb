class Oma < Formula
  desc "Open Managed Agents - self-hosted agent platform"
  homepage "https://github.com/domuk-k/open-managed-agents"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/domuk-k/open-managed-agents/releases/download/v#{version}/oma_#{version}_darwin_arm64.tar.gz"
      # sha256 will be filled by goreleaser
    end
    on_intel do
      url "https://github.com/domuk-k/open-managed-agents/releases/download/v#{version}/oma_#{version}_darwin_amd64.tar.gz"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/domuk-k/open-managed-agents/releases/download/v#{version}/oma_#{version}_linux_arm64.tar.gz"
    end
    on_intel do
      url "https://github.com/domuk-k/open-managed-agents/releases/download/v#{version}/oma_#{version}_linux_amd64.tar.gz"
    end
  end

  def install
    bin.install "oma"
  end

  test do
    assert_match "Open Managed Agents", shell_output("#{bin}/oma --help")
  end
end
