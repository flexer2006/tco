#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

die(){ echo "Error: $*" >&2; exit 1; }
trap 'die "Unexpected error on line $LINENO"' ERR

for c in curl tar uname mktemp cp find; do
  command -v "$c" >/dev/null || die "Missing command: $c"
done

[[ "$(uname -s)" == Linux ]] || die "Linux only"
case "$(uname -m)" in x86_64|amd64) ;; *) die "x86_64/amd64 only" ;; esac

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
root_dir="$(cd -- "${script_dir}/.." && pwd -P)"
models_dir="${root_dir}/models"
ort_version="${ORT_VERSION:-1.24.1}"

archive="onnxruntime-linux-x64-${ort_version}.tgz"
url="https://github.com/microsoft/onnxruntime/releases/download/v${ort_version}/${archive}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf -- "${tmp_dir}"' EXIT
mkdir -p -- "${models_dir}"

curl -fLsS --retry 5 --retry-delay 2 --retry-connrefused \
  -o "${tmp_dir}/${archive}" "${url}"

tar -xzf "${tmp_dir}/${archive}" -C "${tmp_dir}"

target="${models_dir}/libonnxruntime.so"
found="$(find "${tmp_dir}" -type f \( \
  -name "libonnxruntime.so.${ort_version}" -o -name 'libonnxruntime.so' \) \
  -print -quit)"

[[ -n "${found}" ]] || die "libonnxruntime.so not found"
cp -f -- "${found}" "${target}"
chmod 0644 -- "${target}"

echo "Installed ${target}"
echo "Set ONNXRUNTIME_SHARED_LIBRARY=./models/libonnxruntime.so"