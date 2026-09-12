#!/usr/bin/env bash
set -euo pipefail

# Usage:
# ./docker-build-push.sh latest,v1.0.0

REGISTRY="ghcr.io"
IMAGE_NAME="$(git config --get remote.origin.url | sed -E 's#.*github.com[:/]([^/]+/[^/.]+)(\.git)?#\1#' | tr '[:upper:]' '[:lower:]')"

TAGS="${1:-latest}"

# Build once with first tag
FIRST_TAG="$(echo "$TAGS" | cut -d',' -f1)"
FULL_IMAGE="$REGISTRY/$IMAGE_NAME:$FIRST_TAG"

docker buildx build \
  --load \
  -t "$FULL_IMAGE" \
  .

# Add remaining tags
IFS=',' read -ra TAG_ARRAY <<< "$TAGS"
for tag in "${TAG_ARRAY[@]}"; do
    docker tag "$FULL_IMAGE" "$REGISTRY/$IMAGE_NAME:$tag"
done

# Push all tags
for tag in "${TAG_ARRAY[@]}"; do
    docker push "$REGISTRY/$IMAGE_NAME:$tag"
done