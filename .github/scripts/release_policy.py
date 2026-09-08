"""Strict SemVer release classification; Docker metadata must not infer latest."""

import os
import re
import sys

NUMBER = r"(?:0|[1-9][0-9]*)"
IDENTIFIER = rf"(?:{NUMBER}|[0-9]*[A-Za-z-][0-9A-Za-z-]*)"
SEMVER = re.compile(
    rf"v(?P<version>(?P<major>{NUMBER})\.(?P<minor>{NUMBER})\.{NUMBER}"
    rf"(?:-(?P<pre>{IDENTIFIER}(?:\.{IDENTIFIER})*))?"
    r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?)"
)


def classify(tag):
    match = SEMVER.fullmatch(tag)
    if not match:
        raise ValueError(f"Invalid release tag {tag!r}; use v1.2.3 or v1.2.3-rc.1")
    version = match["version"]
    if len(version) > 114:
        raise ValueError("Release version exceeds the Docker variant tag length limit")
    return {
        "version": version,
        "docker_version": version.replace("+", "_"),
        "minor": f'{match["major"]}.{match["minor"]}',
        "stable": str(match["pre"] is None).lower(),
        "prerelease": str(match["pre"] is not None).lower(),
    }


if __name__ == "__main__":
    try:
        values = classify(sys.argv[1])
    except ValueError as error:
        sys.exit(str(error))
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
        for key, value in values.items():
            print(f"{key}={value}", file=output)
