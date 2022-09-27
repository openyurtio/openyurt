#!/usr/bin/env bash

# Copyright 2020 The OpenYurt Authors.
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

set -x
set -e
set -u

function installGolangToEdge {
    local goroot="/usr/local/go"
    local gopath="/home/openyurt/workspace/golang"
    local golangCodeSource="https://storage.googleapis.com/golang/go1.16.linux-amd64.tar.gz"
    local goFile="go1.16.tar.gz"

    echo "Begin install golang"
    mkdir -p $goroot
    mkdir -p $gopath
    wget -O ${goFile} ${golangCodeSource}
    tar -zxf ${goFile} -C "/usr/local"

#   set goroot&gopath env
    local etcProfile="/etc/profile"
    local exportGoroot="export GOROOT=${goroot}"
    cat <<EOF > ${etcProfile}
    export GOPATH=${gopath}
    export PATH=${goroot}/bin:${gopath}/bin:${PATH}
EOF

    source $etcProfile
#    set goproxy env
    local goproxy="https://goproxy.cn"
    go env -w GOPROXY=${goproxy},direct
}
installGolangToEdge