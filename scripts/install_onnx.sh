#!/bin/sh
set -eu

die() {
	printf 'Error: %s\n' "$*" >&2
	exit 1
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

need_cmd uname
need_cmd mktemp
need_cmd tar
need_cmd cp
need_cmd find
need_cmd mkdir
need_cmd chmod

if command -v curl >/dev/null 2>&1; then
	fetch() {
		curl -fLsS --retry 5 --retry-delay 2 --retry-connrefused -o "$1" "$2"
	}
elif command -v wget >/dev/null 2>&1; then
	fetch() {
		wget -q -O "$1" "$2"
	}
else
	die "missing command: curl or wget"
fi

os_name=$(uname -s)
arch_name=$(uname -m)

[ "$os_name" = Linux ] || die "Linux only (got ${os_name})"

case "$arch_name" in
x86_64 | amd64) ;;
*) die "x86_64/amd64 only (got ${arch_name})" ;;
esac

script_path=$0
case "$script_path" in
/*) ;;
*) script_path=$(pwd)/$script_path ;;
esac

script_dir=$(CDPATH='' cd -- "$(dirname -- "$script_path")" && pwd -P)
root_dir=$(CDPATH='' cd -- "${script_dir}/.." && pwd -P)
models_dir=${MODELS_DIR:-"${root_dir}/models"}
ort_version=${ORT_VERSION:-1.28.0}
ort_sha256=${ORT_SHA256:-}

archive="onnxruntime-linux-x64-${ort_version}.tgz"
url="https://github.com/microsoft/onnxruntime/releases/download/v${ort_version}/${archive}"

tmp_dir=$(mktemp -d)
cleanup() {
	status=$?
	rm -rf -- "${tmp_dir}"
	return "$status"
}
trap cleanup EXIT INT TERM HUP

mkdir -p -- "${models_dir}"
archive_path="${tmp_dir}/${archive}"

fetch "${archive_path}" "${url}"

if [ -n "${ort_sha256}" ]; then
	if command -v sha256sum >/dev/null 2>&1; then
		printf '%s  %s\n' "${ort_sha256}" "${archive_path}" | sha256sum -c - >/dev/null
	elif command -v shasum >/dev/null 2>&1; then
		printf '%s  %s\n' "${ort_sha256}" "${archive_path}" | shasum -a 256 -c - >/dev/null
	else
		die "ORT_SHA256 set but sha256sum/shasum not available"
	fi
fi

tar -xzf "${archive_path}" -C "${tmp_dir}"

found=
for candidate in \
	"${tmp_dir}/onnxruntime-linux-x64-${ort_version}/lib/libonnxruntime.so.${ort_version}" \
	"${tmp_dir}/onnxruntime-linux-x64-${ort_version}/lib/libonnxruntime.so"; do
	if [ -f "${candidate}" ]; then
		found=${candidate}
		break
	fi
done

if [ -z "${found}" ]; then
	found=$(find "${tmp_dir}" -type f -name "libonnxruntime.so.${ort_version}" -print 2>/dev/null)
	if [ -z "${found}" ]; then
		found=$(find "${tmp_dir}" -type f -name 'libonnxruntime.so' -print 2>/dev/null)
	fi
	found=$(printf '%s\n' "${found}" | sed -n '1p')
fi

[ -n "${found}" ] || die "libonnxruntime.so not found in archive"

target="${models_dir}/libonnxruntime.so"
tmp_target="${target}.tmp.$$"
cp -f -- "${found}" "${tmp_target}"
chmod 0644 -- "${tmp_target}"
mv -f -- "${tmp_target}" "${target}"

printf 'Installed %s\n' "${target}"
printf 'Set ONNXRUNTIME_SHARED_LIBRARY=%s\n' "${target}"