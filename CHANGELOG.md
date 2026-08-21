# Changelog

## [1.4.2](https://github.com/alrayyes/backup-git-repos/compare/v1.4.1...v1.4.2) (2026-08-21)


### Bug Fixes

* bump the pinned apk git version, alpine 3.24.1 dropped 2.49.1-r0 ([#76](https://github.com/alrayyes/backup-git-repos/issues/76)) ([6f87794](https://github.com/alrayyes/backup-git-repos/commit/6f87794d258e37f34d91e05a15a54a3415e35ff1)), closes [#75](https://github.com/alrayyes/backup-git-repos/issues/75)

## [1.4.1](https://github.com/alrayyes/backup-git-repos/compare/v1.4.0...v1.4.1) (2026-08-21)


### Bug Fixes

* bump the pinned Forgejo test image, 16.0.2 was pruned from the registry ([#74](https://github.com/alrayyes/backup-git-repos/issues/74)) ([1cd4eae](https://github.com/alrayyes/backup-git-repos/commit/1cd4eae17c4dff20fb46c993851ccff34a93b8d7)), closes [#73](https://github.com/alrayyes/backup-git-repos/issues/73)
* **deps:** bump alpine from 3.22.2 to 3.24.1 ([#69](https://github.com/alrayyes/backup-git-repos/issues/69)) ([e30ad46](https://github.com/alrayyes/backup-git-repos/commit/e30ad463026a89beaf83c3ce9a4987c7fa8990df))
* log which repository skip_mirrors excludes ([#65](https://github.com/alrayyes/backup-git-repos/issues/65)) ([a50c9a2](https://github.com/alrayyes/backup-git-repos/commit/a50c9a25e6a65c41be2e36e69193f6ccfbbda14c)), closes [#64](https://github.com/alrayyes/backup-git-repos/issues/64)

## [1.4.0](https://github.com/alrayyes/backup-git-repos/compare/v1.3.0...v1.4.0) (2026-08-18)


### Features

* add --dry-run to run to preview what a backup would do ([#60](https://github.com/alrayyes/backup-git-repos/issues/60)) ([7c332c2](https://github.com/alrayyes/backup-git-repos/commit/7c332c23b39acc3e2cacb4878805813e3637d080)), closes [#50](https://github.com/alrayyes/backup-git-repos/issues/50)
* default --config to the XDG config path when not passed ([#56](https://github.com/alrayyes/backup-git-repos/issues/56)) ([a3c4f45](https://github.com/alrayyes/backup-git-repos/commit/a3c4f45079470d6702e86fd9ad57307d7d76cbfc)), closes [#47](https://github.com/alrayyes/backup-git-repos/issues/47)
* skip Forgejo repositories that mirror an external upstream ([#59](https://github.com/alrayyes/backup-git-repos/issues/59)) ([6e0b67b](https://github.com/alrayyes/backup-git-repos/commit/6e0b67be95b429ceaf6576b4c5162c133dbbfb50)), closes [#49](https://github.com/alrayyes/backup-git-repos/issues/49)


### Bug Fixes

* create a repo's destination directory tree before mirroring it ([#53](https://github.com/alrayyes/backup-git-repos/issues/53)) ([05b69e4](https://github.com/alrayyes/backup-git-repos/commit/05b69e47d9efeb124e0d01b64874352a34f74500)), closes [#48](https://github.com/alrayyes/backup-git-repos/issues/48)
* fail listing when a classic GitHub token lacks the repo scope ([#55](https://github.com/alrayyes/backup-git-repos/issues/55)) ([907dfd7](https://github.com/alrayyes/backup-git-repos/commit/907dfd704982a13d027f497decafb1f6d86a91d8)), closes [#51](https://github.com/alrayyes/backup-git-repos/issues/51)

## [1.3.0](https://github.com/alrayyes/backup-git-repos/compare/v1.2.0...v1.3.0) (2026-08-18)


### Features

* publish a multi-arch container image to GHCR on release ([#43](https://github.com/alrayyes/backup-git-repos/issues/43)) ([5bed29b](https://github.com/alrayyes/backup-git-repos/commit/5bed29b501d859b0ac66a5505d7c025dc90d9b11))


### Bug Fixes

* run the container image as non-root, lint it with hadolint ([#45](https://github.com/alrayyes/backup-git-repos/issues/45)) ([51e7d68](https://github.com/alrayyes/backup-git-repos/commit/51e7d685c867a9c8aaf8c8923ea0e7b5a0549d9c))

## [1.2.0](https://github.com/alrayyes/backup-git-repos/compare/v1.1.0...v1.2.0) (2026-08-18)


### Features

* add a progress bar and --verbose logging to run ([#41](https://github.com/alrayyes/backup-git-repos/issues/41)) ([0c9b0f3](https://github.com/alrayyes/backup-git-repos/commit/0c9b0f31c330f9930d9511a17991335e97ed5886)), closes [#28](https://github.com/alrayyes/backup-git-repos/issues/28)
* add config init to write a starter config file ([#39](https://github.com/alrayyes/backup-git-repos/issues/39)) ([87ed249](https://github.com/alrayyes/backup-git-repos/commit/87ed24940cf295f1bcb21628a0ac694d57a9a448)), closes [#26](https://github.com/alrayyes/backup-git-repos/issues/26)
* support a literal token alongside token_env in the config ([#40](https://github.com/alrayyes/backup-git-repos/issues/40)) ([767753a](https://github.com/alrayyes/backup-git-repos/commit/767753ab100b208965063b4ca63a1614cfd3dac1)), closes [#27](https://github.com/alrayyes/backup-git-repos/issues/27)


### Bug Fixes

* **ci:** give the nightly GitLab test suite a 35m timeout ([#24](https://github.com/alrayyes/backup-git-repos/issues/24)) ([540c2a4](https://github.com/alrayyes/backup-git-repos/commit/540c2a493ddeb3a0fff3e240c88e107d6e385e07)), closes [#23](https://github.com/alrayyes/backup-git-repos/issues/23)
* **ci:** run disk cleanup before Set up Go in nightly GitLab job ([#19](https://github.com/alrayyes/backup-git-repos/issues/19)) ([4421ac8](https://github.com/alrayyes/backup-git-repos/commit/4421ac8a42d56e7a7f152f815027ea3e30f89657))
* default concurrency to GOMAXPROCS, not 1 ([#36](https://github.com/alrayyes/backup-git-repos/issues/36)) ([5bbed06](https://github.com/alrayyes/backup-git-repos/commit/5bbed069bd757026cb6d1abedb49bebc25a2d005)), closes [#31](https://github.com/alrayyes/backup-git-repos/issues/31)
* **deps:** bump github.com/stretchr/testify from 1.11.1 to 1.12.0 ([#16](https://github.com/alrayyes/backup-git-repos/issues/16)) ([f49bc3d](https://github.com/alrayyes/backup-git-repos/commit/f49bc3d3f006f25e3516f293eac1c66f3c1092c2))
* don't persist a git mirror for archived repos that get archived ([#38](https://github.com/alrayyes/backup-git-repos/issues/38)) ([4cdd936](https://github.com/alrayyes/backup-git-repos/commit/4cdd9368fd6d585f657a69f370b57f43b2bd6f87)), closes [#30](https://github.com/alrayyes/backup-git-repos/issues/30)
* expand ~ in --dest, --archive-dir, and config dest ([#35](https://github.com/alrayyes/backup-git-repos/issues/35)) ([594e727](https://github.com/alrayyes/backup-git-repos/commit/594e7276f5d530e52a59d245ddcad00eac1a3ad2)), closes [#29](https://github.com/alrayyes/backup-git-repos/issues/29)
* **test:** stop archive test git helpers from leaking hook env into temp repos ([#34](https://github.com/alrayyes/backup-git-repos/issues/34)) ([1707230](https://github.com/alrayyes/backup-git-repos/commit/170723051086ea91527d069b8109ffbb726f35fe)), closes [#33](https://github.com/alrayyes/backup-git-repos/issues/33)


### Performance Improvements

* **test:** disable exporters and unused GitLab CE subsystems in nightly fixture ([#22](https://github.com/alrayyes/backup-git-repos/issues/22)) ([079f5eb](https://github.com/alrayyes/backup-git-repos/commit/079f5ebc15710e5b2ab2df85d3275f138b5d8de8))

## [1.1.0](https://github.com/alrayyes/backup-git-repos/compare/v1.0.0...v1.1.0) (2026-08-16)


### Features

* back up a GitHub.com account ([#12](https://github.com/alrayyes/backup-git-repos/issues/12)) ([251c999](https://github.com/alrayyes/backup-git-repos/commit/251c99934d1c66ea825af4b3cbbbf736dbb6c5dc))

## 1.0.0 (2026-08-15)


### Features

* back up a Forgejo instance ([#2](https://github.com/alrayyes/backup-git-repos/issues/2)) ([25d8a68](https://github.com/alrayyes/backup-git-repos/commit/25d8a6876596196ac04507f0c7276a21b67b345c))
* back up a GitLab instance ([#3](https://github.com/alrayyes/backup-git-repos/issues/3)) ([5eff2a1](https://github.com/alrayyes/backup-git-repos/commit/5eff2a143b4108bd760f30fc5b3dc6f7387ee5aa))
* write archived repos as tar.gz ([#4](https://github.com/alrayyes/backup-git-repos/issues/4)) ([7a450c2](https://github.com/alrayyes/backup-git-repos/commit/7a450c2c0af0c9679cb1b35d8477229f096ba910))
