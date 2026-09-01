# Install

One package per package manager, native one first, from-source last.

## Arch Linux, from the AUR

```bash
git clone https://aur.archlinux.org/backup-git-repos-bin.git
cd backup-git-repos-bin
makepkg -si
```

Or with an AUR helper: `paru -S backup-git-repos-bin` /
`yay -S backup-git-repos-bin`. `-bin` because it installs the same
prebuilt binary the GitHub release ships, not a from-source build --
the AUR's own naming convention for that.

## Debian/Ubuntu (.deb) and Fedora/RHEL (.rpm)

Download the matching `.deb`/`.rpm` from the [latest
release](https://github.com/alrayyes/backup-git-repos/releases/latest)
(`linux_amd64`/`linux_arm64`), then:

```bash
# Debian/Ubuntu
sudo dpkg -i backup-git-repos_*_linux_amd64.deb

# Fedora/RHEL
sudo rpm -i backup-git-repos_*_linux_amd64.rpm
```

Neither is hosted in an apt/yum repository. It's a package to download
and install once, nothing more to add.

## Docker

A natural fit for a scheduled job -- `git` and all, no language runtime to
install. Runs as UID/GID `1000`, not root, so the destination directory
needs to be writable by that UID on the host:

```bash
mkdir -p /srv/backups/git && chown 1000:1000 /srv/backups/git
docker run --rm \
  --read-only --tmpfs /tmp \
  --cap-drop=ALL --security-opt=no-new-privileges \
  --memory=512m --cpus=2 \
  -v /srv/backups/git:/srv/backups/git -v ./config.yaml:/config.yaml:ro \
  ghcr.io/alrayyes/backup-git-repos:latest run --config /config.yaml
```

The image needs no capability beyond the default network access every
container gets, so `--cap-drop=ALL` alone is the whole capability line,
with nothing to add back. `--tmpfs /tmp` matters specifically for
`--archive`: an already-archived repository mirrors into a scratch
directory under `/tmp` before being written out as a `.tar.gz` (see
[Usage](README.md#usage)), which needs somewhere writable once
`--read-only` locks the rest of the image's own filesystem. Size the
mount for your largest archived repository if the default (half the
host's RAM) isn't enough, and size `--memory`/`--cpus` for how many
repositories `--concurrency` mirrors at once.

Images are multi-arch (`linux/amd64`, `linux/arm64`), tagged `latest` and
per version, and published alongside every release at
[ghcr.io/alrayyes/backup-git-repos](https://github.com/alrayyes/backup-git-repos/pkgs/container/backup-git-repos).

## Nix and NixOS

```bash
nix run github:alrayyes/backup-git-repos       # try it
nix profile install github:alrayyes/backup-git-repos  # keep it
```

No hosted binary cache -- a first run/install builds from source, same
tradeoff as every from-source path here.

## `go install`

```bash
go install github.com/alrayyes/backup-git-repos/cmd/backup-git-repos@latest
```

## From source

```bash
git clone https://github.com/alrayyes/backup-git-repos.git
cd backup-git-repos
go build -o backup-git-repos ./cmd/backup-git-repos
```

Released binaries for every platform preceding this are also attached directly to
each [GitHub release](https://github.com/alrayyes/backup-git-repos/releases)
as plain `.tar.gz` archives, for anywhere none of those fits.
