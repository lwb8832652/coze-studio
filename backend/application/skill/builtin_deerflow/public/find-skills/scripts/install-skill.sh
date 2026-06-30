#!/bin/bash
#
# Copyright 2025 coze-dev Authors
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
#


# Install a skill and link it to the project's skills/custom directory
# Usage: ./skills/install-skill.sh <owner/repo@skill-name>
# Example: ./skills/install-skill.sh vercel-labs/agent-skills@vercel-react-best-practices

set -e

if [[ -z "$1" ]]; then
  echo "Usage: $0 <owner/repo@skill-name>"
  echo "Example: $0 vercel-labs/agent-skills@vercel-react-best-practices"
  exit 1
fi

FULL_SKILL_NAME="$1"

# Extract skill name (the part after @)
SKILL_NAME="${FULL_SKILL_NAME##*@}"

if [[ -z "$SKILL_NAME" || "$SKILL_NAME" == "$FULL_SKILL_NAME" ]]; then
  echo "Error: Invalid skill format. Expected: owner/repo@skill-name"
  exit 1
fi

# Find project root by looking for deer-flow.code-workspace
find_project_root() {
  local dir="$PWD"
  while [[ "$dir" != "/" ]]; do
    if [[ -f "$dir/deer-flow.code-workspace" ]]; then
      echo "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  echo ""
  return 1
}

PROJECT_ROOT=$(find_project_root)

if [[ -z "$PROJECT_ROOT" ]]; then
  echo "Error: Could not find project root (deer-flow.code-workspace not found)"
  exit 1
fi

SKILL_SOURCE="$HOME/.agents/skills/$SKILL_NAME"
SKILL_TARGET="$PROJECT_ROOT/skills/custom"

# Step 1: Install the skill using npx
npx skills add "$FULL_SKILL_NAME" -g -y > /dev/null 2>&1

# Step 2: Verify installation
if [[ ! -d "$SKILL_SOURCE" ]]; then
  echo "Skill '$SKILL_NAME' installation failed"
  exit 1
fi

# Step 3: Create symlink
mkdir -p "$SKILL_TARGET"
ln -sf "$SKILL_SOURCE" "$SKILL_TARGET/"

echo "Skill '$SKILL_NAME' installed successfully"
