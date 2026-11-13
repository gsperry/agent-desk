#!/bin/sh
# Renders the Homebrew formula and cask for a release from its SHA256SUMS.
#
#   scripts/homebrew.sh 0.1.0 [SHA256SUMS] [out-dir]
#
# Without a SHA256SUMS path it downloads the one attached to release v<version>
# with gh. The rendered agent-desk.rb and agent-desk-app.rb go to out-dir
# (packaging/homebrew/out by default), ready to copy into the tap's Formula/
# and Casks/ folders.
set -eu
version=${1:?usage: scripts/homebrew.sh <version> [SHA256SUMS] [out-dir]}
case $version in v*) version=${version#v} ;; esac
here=$(cd "$(dirname "$0")/.." && pwd)
sums=${2:-}
out=${3:-$here/packaging/homebrew/out}
if [ -z "$sums" ]; then
	tmp=$(mktemp -d)
	trap 'rm -r "$tmp"' EXIT
	gh release download "v$version" --repo gsperry/agent-desk --pattern SHA256SUMS --dir "$tmp"
	sums=$tmp/SHA256SUMS
fi
sha() {
	value=$(awk -v f="$1" '$2 == f { print $1 }' "$sums")
	[ -n "$value" ] || { echo "no checksum for $1 in $sums" >&2; exit 1; }
	echo "$value"
}
mkdir -p "$out"
render() {
	sed -e "s/@VERSION@/$version/g" \
		-e "s/@SHA_darwin_arm64@/$(sha "agent-desk_${version}_darwin_arm64.tar.gz")/" \
		-e "s/@SHA_darwin_amd64@/$(sha "agent-desk_${version}_darwin_amd64.tar.gz")/" \
		-e "s/@SHA_linux_arm64@/$(sha "agent-desk_${version}_linux_arm64.tar.gz")/" \
		-e "s/@SHA_linux_amd64@/$(sha "agent-desk_${version}_linux_amd64.tar.gz")/" \
		-e "s/@SHA_macos@/$(sha "Agent-Desk_${version}_macos.zip")/" \
		"$here/packaging/homebrew/$1.in" > "$out/$1"
}
render agent-desk.rb
render agent-desk-app.rb
echo "wrote $out/agent-desk.rb and $out/agent-desk-app.rb"
