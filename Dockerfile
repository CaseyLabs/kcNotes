# syntax=docker/dockerfile:1

# The root Makefile/scripts pass the locked Go image in as the development base.
ARG DEV_BASE_IMAGE

FROM ${DEV_BASE_IMAGE:-golang:1.26.2-trixie} AS dev

# The cleanup at the end removes apt cache files so the image stays smaller.
RUN apt-get update && \
    apt-get install -y --no-install-recommends bash ca-certificates curl git jq make nodejs npm shellcheck tar gzip && \
    rm -rf /var/lib/apt/lists/*

# Create a dedicated unprivileged user instead of running as root. This is a
# safer default for local development and for CI jobs that use this image.
RUN groupadd --gid 10001 app && \
    useradd --uid 10001 --gid 10001 --create-home --home-dir /home/app --shell /usr/sbin/nologin app

# `/workspace` is where repository files will live inside the container.
WORKDIR /workspace
# Some tools look at `$HOME` for configuration and temporary files, so point it
# at the home directory we created for the non-root user.
ENV HOME=/home/app
# All following instructions and the default command run as the `app` user.
USER app:app

COPY . .

CMD ["sh", "-eu", "-c", \
    "printf '%s\n' \
    'kcNotes development image ready.' \
    'Use the root Makefile entrypoints:' \
    '  make build' \
    '  make test' \
    '  make run'"]
