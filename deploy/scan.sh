#!/bin/sh
# Build the image and fail if Trivy finds HIGH/CRITICAL vulnerabilities.
set -e
cd "$(dirname "$0")/.."
docker build -t morningbrief:scan .
trivy image --severity HIGH,CRITICAL --exit-code 1 morningbrief:scan
