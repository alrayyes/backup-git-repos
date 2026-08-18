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

RUN apk add --no-cache git=2.49.1-r0

COPY ${TARGETOS}/${TARGETARCH}/backup-git-repos /usr/local/bin/backup-git-repos

ENTRYPOINT ["/usr/local/bin/backup-git-repos"]
