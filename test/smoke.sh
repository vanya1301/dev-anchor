#!/bin/sh
# Smoke test: runs the built `da` binary inside a fresh, unmodified distro
# container (imitating a brand-new machine) and verifies `da pull` actually
# installs a tool via the real package manager and creates the expected
# symlink. Invoked by the `smoke` job in .github/workflows/build.yml as:
#
#   sh /work/test/smoke.sh <apt|dnf|pacman> <label>
set -eu

manager="$1"
label="${2:-$1}"

# These base images run as root with no `sudo` binary; da's linux
# installers shell out to `sudo <manager> ...` regardless. Since we're
# already root, a no-op shim is correct here (this is a CI-only
# accommodation, not something da itself should assume in production).
if ! command -v sudo >/dev/null 2>&1; then
	printf '#!/bin/sh\nexec "$@"\n' >/usr/local/bin/sudo
	chmod +x /usr/local/bin/sudo
fi

# A bare container has no package index yet — a real fresh machine's cloud
# image is normally provisioned with one already. Refresh it once, same as
# any first-boot setup would.
case "$manager" in
apt)
	apt-get update -qq
	;;
dnf)
	dnf makecache -q || true
	;;
pacman)
	pacman-key --init >/dev/null 2>&1 || true
	pacman-key --populate archlinux >/dev/null 2>&1 || true
	pacman -Sy --noconfirm
	;;
*)
	echo "unknown manager: $manager" >&2
	exit 1
	;;
esac

/work/da pull /work/test/fixtures/smoke --yes

if ! command -v tree >/dev/null 2>&1; then
	echo "FAIL ($label): tree was not installed" >&2
	exit 1
fi

link="$HOME/.testrc"
if [ ! -L "$link" ]; then
	echo "FAIL ($label): $link is not a symlink" >&2
	exit 1
fi

target="$(readlink "$link")"
case "$target" in
*/test/fixtures/smoke/configs/testrc) ;;
*)
	echo "FAIL ($label): $link -> $target (unexpected target)" >&2
	exit 1
	;;
esac

echo "SMOKE OK ($label): tree installed, .testrc -> $target"
