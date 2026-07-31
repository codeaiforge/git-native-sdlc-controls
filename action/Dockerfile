# The action is a wrapper around the same binary the CLI ships — no logic here.
#
# The build context is the repository root (see action.yml, which lives there for
# exactly this reason), so the image builds the working tree rather than fetching
# a published module. `docker build -f action/Dockerfile .` from the repo root
# reproduces what GitHub does.
FROM golang:1.22-alpine AS build

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/sdlc-controls ./cmd/sdlc-controls

FROM alpine:3.20
# git is the only runtime dependency: the engine reads the diff, nothing else.
RUN apk add --no-cache git

# The workspace is mounted from the host and owned by another uid, which git
# refuses to read as a repository. This is the config-only form of
# `git config --global --add safe.directory '*'`, so the entrypoint stays the
# bare binary and the action keeps no logic of its own.
ENV GIT_CONFIG_COUNT=1 \
    GIT_CONFIG_KEY_0=safe.directory \
    GIT_CONFIG_VALUE_0=*

COPY --from=build /out/sdlc-controls /usr/local/bin/sdlc-controls
ENTRYPOINT ["/usr/local/bin/sdlc-controls"]
