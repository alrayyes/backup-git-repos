# goreleaser stages the binary it already cross-compiled into this build's
# context and runs `docker build` here -- there's no Go toolchain stage,
# just a runtime image with git on PATH, since Mirror shells out to it the
# same way it does outside a container.
FROM alpine:3.22.2@sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412

# dockers_v2 stages each platform's binary at <goos>/<goarch>/<binary> in the
# build context rather than flat at its root, and buildx populates these
# automatically per --platform target -- they only need declaring to be used.
ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache git=2.49.1-r0 && \
    addgroup -g 1000 backup && \
    adduser -D -u 1000 -G backup backup

COPY ${TARGETOS}/${TARGETARCH}/backup-git-repos /usr/local/bin/backup-git-repos

# UID/GID fixed at 1000 rather than left to adduser's default, since it's
# the caller's own mounted volume that has to be writable by whatever this
# is -- a fixed, documented number is something a Dockerfile line or a
# --user flag can target; "whatever adduser picked this build" isn't.
USER 1000:1000

ENTRYPOINT ["/usr/local/bin/backup-git-repos"]
