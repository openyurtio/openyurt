#!/bin/bash

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

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
CONTROLLER_NAME_FILE=$YURT_ROOT/cmd/yurt-manager/names/controller_names.go


complement_rbac() {
    local input_file="$1" # Accept input_file as an argument
    local tmp_file="${input_file}.tmp"
    local namespace="kube-system"
    local service_accounts=() # Indexed array for service account names
    
    # Check if the last line of the input file is "---"
    if [[ "$(tail -n1 "$input_file")" != "---" ]]; then
        # Temporarily append "---" to the file content during the processing
        exec 3< <(cat "$input_file" && echo "---")
    else
        # Use the file directly if it already ends with "---"
        exec 3<"$input_file"
    fi
    cp "$input_file" "$tmp_file"
    local kind name # Define variables to hold 'kind' and 'name'
    
    # Process the file line by line
    while IFS= read -r line <&3; do
        # Extract 'kind'
        if [[ "$line" =~ ^kind:\ (.*) ]]; then
            kind=${BASH_REMATCH[1]}
        fi
        # Extract 'name'
        if [[ "$line" =~ ^[[:space:]]*name:\ (.*) ]]; then
            name=${BASH_REMATCH[1]}
            # Add the service account name if it does not already exist in the array
            if [[ ! " ${service_accounts[*]} " =~ " ${name} " ]]; then
                service_accounts+=("$name")
            fi
        fi
        # End of a document, check if kind is Role or ClusterRole
        if [[ "$line" == "---" ]] && [ -n "$kind" ] && [ -n "$name" ]; then
            local binding_kind="RoleBinding"
            local namespace_metadata="  namespace: $namespace"
            if [ "$kind" = "ClusterRole" ]; then
                binding_kind="ClusterRoleBinding"
                namespace_metadata="" # ClusterRoleBinding doesn't have a namespace in its metadata
            fi
            # Generate RoleBinding or ClusterRoleBinding YAML
            {
                echo "---
apiVersion: rbac.authorization.k8s.io/v1
kind: $binding_kind
metadata:
  name: ${name}-binding
$namespace_metadata
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: $kind
  name: $name
subjects:
- kind: ServiceAccount
  name: $name
  namespace: $namespace
"
            } >> "$tmp_file"
            echo "Complement ${binding_kind} for ${name}"
            # Reset variables for the next object
            kind=""
            name=""
        fi
    done
    
    # Generate ServiceAccount YAML for each unique name
    for sa_name in "${service_accounts[@]}"; do
        {
            echo "---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: $sa_name
  namespace: $namespace
"
        } >> "$tmp_file"

        echo "Complement ServiceAccount for $sa_name"
    done
    # Close file descriptor 3
    exec 3<&-

    mv "$tmp_file" "$input_file"
}

# extract controller names from CONTROLLER_NAME_FILE
extract_controller_names() {
    awk -F'"' '/= "/ { print $2 }' "$CONTROLLER_NAME_FILE"
}

# How to use:
# complement_rbac "_output/manifest/auto_generate/rbac/yurtcoordinator.yaml"

# result=$(extract_controller_names)
# while IFS= read -r line; do
#     echo "Extracted value: $line"
# done <<< "$result"

# result=$(extract_controller_names)
# while IFS= read -r line; do
#     controller_file_name="${line//-/_}.go"
#     controller_file_path=$(find $YURT_ROOT -type f -name $controller_file_name)
#     # Assuming file_path variable assignment from above
#     if [ -n "$controller_file_path" ]; then
#         echo "Found: $controller_file_path"
#     else
#         echo "File $controller_file_name not found."
#     fi
# done <<< "$result"

