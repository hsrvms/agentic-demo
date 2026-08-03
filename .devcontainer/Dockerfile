# syntax=docker/dockerfile:1.7

# Keep runtime versions explicit so upgrades are reviewable and cached independently.
ARG GO_IMAGE=golang:1.26.5-bookworm
ARG NODE_IMAGE=node:24.18.0-bookworm-slim
ARG DEBIAN_IMAGE=debian:bookworm-slim
ARG MINIO_MC_IMAGE=minio/mc:RELEASE.2025-08-13T08-35-41Z

FROM ${GO_IMAGE} AS go-tools
ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=local

ARG AIR_VERSION=v1.67.1
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/air-verse/air@${AIR_VERSION}

ARG GOLANGCI_LINT_VERSION=v2.12.2
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}

ARG DELVE_VERSION=v1.27.0
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION}

ARG SQLC_VERSION=v1.31.1
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION}

ARG MIGRATE_VERSION=v4.19.1
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@${MIGRATE_VERSION}

ARG TEMPL_VERSION=v0.3.1020
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/a-h/templ/cmd/templ@${TEMPL_VERSION}

# gopls is required by Zed's Go language support; staticcheck complements golangci-lint.
ARG GOPLS_VERSION=v0.23.0
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install golang.org/x/tools/gopls@${GOPLS_VERSION}

ARG STATICCHECK_VERSION=2026.1
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION}

FROM ${NODE_IMAGE} AS node-tools
ARG PI_VERSION=0.83.0
ARG TAILWIND_VERSION=4.3.3
RUN npm install --global --ignore-scripts \
        @earendil-works/pi-coding-agent@${PI_VERSION} \
        @tailwindcss/cli@${TAILWIND_VERSION} \
    && npm cache clean --force

FROM ${MINIO_MC_IMAGE} AS minio-client

FROM ${DEBIAN_IMAGE}

ARG USERNAME=dev
ARG USER_UID=1000
ARG USER_GID=1000

ENV DEBIAN_FRONTEND=noninteractive

# Install curl and gpg first so we can add the GitHub CLI apt repository.
RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates curl gpg \
    && mkdir -p /usr/share/keyrings \
    && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        | gpg --dearmor -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update \
    && apt-get install --yes --no-install-recommends \
        bash \
        dnsutils \
        fd-find \
        gcc \
        gh \
        git \
        iproute2 \
        iputils-ping \
        jq \
        less \
        libc6-dev \
        lsof \
        make \
        netcat-openbsd \
        openssh-client \
        postgresql-client \
        procps \
        redis-tools \
        ripgrep \
        unzip \
        vim \
        wget \
    && apt-get remove --yes gpg \
    && apt-get autoremove --yes \
    && rm -rf /var/lib/apt/lists/* \
    && ln -s /usr/bin/fdfind /usr/local/bin/fd \
    && groupadd --gid "${USER_GID}" "${USERNAME}" \
    && useradd --uid "${USER_UID}" --gid "${USER_GID}" --create-home --shell /bin/bash "${USERNAME}" \
    && install -d -o "${USER_UID}" -g "${USER_GID}" /workspace /home/${USERNAME}/.cache \
    && install -d -m 0777 /go/pkg/mod /go/build-cache \
    && install -d -m 0777 /home/${USERNAME}/.npm \
    && printf '%s\n' \
        'export GOPATH=/go' \
        'export GOMODCACHE=/go/pkg/mod' \
        'export GOCACHE=/go/build-cache' \
        'export GOTOOLCHAIN=local' \
        'export PATH=/usr/local/go/bin:/go/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin' \
        > /etc/profile.d/devcontainer.sh

COPY --from=go-tools /usr/local/go/ /usr/local/go/
COPY --from=go-tools /go/bin/ /usr/local/bin/
COPY --from=node-tools /usr/local/ /usr/local/
COPY --from=minio-client /usr/bin/mc /usr/local/bin/mc
COPY .devcontainer/bashrc /home/${USERNAME}/.bashrc
RUN chown ${USER_UID}:${USER_GID} /home/${USERNAME}/.bashrc

ENV DEBIAN_FRONTEND= \
    HOME=/home/${USERNAME} \
    GOPATH=/go \
    GOMODCACHE=/go/pkg/mod \
    GOCACHE=/go/build-cache \
    GOTOOLCHAIN=local \
    BASH_ENV=/home/${USERNAME}/.bashrc \
    ENV=/home/${USERNAME}/.bashrc \
    PATH=/usr/local/go/bin:/go/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin

USER ${USERNAME}
WORKDIR /workspace
CMD ["sleep", "infinity"]
