#!/bin/sh

set -eu

repo_root="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

repo="${HOMEBREW_GITHUB_REPO:-kacy/chatbubbles}"

case "$#" in
	2)
		version="$(tr -d '\n' < VERSION)"
		arm64_sha="$1"
		amd64_sha="$2"
		;;
	3)
		version="$1"
		arm64_sha="$2"
		amd64_sha="$3"
		;;
	*)
	echo "usage: scripts/render-homebrew-formula.sh [version] <arm64-sha256> <amd64-sha256>" >&2
	echo "example: scripts/render-homebrew-formula.sh 0.1.3 <arm64-sha> <amd64-sha>" >&2
	exit 1
		;;
esac

release_tag="v$version"
release_base_url="https://github.com/$repo/releases/download/$release_tag"
arm64_url="$release_base_url/chatbubbles_${version}_darwin_arm64.tar.gz"
amd64_url="$release_base_url/chatbubbles_${version}_darwin_amd64.tar.gz"

cat <<EOF
class Chatbubbles < Formula
  desc "Small HTTPS API for iMessage over Tailscale"
  homepage "https://github.com/$repo"
  version "$version"

  depends_on "imsg"
  depends_on macos: :sonoma

  if Hardware::CPU.arm?
    url "$arm64_url"
    sha256 "$arm64_sha"
  else
    url "$amd64_url"
    sha256 "$amd64_sha"
  end

  def install
    bin.install "chatbubbles", "chatbubbles-cli"
    pkgshare.install "README.md"
  end

  def caveats
    <<~EOS
      chatbubbles still needs a few manual bits on top of the brew install:

      - install and sign in to tailscale on this mac
      - make sure Messages is signed in and working
      - grant full disk access to the terminal or daemon you use to run chatbubbles
      - grant automation permissions when macOS prompts for Messages control

      runtime state lives in:
        ~/.local/share/chatbubbles
    EOS
  end

  test do
    assert_match "listen address", shell_output("#{bin}/chatbubbles -h")
    assert_match "control socket path", shell_output("#{bin}/chatbubbles-cli -h 2>&1", 1)
  end
end
EOF
