#!/bin/sh
# Usage: sandbox-fd-shim.sh <nearcore sandbox version> <output dir>
# Prints the path of a near-sandbox wrapper that preloads fd-limit-shim.so.
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
