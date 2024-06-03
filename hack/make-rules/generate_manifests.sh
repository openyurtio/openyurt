#!/bin/bash
# set -x

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

# Define the directory to search in and the pattern to search for
YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"

CONTROLLER_GEN=$YURT_ROOT/bin/controller-gen

WEBHOOK_DIR=$YURT_ROOT/pkg/yurtmanager/webhook
OUTPUT_DIR=$YURT_ROOT/_output/manifest/auto_generate
PATTERN="+kubebuilder:rbac"
CRD_OPTIONS="crd:crdVersions=v1,maxDescLen=1000"

source "${YURT_ROOT}/hack/lib/complement-rbac.sh"


# 1. generate RBAC yaml files

# 1.1 generate controller RBAC yaml files
echo "Generate RBAC for controllers"

result=$(extract_controller_names)
while IFS= read -r role_name; do
    controller_file_name="${role_name//-/_}.go"
    controller_file_path=$(find $YURT_ROOT -type f -name $controller_file_name)
    # Assuming file_path variable assignment from above
    if [ -n "$controller_file_path" ]; then
        echo "Generate RBAC for $role_name"
        $CONTROLLER_GEN rbac:roleName="${role_name}" paths=$controller_file_path/.. output:rbac:artifacts:config=${OUTPUT_DIR}/rbac && mv ${OUTPUT_DIR}/rbac/role.yaml ${OUTPUT_DIR}/rbac/${role_name}.yaml
    else
        echo "File $controller_file_name not found."
    fi
done <<< "$result"

# 1.2 generate webhook RBAC yaml files
echo "Generate RBAC for webhook"

# Loop through each first sublevel directory
for dir in "$WEBHOOK_DIR"/*/; do
    # Avoid a non-existent directory glob problem
    [ -e "$dir" ] || continue
    
    # Search for files containing the pattern within the current subdirectory
    found_file=$(grep -lR -e "$PATTERN" "$dir" | head -n 1)
    
    if [ ! -z "$found_file" ]; then
        echo "Generate RBAC for $dir"
        # If a matching file is found, extract directory base name
        role_name=$(basename "${dir}")
        $CONTROLLER_GEN rbac:roleName="${role_name}" paths=${dir}/... output:rbac:artifacts:config=${OUTPUT_DIR}/rbac && mv ${OUTPUT_DIR}/rbac/role.yaml ${OUTPUT_DIR}/rbac/${role_name}.yaml
    fi
done

# Loop through ${OUTPUT_DIR}/rbac and generate RoleBinding/ClusterRoleBinding/ServiceAccount
for file in ${OUTPUT_DIR}/rbac/*.yaml; do
    complement_rbac "$file"
done

$CONTROLLER_GEN rbac:roleName=basecontroller paths=$YURT_ROOT/pkg/yurtmanager/controller/... output:rbac:artifacts:config=${OUTPUT_DIR}/rbac && mv ${OUTPUT_DIR}/rbac/role.yaml ${OUTPUT_DIR}/rbac/basecontroller.yaml
$CONTROLLER_GEN rbac:roleName=webhook paths=$YURT_ROOT/pkg/yurtmanager/webhook/... output:rbac:artifacts:config=${OUTPUT_DIR}/rbac && mv ${OUTPUT_DIR}/rbac/role.yaml ${OUTPUT_DIR}/rbac/webhook.yaml
echo "Generate RBAC for base controller/webhook"

# 2. generate CRD/Webhook yaml files
$CONTROLLER_GEN $CRD_OPTIONS webhook paths="./pkg/..." output:crd:artifacts:config=$OUTPUT_DIR/crd  output:webhook:artifacts:config=$OUTPUT_DIR/webhook
# remove empty name crd file
rm -f $OUTPUT_DIR/crd/_.yaml
echo "Generate CRD/Webhook for base controller/webhook"
