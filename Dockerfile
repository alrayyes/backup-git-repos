# goreleaser stages the binary it already cross-compiled into this build's
# context and runs `docker build` here -- there's no Go toolchain stage,
# just a runtime image with git on PATH, since Mirror shells out to it the
# same way it does outside a container.
FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

# dockers_v2 stages each platform's binary at <goos>/<goarch>/<binary> in the
# build context rather than flat at its root, and buildx populates these
# automatically per --platform target -- they only need declaring to be used.
ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache git=2.54.0-r0 && \
    addgroup -g 1000 backup && \
    adduser -D -u 1000 -G backup backup

COPY ${TARGETOS}/${TARGETARCH}/backup-git-repos /usr/local/bin/backup-git-repos

# UID/GID fixed at 1000 rather than left to adduser's default, since it's
# the caller's own mounted volume that has to be writable by whatever this
# is -- a fixed, documented number is something a Dockerfile line or a
# --user flag can target; "whatever adduser picked this build" isn't.
USER 1000:1000

ENTRYPOINT ["/usr/local/bin/backup-git-repos"]
