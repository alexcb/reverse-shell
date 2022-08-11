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
    ARG GO_EXTRA_LDFLAGS
    ARG GOARCH
    RUN test -n "$GOOS" && test -n "$GOARCH"
    ARG GOCACHE=/go-cache
    RUN mkdir -p build
    RUN --mount=type=cache,target=$GOCACHE \
        go build \
            -o build/reverseshell-client \
            -ldflags "-X main.Version=$RELEASE_TAG $GO_EXTRA_LDFLAGS" \
            cmd/client/main.go
    RUN --mount=type=cache,target=$GOCACHE \
        go build \
            -o build/reverseshell-server \
            -ldflags "-X main.Version=$RELEASE_TAG $GO_EXTRA_LDFLAGS" \
            cmd/server/main.go
    SAVE ARTIFACT build/reverseshell-client AS LOCAL "build/$GOOS/$GOARCH/reverseshell-client"
    SAVE ARTIFACT build/reverseshell-server AS LOCAL "build/$GOOS/$GOARCH/reverseshell-server"

reverseshell-darwin:
    COPY \
        --build-arg GOOS=darwin \
        --build-arg GOARCH=amd64 \
        --build-arg GO_EXTRA_LDFLAGS= \
        +reverseshell/* ./
    SAVE ARTIFACT ./*

reverseshell-linux:
    BUILD \
        --build-arg GOOS=linux \
        --build-arg GOARCH=amd64 \
        --build-arg GO_EXTRA_LDFLAGS="-linkmode external -extldflags -static" \
        +reverseshell

reverseshell-all:
    COPY +reverseshell-linux/reverseshell ./reverseshell-linux-amd64
    COPY +reverseshell-darwin/reverseshell ./reverseshell-darwin-amd64
    SAVE ARTIFACT ./*

release:
    BUILD +test
    FROM node:13.10.1-alpine3.11
    RUN npm install -g github-release-cli@v1.3.1
    WORKDIR /release
    COPY +reverseshell-linux/reverseshell ./reverseshell-linux-amd64
    COPY +reverseshell-darwin/reverseshell ./reverseshell-darwin-amd64
    ARG RELEASE_TAG
    ARG EARTHLY_GIT_HASH
    ARG BODY="No details provided"
    RUN --secret GITHUB_TOKEN=+secrets/GITHUB_TOKEN test -n "$GITHUB_TOKEN"
    RUN --push \
        --secret GITHUB_TOKEN=+secrets/GITHUB_TOKEN \
        github-release upload \
        --owner alexcb \
        --repo secret-share \
        --commitish "$EARTHLY_GIT_HASH" \
        --tag "$RELEASE_TAG" \
        --name "$RELEASE_TAG" \
        --body "$BODY" \
        ./reverseshell-*
