#!/bin/bash

# Copyright 2023 The Kubernetes Authors.
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
set -o xtrace

# This script generates the config and rules required for testing the master branch of k/k
# with publishing bot

BOT_CONFIG_DIRECTORY="${1:-bot-configs}"

mkdir "${BOT_CONFIG_DIRECTORY}"

## generate the required config
# use the content from configmap in the data section
sed -e '1,/config: |/d' configs/kubernetes-configmap.yaml > "${BOT_CONFIG_DIRECTORY}"/config
# The additional .tmp extension is used after -i to make it portable across *BSD and GNU.
#   Ref: https://unix.stackexchange.com/a/92907
# Also \t is not recognized in non GNU sed implementation. Therefore a tab is used as is.
# remove leading white spaces from the generated file
sed -i.tmp 's/^[     ]*//' "${BOT_CONFIG_DIRECTORY}"/config
# remove the github-issue key
sed -i.tmp '/github-issue/d' "${BOT_CONFIG_DIRECTORY}"/config
# set dry run to true
sed -i.tmp -e 's/dry-run: false/dry-run: true/g' "${BOT_CONFIG_DIRECTORY}"/config

## generate the required rules
# get the rules file from the k/k repo
wget https://raw.githubusercontent.com/kubernetes/kubernetes/master/staging/publishing/rules.yaml -O "${BOT_CONFIG_DIRECTORY}"/rules
# change permission so that yq container can make changes to the rules file
chmod 666 "${BOT_CONFIG_DIRECTORY}"/rules
# only work on master branch
# yq is used to remove non master branch related rules
docker run \
    --rm \
    -v "${PWD}/${BOT_CONFIG_DIRECTORY}":/workdir \
    mikefarah/yq:4.32.2 -i 'del( .rules.[].branches.[] | select (.name != "master"))' rules
