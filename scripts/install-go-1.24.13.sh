#!/usr/bin/env sh
set -eu
version=1.24.13
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$os/$arch" in
  linux/x86_64) platform=linux-amd64; expected=1fc94b57134d51669c72173ad5d49fd62afb0f1db9bf3f798fd98ee423f8d730 ;;
  linux/aarch64|linux/arm64) platform=linux-arm64; expected=74d97be1cc3a474129590c67ebf748a96e72d9f3a2b6fef3ed3275de591d49b3 ;;
  darwin/x86_64) platform=darwin-amd64; expected=6cc6549b06725220b342b740497ffd24e0ebdcef75781a77931ca199f46ad781 ;;
  darwin/arm64) platform=darwin-arm64; expected=f282d882c3353485e2fc6c634606d85caf36e855167d59b996dbeae19fa7629a ;;
  *) echo "Unsupported platform: $os/$arch" >&2; exit 1 ;;
esac
archive="go${version}.${platform}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fL "https://go.dev/dl/$archive" -o "$tmp/$archive"
if command -v sha256sum >/dev/null 2>&1; then
  printf '%s  %s\n' "$expected" "$tmp/$archive" | sha256sum -c -
else
  printf '%s  %s\n' "$expected" "$tmp/$archive" | shasum -a 256 -c -
fi
install_root="${HOME}/.local/go-${version}"
mkdir -p "$install_root"
tar -xzf "$tmp/$archive" -C "$install_root"
printf '\nGo %s installed at %s/go/bin/go\n' "$version" "$install_root"
printf 'Add this to your shell profile: export PATH="%s/go/bin:$PATH"\n' "$install_root"
"$install_root/go/bin/go" version
