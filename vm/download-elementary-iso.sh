#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./vm/download-elementary-iso.sh [--arch amd64|arm64] [--output PATH] [--print-url]

Downloads the latest elementary OS ISO by scraping the current download page.

Options:
  --arch amd64|arm64   ISO architecture to fetch. Default: amd64
  --output PATH        Destination file path. Default: ~/Downloads/elementaryos-<arch>.iso
  --print-url          Resolve and print the current ISO URL without downloading it
  -h, --help           Show this help text
EOF
}

arch="amd64"
output_path=""
print_url="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --arch)
      arch="$2"
      shift 2
      ;;
    --output)
      output_path="$2"
      shift 2
      ;;
    --print-url)
      print_url="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

case "$arch" in
  amd64|arm64)
    ;;
  *)
    echo "Unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

if [[ -z "$output_path" ]]; then
  output_path="$HOME/Downloads/elementaryos-$arch.iso"
fi

page_html="$(curl -fsSL https://elementary.io/)"
relative_url="$({ printf '%s' "$page_html" | grep -Eo "/download/[^\"[:space:]]*elementaryos-[0-9.]+-stable-${arch}\.[0-9]+\.iso" | head -n 1; } || true)"

if [[ -z "$relative_url" ]]; then
  echo "Could not locate the latest elementary OS $arch ISO URL." >&2
  exit 1
fi

download_url="https://elementary.io${relative_url}"

if [[ "$print_url" == "true" ]]; then
  printf '%s\n' "$download_url"
  exit 0
fi

mkdir -p "$(dirname "$output_path")"
temp_path="${output_path}.part"

echo "Downloading elementary OS $arch ISO..."
echo "Source: $download_url"
echo "Target: $output_path"

curl --fail --location --progress-bar --output "$temp_path" "$download_url"

if file "$temp_path" | grep -qi 'HTML'; then
  rm -f "$temp_path"
  echo "Download did not return an ISO image. elementary.io redirected the request to HTML." >&2
  echo "Use the browser download flow on https://elementary.io/ and save the ISO manually." >&2
  exit 1
fi

mv "$temp_path" "$output_path"

echo "Download complete: $output_path"