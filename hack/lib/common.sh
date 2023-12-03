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

# get_output_name generates the executable's name. If the $PROJECT_PREFIX
# is set, it subsitutes the prefix of the executable's name with the env,
# otherwise the basename of the target is used
get_output_name() {
    local oup_name=$(canonicalize_target $1)
    PROJECT_PREFIX=${PROJECT_PREFIX:-}
    if [ -z $PROJECT_PREFIX ]; then
        oup_name=${oup_name}
    elif [ "$PROJECT_PREFIX" = "yurt" ]; then
        oup_name=${oup_name}
    else
        oup_name=${oup_name/yurt-/$PROJECT_PREFIX}
        oup_name=${oup_name/yurt/$PROJECT_PREFIX}
    fi
    echo $oup_name
}

# canonicalize_target delete the first four characters when
# target begins with "cmd/"
canonicalize_target() {
    local target=$1
    if [[ "$target" =~ ^cmd/.* ]]; then
        target=${target:4}
    fi

    echo $target
}

# is_build_on_host is used to verify binary build on host or not
is_build_on_host() {
  if [[ "$(go env GOOS)" == "$(go env GOHOSTOS)" && "$(go env GOARCH)" == "$(go env GOHOSTARCH)" ]]; then
      # build binary on the host
      return 0
  else
      # do not build binary on the host
      return 1
  fi
}

# project_info generates the project information and the corresponding value
# for 'ldflags -X' option
project_info() {
    PROJECT_INFO_PKG=${YURT_MOD}/pkg/projectinfo
    echo "-X ${PROJECT_INFO_PKG}.projectPrefix=${PROJECT_PREFIX}"
    echo "-X ${PROJECT_INFO_PKG}.labelPrefix=${LABEL_PREFIX}"
    echo "-X ${PROJECT_INFO_PKG}.gitVersion=${GIT_VERSION}"
    echo "-X ${PROJECT_INFO_PKG}.gitCommit=${GIT_COMMIT}"
    echo "-X ${PROJECT_INFO_PKG}.buildDate=${BUILD_DATE}"
    
    maintainingVersions=$(get_maintained_versions | tr " " ",")
    versionSeparator=","
    echo "-X ${PROJECT_INFO_PKG}.separator=${versionSeparator}"
    echo "-X ${PROJECT_INFO_PKG}.maintainingVersions=${maintainingVersions}"
    echo "-X ${PROJECT_INFO_PKG}.nodePoolLabelKey=${NODEPOOL_LABEL_KEY}"
}

# get_binary_dir_with_arch generated the binary's directory with GOOS and GOARCH.
# eg: ./_output/bin/darwin/arm64/
get_binary_dir_with_arch(){
    echo $1/$(go env GOOS)/$(go env GOARCH)
}

# get openyurt versions we still maintain
# returned versions are separated by space
get_maintained_versions() {
    # we currently maintain latest 3 versions including all their maintained releases, 
    # such as v1.0.0-rc1 v1.0.0 v0.7.0 v0.7.1 v0.6.0 v0.6.1
    MAINTAINED_VERSION_NUM=${MAINTAINED_VERSION_NUM:-3}
    allVersions=$(git for-each-ref refs/tags --sort=authordate | awk '{print $3}' | awk -F '/' '{print $3}')
    latestVersion=$(git for-each-ref refs/tags --sort=authordate | awk 'END{print}' |awk '{print $3}' | awk -F '/' '{print $3}')
    major=$(echo $latestVersion | awk -F '.' '{print $1}')
    major=${major#v}
    minor=$(echo $latestVersion | awk -F '.' '{print $2}')
    versions=""

    for ((cnt=0;cnt<$MAINTAINED_VERSION_NUM;cnt++)); do
        versions+=" "$(echo $allVersions | tr " " "\n" | grep -E "v$major\.$minor\..*")
        if [ $minor -eq 0 ]; then
            major=$[$major-1]
            minor=$(echo $allVersions | tr " " "\n" | grep -E -o "v$major\.[0-9]+\..*" | awk 'END{print}' | awk -F '.' '{print $2}')
        else
            minor=$[$minor-1]
        fi
    done
    echo $versions
}

get_image_tag() {
    tag=$(git describe --abbrev=0 --tags)
    commit=$(git rev-parse HEAD) 
    
    if $(git tag --points-at ${commit}); then
        echo ${tag}-$(echo ${commit} | cut -c 1-7)
    else
        echo ${tag}
    fi
}