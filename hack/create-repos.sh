#!/usr/bin/env bash

# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")
source "${SCRIPT_ROOT}/repos.sh"

if [ "$#" = 0 ] || [ "$#" -gt 2 ]; then
    echo "usage: $0 [source-github-user-name] dest-github-user-name"
    echo
    echo "This connects to git@github.com:<from>/<repo>. Set GITHUB_HOST to access git@<GITHUB_HOST>:<from>/<repo> instead."
    exit 1
fi

FROM="kubernetes"
TO="${1}"
if [ "$#" -ge 2 ]; then
    FROM="${TO}"
    TO="${2}"
fi
GITHUB_HOST=${GITHUB_HOST:-github.com}

repo_count=${#repos[@]}

# safety check
if [ "${TO}" = "kubernetes" ]; then
    echo "Cannot operate on kubernetes directly" 1>&2
    exit 1
fi

destination_repos=( $(curl -ks https://api.github.com/orgs/${TO}/repos | jq ".[].name" | tr -d '"') )
destination_repo_count=${#destination_repos[@]}

if ! command -v gh > /dev/null; then
  echo "Can't find 'gh' tool in PATH, please install from https://github.com/cli/cli"
  exit 1
fi

# Checks if you are logged in. Will error/bail if you are not.
gh auth status

echo "======================="
echo " create repos if needed"
echo "======================="
for (( i=0; i<${repo_count}; i++ )); do
    found=0
    for (( j=0; j<${destination_repo_count}; j++ )); do
        if [[ "${repos[i]}" == ${destination_repos[j]} ]]; then
            found=1
        fi
    done
    if [[ $found -eq 1 ]]; then
        echo "repository found: ${repos[i]}"
    else
        echo "repository not found: ${repos[i]}"
        gh repo fork "kubernetes/${repos[i]}" --org "${TO}" --remote --clone=false
    fi
done
