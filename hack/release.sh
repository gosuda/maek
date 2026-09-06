#!/usr/bin/env bash
set -euo pipefail

# Determine repository root
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

TAG_ONLY=0
VERSION_ARG=""
BUMP_ARG=""

while [ $# -gt 0 ]; do
    case "$1" in
        --tag-only)
            TAG_ONLY=1
            shift
            ;;
        --bump=*)
            BUMP_ARG="${1#*=}"
            shift
            ;;
        --version=*)
            VERSION_ARG="${1#*=}"
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [--tag-only] [--bump=patch|minor|major] [VERSION]"
            echo ""
            echo "Options:"
            echo "  --tag-only       Only create and push git tag without running GoReleaser"
            echo "  --bump=TYPE      Bump type: patch, minor, major"
            echo "  --version=TAG    Specify release tag directly (e.g. v1.0.0)"
            echo "  -h, --help       Show this help message"
            exit 0
            ;;
        v*|[0-9]*)
            VERSION_ARG="$1"
            shift
            ;;
        *)
            echo "Error: Unknown argument '$1'" >&2
            exit 1
            ;;
    esac
done

TARGET_VERSION="${VERSION_ARG:-${VERSION:-}}"
BUMP_TYPE="${BUMP_ARG:-${BUMP:-}}"

prompt_user() {
    local prompt_msg="$1"
    if [ -r /dev/tty ]; then
        printf "%s" "$prompt_msg" > /dev/tty
        read -r USER_INPUT < /dev/tty
    else
        printf "%s" "$prompt_msg"
        read -r USER_INPUT
    fi
}

# 1. Verify clean git working tree
if [ -n "$(git status --porcelain)" ]; then
    echo "Error: Working tree is dirty. Please commit or stash changes before releasing." >&2
    exit 1
fi

HEAD_TAG="$(git tag --points-at HEAD | head -n 1)"
TAG=""

# 2. Check if a version was explicitly specified
if [ -n "$TARGET_VERSION" ]; then
    TAG="$TARGET_VERSION"
    case "$TAG" in
        v*) ;;
        *) TAG="v$TAG" ;;
    esac
elif [ -n "$HEAD_TAG" ]; then
    echo "==> Current commit is already tagged as $HEAD_TAG"
    prompt_user "Use existing tag $HEAD_TAG? [Y/n]: "
    case "$USER_INPUT" in
        [nN]*)
            HEAD_TAG=""
            ;;
        *)
            TAG="$HEAD_TAG"
            ;;
    esac
fi

# 3. Calculate latest tag and potential bump versions
if [ -z "$TAG" ]; then
    LATEST="$(git describe --tags --abbrev=0 2>/dev/null || echo "")"
    if [ -z "$LATEST" ]; then
        DISP="(none)"
        PATCH="v0.1.0"
        MINOR="v0.2.0"
        MAJOR="v1.0.0"
    else
        DISP="$LATEST"
        CLEAN_VER="${LATEST#v}"
        MAJOR_NUM="$(echo "$CLEAN_VER" | cut -d. -f1)"
        MINOR_NUM="$(echo "$CLEAN_VER" | cut -d. -f2)"
        PATCH_NUM="$(echo "$CLEAN_VER" | cut -d. -f3 | cut -d- -f1)"

        PATCH="v${MAJOR_NUM}.${MINOR_NUM}.$((PATCH_NUM + 1))"
        MINOR="v${MAJOR_NUM}.$((MINOR_NUM + 1)).0"
        MAJOR="v$((MAJOR_NUM + 1)).0.0"
    fi

    if [ -n "$BUMP_TYPE" ]; then
        case "$BUMP_TYPE" in
            patch) TAG="$PATCH" ;;
            minor) TAG="$MINOR" ;;
            major) TAG="$MAJOR" ;;
            *)
                echo "Error: Unknown bump type '$BUMP_TYPE' (expected patch, minor, or major)" >&2
                exit 1
                ;;
        esac
    else
        echo ""
        echo "Current release version: $DISP"
        echo "Select the next version to release:"
        echo "  1) Patch  -> $PATCH  [default]"
        echo "  2) Minor  -> $MINOR"
        echo "  3) Major  -> $MAJOR"
        echo "  4) Custom"
        echo "  5) Cancel"
        prompt_user "Choose [1-5]: "
        case "$USER_INPUT" in
            2) TAG="$MINOR" ;;
            3) TAG="$MAJOR" ;;
            4)
                prompt_user "Enter custom version tag (e.g. v1.0.0): "
                TAG="$USER_INPUT"
                ;;
            5)
                echo "Release canceled."
                exit 0
                ;;
            *) TAG="$PATCH" ;;
        esac
        case "$TAG" in
            v*) ;;
            *) TAG="v$TAG" ;;
        esac
    fi
fi

# 4. Confirmation prompt
echo ""
ACTION_LABEL="releasing"
if [ "$TAG_ONLY" -eq 1 ]; then
    ACTION_LABEL="tagging"
fi

prompt_user "Confirm ${ACTION_LABEL} ${TAG}? [Y/n]: "
case "$USER_INPUT" in
    [nN]*)
        echo "Aborted."
        exit 0
        ;;
esac

# 5. Create git tag if needed and push
if [ "$TAG" != "$HEAD_TAG" ]; then
    echo "==> Creating git tag $TAG..."
    git tag -a "$TAG" -m "Release $TAG"
    echo "==> Pushing tag $TAG to origin..."
    git push origin "$TAG" || echo "Warning: Failed to push tag to origin (working offline or no remote permissions)"
fi

if [ "$TAG_ONLY" -eq 1 ]; then
    echo "==> Tag $TAG successfully processed."
    exit 0
fi

# 6. Execute GoReleaser
if [ -z "${GITHUB_TOKEN:-}" ]; then
    echo "==> GITHUB_TOKEN not set locally. Building release artifacts with --skip=publish..."
    echo "==> Note: If tag was pushed to GitHub, GitHub Actions will publish the release automatically."
    go tool goreleaser release --skip=publish --clean
else
    echo "==> GITHUB_TOKEN detected. Publishing release..."
    go tool goreleaser release --clean
fi
