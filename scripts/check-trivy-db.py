#!/usr/bin/env python3
"""Fail when Trivy's databases are missing or more than three days old.

    scripts/check-trivy-db.py <trivy cache dir>

A scan is only as good as its database: the vulnerability database must be
present and fresh, and the Java database too when the scan downloaded one
(images with jars), so a broken or stale download cannot pass.
"""

import datetime
import json
import os
import sys


def main(root: str) -> int:
    now = datetime.datetime.now(datetime.timezone.utc)
    checked = 0
    for name in ("db", "java-db"):
        path = os.path.join(root, name, "metadata.json")
        if not os.path.exists(path):
            if name == "db":
                print(f"::error::no vulnerability database at {path}: the scan did not download one")
                return 1
            continue
        with open(path) as fh:
            updated = json.load(fh)["UpdatedAt"]
        at = datetime.datetime.fromisoformat(updated.replace("Z", "+00:00"))
        age = now - at
        print(f"{name}: updated {updated} ({age.days} days ago)")
        if age > datetime.timedelta(days=3):
            print(f"::error::the {name} database is {age.days} days old (updated {updated})")
            return 1
        checked += 1
    print(f"{checked} database(s) fresh")
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(__doc__)
        sys.exit(2)
    sys.exit(main(sys.argv[1]))
