#!/bin/bash -l
# Copyright 2024 The OpenYurt Authors.
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
git clone --single-branch --depth 1 git@github.com:openyurtio/charts.git charts

echo "clear charts/yurt-coordinator in openyurtio/charts"

if [ -d "charts/charts/yurt-coordinator" ]
then
    echo "charts yurt-coordinator exists, remove it"
    rm -r charts/charts/yurt-coordinator/*
else
    mkdir -p charts/charts/yurt-coordinator
fi

echo "clear charts/yurt-manager in openyurtio/charts"

if [ -d "charts/charts/yurt-manager" ]
then
    echo "charts yurt-manager exists, remove it"
    rm -r charts/charts/yurt-manager/*
else
    mkdir -p charts/charts/yurt-manager
fi

echo "clear charts/yurthub in openyurtio/charts"

if [ -d "charts/charts/yurthub" ]
then
    echo "charts yurthub exists, remove it"
    rm -r charts/charts/yurthub/*
else
    mkdir -p charts/charts/yurthub
fi

echo "clear charts/yurt-iot-dock in openyurtio/charts"

if [ -d "charts/charts/yurt-iot-dock" ]
then
    echo "charts yurt-iot-dock exists, remove it"
    rm -r charts/charts/yurt-iot-dock/*
else
    mkdir -p charts/charts/yurt-iot-dock
fi

echo "copy folder openyurt/charts to openyurtio/charts/charts"

cp -R openyurt/charts/yurt-coordinator/* charts/charts/yurt-coordinator/
cp -R openyurt/charts/yurt-manager/* charts/charts/yurt-manager/
cp -R openyurt/charts/yurthub/* charts/charts/yurthub/
cp -R openyurt/charts/yurt-iot-dock/* charts/charts/yurt-iot-dock/

echo "push to openyurtio/charts from commit: $COMMIT_ID"

cd charts

if [ -z "$(git status --porcelain)" ]; then
  echo "nothing need to push, finished!"
else
  git add .
  git commit -m "align with openyurt helm charts from commit $COMMIT_ID"
  git push origin main
fi
