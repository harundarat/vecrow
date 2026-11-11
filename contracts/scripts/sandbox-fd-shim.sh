#!/bin/sh
# Usage: sandbox-fd-shim.sh <nearcore sandbox version> <output dir>
#
# Prints the path of a near-sandbox wrapper for environments whose hard
# open-file limit is below the 65,535 that nearcore requires. The wrapper runs
# the sandbox binary with fd-limit-shim.so preloaded; point
# NEAR_SANDBOX_BIN_PATH at it. `make test-sandbox` does this automatically
# when `ulimit -H -n` is too low on Linux.
set -eu

version="$1"
dir="$2"
here="$(cd "$(dirname "$0")" && pwd)"
platform="Linux-$(uname -m)"
bin="$dir/near-sandbox-$version"

mkdir -p "$dir"
if [ ! -x "$bin" ]; then
    url="https://s3-us-west-1.amazonaws.com/build.nearprotocol.com/nearcore/$platform/$version/near-sandbox.tar.gz"
    echo "downloading $url" >&2
    curl -fsSL "$url" | tar -xzO "$platform/near-sandbox" > "$bin.part"
    chmod +x "$bin.part"
    mv "$bin.part" "$bin"
fi

cc -shared -fPIC -O2 -o "$dir/fd-limit-shim.so" "$here/fd-limit-shim.c" -ldl

cat > "$dir/near-sandbox" <<WRAPPER
#!/bin/sh
exec env LD_PRELOAD="$dir/fd-limit-shim.so" "$bin" "\$@"
WRAPPER
chmod +x "$dir/near-sandbox"
echo "$dir/near-sandbox"
