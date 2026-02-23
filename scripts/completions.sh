#!/bin/sh
set -e

rm -rf completions
mkdir -p completions

for shell in bash zsh fish; do
  go run ./cmd/dash0 completion "$shell" > "completions/dash0.$shell"
done
