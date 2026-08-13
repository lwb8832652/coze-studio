#!/usr/bin/env bash
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


set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
image_ref="ghcr.io/agent-infra/sandbox:latest"
container_name="newx-aio-contract-$$-${RANDOM}"
volume_name="newx-aio-contract-data-$$-${RANDOM}"
container_created=false
volume_created=false

cleanup() {
  if [[ "$container_created" == "true" ]]; then
    docker rm --force "$container_name" >/dev/null 2>&1 || true
  fi
  if [[ "$volume_created" == "true" ]]; then
    if ! docker volume rm "$volume_name" >/dev/null 2>&1; then
      echo "failed to remove probe volume: $volume_name" >&2
      return 1
    fi
  fi
}
trap cleanup EXIT INT TERM

docker pull "$image_ref" >/dev/null

if docker container inspect "$container_name" >/dev/null 2>&1; then
  echo "refusing to reuse existing probe container" >&2
  exit 1
fi
if docker volume inspect "$volume_name" >/dev/null 2>&1; then
  echo "refusing to reuse existing probe volume" >&2
  exit 1
fi
docker volume create "$volume_name" >/dev/null
volume_created=true

docker run --detach --rm --name "$container_name" \
  --security-opt seccomp=unconfined \
  -e OPENSSL_armcap=0 \
  -e WORKSPACE=/mnt/user-data \
  -p 127.0.0.1::8080 \
  --mount "type=volume,src=$volume_name,dst=/mnt/user-data" \
  "$image_ref" >/dev/null
container_created=true

container_json="$(docker container inspect "$container_name")"
python3 - "$container_json" "$image_ref" <<'PY'
import json
import sys

container = json.loads(sys.argv[1])[0]
host = container["HostConfig"]
bindings = host["PortBindings"]
environment = container["Config"]["Env"]

assert container["Config"]["Image"] == sys.argv[2]
assert not host["Privileged"]
assert host["NetworkMode"] != "host"
assert set(bindings) == {"8080/tcp"}
assert len(bindings["8080/tcp"]) == 1
assert bindings["8080/tcp"][0]["HostIp"] == "127.0.0.1"
assert host.get("SecurityOpt") == ["seccomp=unconfined"]
mounts = container.get("Mounts") or []
assert len(mounts) == 1
assert mounts[0]["Type"] == "volume"
assert mounts[0]["Destination"] == "/mnt/user-data"
assert "OPENSSL_armcap=0" in environment
assert "WORKSPACE=/mnt/user-data" in environment
PY

host_binding="$(docker port "$container_name" 8080/tcp)"
host_port="${host_binding##*:}"
if [[ ! "$host_port" =~ ^[0-9]+$ ]]; then
  echo "failed to resolve loopback AIO port" >&2
  exit 1
fi
base_url="http://127.0.0.1:$host_port"

ready=false
for _ in $(seq 1 180); do
  if curl --fail --silent --show-error --max-time 2 "$base_url/v1/ping" >/dev/null 2>&1 && \
    curl --fail --silent --show-error --max-time 2 "$base_url/v1/sandbox" >/dev/null 2>&1; then
    ready=true
    break
  fi
  if [[ "$(docker inspect "$container_name" --format '{{.State.Running}}')" != "true" ]]; then
    break
  fi
  sleep 1
done
if [[ "$ready" != "true" ]]; then
  docker logs --tail 100 "$container_name" >&2 || true
  echo "official latest AIO failed to become ready" >&2
  exit 1
fi

aio_version="$(docker image inspect "$image_ref" --format '{{index .Config.Labels "version"}}')"

cd "$repo_root/backend"
NEWX_AIO_BASE_URL="$base_url" \
NEWX_AIO_VERSION="$aio_version" \
GOCACHE="${GOCACHE:-/private/tmp/coze-go-build}" \
  go run ./cmd/sandbox-aio-compat-probe
