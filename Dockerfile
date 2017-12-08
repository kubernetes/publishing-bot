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

FROM google/debian:jessie
MAINTAINER Chao Xu <xuchao@google.com>
RUN apt-get update \
 && apt-get install -y -qq git=1:2.1.4-2.1+deb8u5 \
 && apt-get install -y -qq mercurial \
 && apt-get install -y -qq ca-certificates wget jq vim tmux bsdmainutils tig \
 && wget https://storage.googleapis.com/golang/go1.9.2.linux-amd64.tar.gz \
 && tar -C /usr/local -xzf go1.9.2.linux-amd64.tar.gz \
 && rm -rf /var/lib/apt/lists/*

ENV GOPATH="/go-workspace"
ENV PATH="${GOPATH}/bin:/usr/local/go/bin:${PATH}"
ENV GIT_COMMITTER_NAME="Kubernetes Publisher"
ENV GIT_COMMITTER_EMAIL="k8s-publishing-bot@users.noreply.github.com"

WORKDIR "/"

ADD _output/publishing-bot /publishing-bot
ADD _output/collapsed-kube-commit-mapper /collapsed-kube-commit-mapper
ADD _output/sync-tags /sync-tags
ADD artifacts/scripts/ /publish_scripts

CMD ["/publishing-bot", "--dry-run", "--token-file=/token"]
