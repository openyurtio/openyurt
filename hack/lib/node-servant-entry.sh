#!/usr/bin/env sh

# Copyright 2021 The OpenYurt Authors.
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

set -xe

if [ -d "/openyurt" ]; then
    rm -rf /openyurt/usr/local/servant
fi

mkdir -p /openyurt/usr/local/servant
cp /usr/local/bin/node-servant /openyurt/usr/local/servant/

chmod -R +x /openyurt/usr/local/servant/
chroot /openyurt /usr/local/servant/node-servant "$@"
rm -rf /openyurt/usr/local/servant

