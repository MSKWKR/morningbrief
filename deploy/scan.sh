#!/bin/sh
# Build the image, write an HTML vulnerability report, and fail if Trivy
# found HIGH/CRITICAL findings. Mirrors the CI workflow's scan steps.
set -e
cd "$(dirname "$0")/.."
docker build -t morningbrief:scan .

docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v "$(pwd)":/out \
  aquasec/trivy:latest image --format template --template "@/contrib/html.tpl" \
  -o /out/trivy-report.html morningbrief:scan
echo "Report written to trivy-report.html"

docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  aquasec/trivy:latest image --severity HIGH,CRITICAL --exit-code 1 morningbrief:scan
