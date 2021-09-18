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

FROM registry.erda.cloud/retag/debian:stretch
MAINTAINER Stefan Schimanski <sttts@redhat.com>
MAINTAINER Jeremy Wu <hoozecn@gmail.com>
RUN sed -ri 's/[a-z0-9]+?\.debian\.org/mirrors.aliyun.com/' /etc/apt/sources.list
RUN apt-get update \
 && apt-get install -y -qq git \
 && apt-get install -y -qq ca-certificates curl wget jq vim tmux bsdmainutils tig gcc zip \
 && rm -rf /var/lib/apt/lists/*

ENV GOPATH="/go-workspace"
ENV GOROOT="/go-workspace/go"
ENV PATH="${GOPATH}/bin:/go-workspace/go/bin:${PATH}"
ENV GIT_COMMITTER_NAME="Erda Publisher"
ENV GIT_COMMITTER_EMAIL="erda-publishing-bot@users.noreply.github.com"
ENV TERM=xterm
ENV PS1='\h:\w\$'
ENV SHELL=/bin/bash

WORKDIR "/"

ADD _output/publishing-bot /bin/publishing-bot
ADD _output/collapsed-kube-commit-mapper /bin/collapsed-kube-commit-mapper
ADD _output/sync-tags /bin/sync-tags
ADD _output/init-repo /bin/init-repo

ADD _output/gomod-zip /bin/gomod-zip
ADD artifacts/scripts/ /publish_scripts

CMD ["/publishing-bot", "--dry-run", "--token-file=/token"]
