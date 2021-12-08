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

# host_platform returns the host platform determined by golang
host_platform() {
  echo "$(go env GOHOSTOS)/$(go env GOHOSTARCH)"
}

# Parameters
# $1: binary_name
get_component_name() {
  local yurt_component_name
  if [[ $1 =~ yurtctl ]]
  then
    yurt_component_name="yurtctl-servant"
  elif [[ $1 =~ yurt-node-servant ]];
  then
    yurt_component_name="node-servant"
  else
    yurt_component_name=$1
  fi
  echo $yurt_component_name
}
