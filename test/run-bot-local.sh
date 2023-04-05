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

# This script expects a config and rules file in the BOT_CONFIG_DIRECTORY which will be used for running
# the publishing bot
# The image gcr.io/k8s-staging-publishing-bot/k8s-publishing-bot:latest should be available locally
# in the docker daemon

BOT_CONFIG_DIRECTORY="${1:-bot-configs}"

# create the docker volumes
docker volume create local-go-workspace && docker volume create cache

docker run --rm \
    --pull=never \
    -v local-go-workspace:/go-workspace \
    -v cache:/.cache \
    -v "${PWD}/${BOT_CONFIG_DIRECTORY}":/etc/bot-configs \
    gcr.io/k8s-staging-publishing-bot/k8s-publishing-bot:latest \
    /init-repo \
        --alsologtostderr \
        --config=/etc/bot-configs/config \
        --rules-file=/etc/bot-configs/rules

docker run --rm \
    --pull=never \
    -v local-go-workspace:/go-workspace \
    -v cache:/.cache \
    -v "${PWD}/${BOT_CONFIG_DIRECTORY}":/etc/bot-configs \
    gcr.io/k8s-staging-publishing-bot/k8s-publishing-bot:latest \
    /publishing-bot \
        --alsologtostderr \
        --config=/etc/bot-configs/config \
        --rules-file=/etc/bot-configs/rules \
        --dry-run=true

# cleanup the docker volumes
docker volume rm local-go-workspace && docker volume rm cache
