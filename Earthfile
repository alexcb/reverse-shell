VERSION 0.6

FROM golang:1.18-alpine3.16

RUN apk add --update --no-cache \
    bash \
    bash-completion \
    binutils \
    ca-certificates \
    coreutils \
    curl \
    findutils \
    g++ \
    git \
    grep \
    less \
    make \
    openssl \
    openssh-keygen \
    shellcheck \
    util-linux

WORKDIR /reverseshell

deps:
    RUN go install golang.org/x/tools/cmd/goimports@latest
    RUN go install golang.org/x/lint/golint@latest
    RUN go install github.com/gordonklaus/ineffassign@latest
    COPY go.mod go.sum .
    RUN go mod download
    SAVE ARTIFACT go.mod AS LOCAL go.mod
    SAVE ARTIFACT go.sum AS LOCAL go.sum

code:
    FROM +deps
    COPY --dir cmd encconn ./

lint:
    FROM +code
    RUN output="$(ineffassign .)" ; \
        if [ -n "$output" ]; then \
            echo "$output" ; \
            exit 1 ; \
        fi
    RUN output="$(goimports -d $(find . -type f -name '*.go' | grep -v \.pb\.go) 2>&1)"  ; \
        if [ -n "$output" ]; then \
            echo "$output" ; \
            exit 1 ; \
        fi
    RUN golint -set_exit_status ./...
    RUN output="$(go vet ./... 2>&1)" ; \
        if [ -n "$output" ]; then \
            echo "$output" ; \
            exit 1 ; \
        fi

reverseshell:
    FROM +code
    ARG RELEASE_TAG="dev"
    ARG GOOS
    ARG GO_EXTRA_LDFLAGS="-linkmode external -extldflags -static"
    ARG GOARCH
    RUN test -n "$GOOS" && test -n "$GOARCH"
    ARG GOCACHE=/go-cache
    RUN mkdir -p build
    RUN --mount=type=cache,target=$GOCACHE \
        go build \
            -o build/reverseshell-client-$GOOS-$GOARCH \
            -ldflags "-X main.Version=$RELEASE_TAG $GO_EXTRA_LDFLAGS" \
            cmd/client/main.go
    RUN --mount=type=cache,target=$GOCACHE \
        go build \
            -o build/reverseshell-server-$GOOS-$GOARCH \
            -ldflags "-X main.Version=$RELEASE_TAG $GO_EXTRA_LDFLAGS" \
            cmd/server/main.go
    SAVE ARTIFACT build/*

reverseshell-linux-amd64:
    COPY (+reverseshell/* \
        --GOOS=linux \
        --GOARCH=amd64 \
        --VARIANT= \
        ) ./
    SAVE ARTIFACT ./*

reverseshell-linux-arm64:
    COPY (+reverseshell/* \
        --GOOS=linux \
        --GOARCH=arm64 \
        --VARIANT= \
        --GO_EXTRA_LDFLAGS= \
        ) ./
    SAVE ARTIFACT ./*

reverseshell-darwin-amd64:
    COPY (+reverseshell/* \
        --GOOS=darwin \
        --GOARCH=amd64 \
        --VARIANT= \
        --GO_EXTRA_LDFLAGS= \
        ) ./
    SAVE ARTIFACT ./*

reverseshell-darwin-arm64:
    COPY (+reverseshell/* \
        --GOOS=darwin \
        --GOARCH=arm64 \
        --VARIANT= \
        --GO_EXTRA_LDFLAGS= \
        ) ./
    SAVE ARTIFACT ./*

reverseshell-all:
    COPY +reverseshell-linux-amd64/* .
    COPY +reverseshell-linux-arm64/* .
    COPY +reverseshell-darwin-amd64/* .
    COPY +reverseshell-darwin-arm64/* .
    RUN ls -la
    SAVE ARTIFACT ./*

for-linux:
    COPY +reverseshell-linux-amd64/* .
    SAVE ARTIFACT reverseshell-client-linux-amd64 AS LOCAL build/linux/amd64/reverseshell-client
    SAVE ARTIFACT reverseshell-server-linux-amd64 AS LOCAL build/linux/amd64/reverseshell-server

for-linux-arm64:
    COPY +reverseshell-linux-arm64/* .
    SAVE ARTIFACT reverseshell-client-linux-arm64 AS LOCAL build/linux/arm64/reverseshell-client
    SAVE ARTIFACT reverseshell-server-linux-arm64 AS LOCAL build/linux/arm64/reverseshell-server

for-darwin:
    COPY +reverseshell-darwin-amd64/* .
    SAVE ARTIFACT reverseshell-client-darwin-amd64 AS LOCAL build/darwin/amd64/reverseshell-client
    SAVE ARTIFACT reverseshell-server-darwin-amd64 AS LOCAL build/darwin/amd64/reverseshell-server

for-darwin-m1:
    COPY +reverseshell-darwin-arm64/* .
    SAVE ARTIFACT reverseshell-client-darwin-arm64 AS LOCAL build/darwin/arm64/reverseshell-client
    SAVE ARTIFACT reverseshell-server-darwin-arm64 AS LOCAL build/darwin/arm64/reverseshell-server

for-all:
    BUILD +for-linux
    BUILD +for-linux-arm64
    BUILD +for-darwin
    BUILD +for-darwin-m1

release:
    FROM node:13.10.1-alpine3.11
    RUN npm install -g github-release-cli@v1.3.1
    WORKDIR /release
    COPY +reverseshell-all/* .
    ARG RELEASE_TAG
    ARG EARTHLY_GIT_HASH
    ARG BODY="No details provided"
    RUN ls -la
    RUN --secret GITHUB_TOKEN test -n "$GITHUB_TOKEN"
    RUN --push \
        --secret GITHUB_TOKEN \
        github-release upload \
        --owner alexcb \
        --repo reverse-shell \
        --commitish "$EARTHLY_GIT_HASH" \
        --tag "$RELEASE_TAG" \
        --name "$RELEASE_TAG" \
        --body "$BODY" \
        ./reverseshell-*
