#!/bin/bash
set -e

IMPORT_DIR=/tmp/alpine-import
DS_DIR=~/.docksmith

mkdir -p $DS_DIR/images $DS_DIR/layers $DS_DIR/cache

echo "Reading manifest..."
cat $IMPORT_DIR/manifest.json

python3 - << 'PYEOF'
import json, os, shutil, hashlib

import_dir = "/tmp/alpine-import"
ds_dir = os.path.expanduser("~/.docksmith")

os.makedirs(f"{ds_dir}/images", exist_ok=True)
os.makedirs(f"{ds_dir}/layers", exist_ok=True)

with open(f"{import_dir}/manifest.json") as f:
    manifest = json.load(f)

layers = []
for layer in manifest["layers"]:
    digest = layer["digest"]
    hex_part = digest.replace("sha256:", "")
    src = f"{import_dir}/{hex_part}"
    dst = f"{ds_dir}/layers/sha256:{hex_part}.tar"
    if os.path.exists(src):
        shutil.copy2(src, dst)
        size = os.path.getsize(dst)
        layers.append({"digest": digest, "size": size, "createdBy": "alpine base layer"})
        print(f"Imported layer {hex_part[:12]}... ({size} bytes)")
    else:
        print(f"WARNING: layer file not found: {src}")

config_digest = manifest["config"]["digest"].replace("sha256:", "")
config_path = f"{import_dir}/{config_digest}"
with open(config_path) as f:
    config = json.load(f)

env = config.get("config", {}).get("Env") or []
cmd = config.get("config", {}).get("Cmd") or []
workdir = config.get("config", {}).get("WorkingDir") or ""

img_manifest = {
    "name": "alpine",
    "tag": "3.18",
    "digest": "",
    "created": "2024-01-01T00:00:00Z",
    "config": {
        "Env": env,
        "Cmd": cmd,
        "WorkingDir": workdir
    },
    "layers": layers
}

tmp = dict(img_manifest)
tmp["digest"] = ""
canon = json.dumps(tmp, separators=(',', ':'), sort_keys=False)
digest = "sha256:" + hashlib.sha256(canon.encode()).hexdigest()
img_manifest["digest"] = digest

out_path = f"{ds_dir}/images/alpine:3.18.json"
with open(out_path, "w") as f:
    json.dump(img_manifest, f, indent=2)

print(f"Written: {out_path}")
print(f"Digest:  {digest}")
PYEOF
