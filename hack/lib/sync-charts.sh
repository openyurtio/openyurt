#!/bin/bash -l
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

set -e

if [[ -n "$SSH_DEPLOY_KEY" ]]
then
  mkdir -p ~/.ssh
  echo "$SSH_DEPLOY_KEY" > ~/.ssh/id_rsa
  chmod 600 ~/.ssh/id_rsa
fi

echo "git clone"
cd ..
git config --global user.email "openyurt-bot@openyurt.io"
git config --global user.name "openyurt-bot"
git clone --single-branch --depth 1 git@github.com:openyurtio/openyurt-helm.git openyurt-helm

echo "clear openyurt-helm charts/openyurt"

if [ -d "openyurt-helm/charts/openyurt" ]
then
    echo "charts openyurt exists, remove it"
    rm -r openyurt-helm/charts/openyurt/*
else
    mkdir -p openyurt-helm/charts/openyurt
fi

echo "copy folder openyurt/charts to openyurt-helm/charts"

cp -R openyurt/charts/openyurt/* openyurt-helm/charts/openyurt/

echo "push to openyurt-helm"
echo "version: $VERSION, commit: $COMMIT_ID, tag: $TAG"

cd openyurt-helm

if [ -z "$(git status --porcelain)" ]; then
  echo "nothing need to push, finished!"
else
  git add .
  git commit -m "align with openyurt charts $VERSION from commit $COMMIT_ID"
  git tag "$VERSION"
  git push origin main
fi
