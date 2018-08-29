#!/bin/bash

# Copyright 2017 The Kubernetes Authors.
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

# This script publishes the latest changes in the ${src_branch} of
# k8s.io/kubernetes/staging/src/${repo} to the ${dst_branch} of
# k8s.io/${repo}.
#
# dependent_k8s.io_repos are expected to be separated by ",",
# e.g., "client-go,apimachinery". We will expand it to
# "repo:commit,repo:commit..." in the future.
#
# ${kubernetes_remote} is the remote url of k8s.io/kubernetes that will be used
# in .git/config in the local checkout of the ${repo}.
#
# is_library indicates is ${repo} is a library.
#
# The script assumes that the working directory is
# $GOPATH/src/k8s.io/${repo}.
#
# The script is expected to be run by other publish scripts.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

if [ ! $# -eq 13 ]; then
    echo "usage: $0 repo src_branch dst_branch dependent_k8s.io_repos required_packages kubernetes_remote subdirectory source_repo_org source_repo_name base_package is_library recursive_delete_pattern skip_tags"
    exit 1
fi

# the target repo
REPO="${1}"
# src branch of k8s.io/kubernetes
SRC_BRANCH="${2:-master}"
# dst branch of k8s.io/${repo}
DST_BRANCH="${3:-master}"
# dependent k8s.io repos
DEPS="${4}"
# required packages that are manually copied completely into vendor/, e.g. k8s.io/code-generator or a sub-package. They must be dependencies as well, either via Go imports or via ${DEPS}.
REQUIRED="${5}"
# Remote url for Kubernetes. If empty, will fetch kubernetes
# from https://github.com/kubernetes/kubernetes.
SOURCE_REMOTE="${6}"
# maps to staging/k8s.io/src/${REPO}
SUBDIR="${7}"
# source repository organization name (eg. kubernetes)
SOURCE_REPO_ORG="${8}"
# source repository name (eg. kubernetes) has to be set for the sync-tags
SOURCE_REPO_NAME="${9}"

shift 9

# base package name (eg. k8s.io)
BASE_PACKAGE="${1-k8s.io}"
# If ${REPO} is a library
IS_LIBRARY="${2}"
# A ls-files pattern like "*/BUILD *.ext pkg/foo.go Makefile"
RECURSIVE_DELETE_PATTERN="${3}"
# Skip syncing tags
SKIP_TAGS="${4}"

readonly REPO SRC_BRANCH DST_BRANCH DEPS REQUIRED SOURCE_REMOTE SOURCE_REPO_ORG SUBDIR SOURCE_REPO_NAME BASE_PACKAGE IS_LIBRARY RECURSIVE_DELETE_PATTERN SKIP_TAGS

SCRIPT_DIR=$(dirname "${BASH_SOURCE}")
source "${SCRIPT_DIR}"/util.sh

echo "Running garbage collection."
git gc --auto
echo "Fetching from origin."
git fetch origin --no-tags
echo "Cleaning up checkout."
git rebase --abort >/dev/null || true
git reset -q --hard
git clean -q -f -f -d
git checkout -q $(git rev-parse HEAD) || true
git branch -D "${DST_BRANCH}" >/dev/null || true
git remote set-head origin -d >/dev/null # this let's filter-branch fail
if git rev-parse origin/"${DST_BRANCH}" &>/dev/null; then
    echo "Switching to origin/${DST_BRANCH}."
    git branch -f "${DST_BRANCH}" origin/"${DST_BRANCH}" >/dev/null
    git checkout -q "${DST_BRANCH}"
else
    # this is a new branch. Create an orphan branch without any commit.
    echo "Branch origin/${DST_BRANCH} not found. Creating orphan ${DST_BRANCH} branch."
    git checkout -q --orphan "${DST_BRANCH}"
    git rm -q --ignore-unmatch -rf .
fi

# sync_repo cherry-picks the commits that change
# k8s.io/kubernetes/staging/src/k8s.io/${REPO} to the ${DST_BRANCH}
sync_repo "${SOURCE_REPO_ORG}" "${SOURCE_REPO_NAME}" "${SUBDIR}" "${SRC_BRANCH}" "${DST_BRANCH}" "${SOURCE_REMOTE}" "${DEPS}" "${REQUIRED}" "${BASE_PACKAGE}" "${IS_LIBRARY}" "${RECURSIVE_DELETE_PATTERN}"

# add tags.
LAST_BRANCH=$(git rev-parse --abbrev-ref HEAD)
LAST_HEAD=$(git rev-parse HEAD)
EXTRA_ARGS=()
PUSH_SCRIPT=../push-tags-${REPO}-${DST_BRANCH}.sh
echo "#!/bin/bash" > ${PUSH_SCRIPT}
chmod +x ${PUSH_SCRIPT}

if [[ -z "${SKIP_TAGS}}" ]]; then
    /sync-tags --prefix "$(echo ${SOURCE_REPO_NAME})-" \
               --commit-message-tag $(echo ${SOURCE_REPO_NAME} | sed 's/^./\L\u&/')-commit \
               --source-remote upstream --source-branch "${SRC_BRANCH}" \
               --push-script ${PUSH_SCRIPT} \
               --dependencies "${DEPS}" \
               -alsologtostderr \
               "${EXTRA_ARGS[@]-}"
    if [ "${LAST_HEAD}" != "$(git rev-parse ${LAST_BRANCH})" ]; then
        echo "Unexpected: branch ${LAST_BRANCH} has diverted to $(git rev-parse HEAD) from ${LAST_HEAD} before tagging."
        exit 1
    fi
fi
git checkout ${LAST_BRANCH}
