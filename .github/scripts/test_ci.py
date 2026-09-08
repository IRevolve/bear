import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

import yaml

from release_policy import classify

ROOT = Path(__file__).resolve().parents[2]


class ReleasePolicyTest(unittest.TestCase):
    def test_classification(self):
        for tag in ("v1.2.3", "v0.0.0", "v1.2.3+build.12"):
            with self.subTest(tag=tag):
                self.assertEqual(classify(tag)["stable"], "true")
        for tag in ("v1.2.3-rc.1", "v1.2.3-beta.2", "v1.2.3-alpha", "v1.2.3-a",
                    "v1.2.3-alpine", "v1.2.3-debian", "v1.2.3-0+build.1"):
            with self.subTest(tag=tag):
                self.assertEqual(classify(tag)["prerelease"], "true")
        for tag in ("v1.2.3a", "1.2.3", "v01.2.3", "v1.2", "v1.2.3-01",
                    "v1.2.3-rc..1", "v1.2.3+", "v1.2.3\n", "v1.2.3;exit 0",
                    "v1.2.3-" + "a" * 120):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                classify(tag)

    def test_no_alias_or_variant_collisions(self):
        workflow = yaml.safe_load((ROOT / ".github/workflows/release.yml").read_text())
        job = workflow["jobs"]["docker"]
        meta = next(step for step in job["steps"] if step.get("id") == "meta")["with"]
        self.assertEqual(meta["flavor"], "latest=false")
        self.assertNotIn("type=semver", meta["tags"])
        self.assertIn("matrix.prerelease_prefix", meta["tags"])
        self.assertEqual(meta["tags"].count("needs.version.outputs.stable == 'true'"), 3)
        stable_tags = set()
        prerelease_tags = set()
        for variant in job["strategy"]["matrix"]["include"]:
            suffix = variant["suffix"]
            tags = {"1.2.3" + suffix, "1.2" + suffix, "latest" + suffix, variant["alias"]}
            self.assertFalse(tags & stable_tags)
            stable_tags.update(tags)
            for label in ("rc.1", "beta.1", "alpine", "debian"):
                tag = variant["prerelease_prefix"] + "1.2.3-" + label + suffix
                self.assertNotIn(tag, prerelease_tags)
                prerelease_tags.add(tag)
        self.assertFalse(stable_tags & prerelease_tags)
        self.assertTrue({"latest", "latest-alpine", "alpine", "latest-debian", "debian"} <= stable_tags)

    def test_publication_gates(self):
        for filename, publishers in (("release.yml", ("release", "docker")),
                                     ("docs.yml", ("deploy",))):
            jobs = yaml.safe_load((ROOT / ".github/workflows" / filename).read_text())["jobs"]
            self.assertEqual(jobs["verify"]["uses"], "./.github/workflows/ci.yml")
            for publisher in publishers:
                self.assertIn("verify", jobs[publisher]["needs"])
        ci = yaml.safe_load((ROOT / ".github/workflows/ci.yml").read_text())
        # PyYAML uses YAML 1.1, where the GitHub key `on` parses as True.
        self.assertIn("pull_request", ci[True])
        commands = [step.get("run") for step in ci["jobs"]["test"]["steps"]]
        for command in ("go test ./...", "go vet ./...", "go test -race ./..."):
            self.assertIn(command, commands)


class ExampleSyntaxTest(unittest.TestCase):
    def test_jenkins_safety(self):
        for path in (ROOT / "examples").glob("Jenkinsfile*"):
            text = path.read_text()
            self.assertIn("gitUsernamePassword(", text)
            self.assertNotIn("git remote set-url", text)
            self.assertNotIn("git config", text)
            self.assertIn("not { changeRequest() }", text)
            self.assertIn('test "$BRANCH_NAME" = main', text)
            self.assertIn('git check-ref-format --branch "$BRANCH_NAME"', text)
            self.assertIn('bear apply --git-remote origin --git-branch "$BRANCH_NAME"', text)
            self.assertIn("dir('examples')", text)
            for identity in ("GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"):
                self.assertIn(identity, text)
            self.assertNotIn("always { cleanWs() }", text)
        self.assertIn('args \'--entrypoint=""\'', (ROOT / "examples/Jenkinsfile.docker").read_text())

    def test_yaml_json_and_shell(self):
        for directory in (ROOT / ".github/workflows", ROOT / "examples"):
            for path in directory.rglob("*.yml"):
                yaml.safe_load(path.read_text())
        for path in (ROOT / "examples").rglob("package.json"):
            if "node_modules" not in path.parts:
                json.loads(path.read_text())
        scripts = [(ROOT / "examples/ci/skip-lock-only.sh").read_text()]
        for path in (ROOT / "examples").glob("Jenkinsfile*"):
            scripts.extend(re.findall(r"sh\s+'''(.*?)'''", path.read_text(), re.S))
        for path in (ROOT / ".github/workflows").glob("*.yml"):
            workflow = yaml.safe_load(path.read_text())
            for job in workflow["jobs"].values():
                scripts.extend(step["run"] for step in job.get("steps", []) if "run" in step)
        for script in scripts:
            # Expressions are expanded by Actions before invoking the shell.
            script = re.sub(r"\$\{\{.*?\}\}", "example", script)
            subprocess.run(["bash", "-n"], input=script, text=True, check=True)

    def test_documentation_yaml(self):
        for path in (ROOT / "docs").rglob("*.md"):
            for block in re.findall(r"```yaml[^\n]*\n(.*?)```", path.read_text(), re.S):
                yaml.safe_load(block)

    def test_skip_predicate(self):
        # Mock read-only git queries; no commits or repository mutations needed.
        with tempfile.TemporaryDirectory() as directory:
            git = Path(directory) / "git"
            git.write_text('''#!/bin/sh
case "$1" in
  rev-parse) exit "${MISSING_BASE:-0}" ;;
  rev-list)
    if [ "$2" = --count ]; then printf '%s\\n' "${COUNT:-1}";
    else printf '%s\\n' "${PARENTS:-head parent}"; fi ;;
  log)
    case "$3" in
      --format=%ae) printf '%s\\n' "${AUTHOR:-bear-ci@example.com}" ;;
      --format=%ce) printf '%s\\n' "${COMMITTER:-bear-ci@example.com}" ;;
      --format=%s) printf '%s\\n' "${SUBJECT:-chore(bear): update lock file [skip ci]}" ;;
    esac ;;
  diff) printf '%s\\n' "${FILES:-examples/bear.lock.yml}" ;;
  *) exit 2 ;;
esac
''')
            git.chmod(0o700)
            env = {**os.environ, "PATH": directory + os.pathsep + os.environ["PATH"]}
            command = ["sh", str(ROOT / "examples/ci/skip-lock-only.sh"), "previous", "bear-ci@example.com"]
            self.assertEqual(subprocess.run(command, env=env).returncode, 0)
            for overrides in ({"FILES": "examples/bear.lock.yml\nservices/api/main.go"},
                              {"FILES": "services/api/main.go"}, {"AUTHOR": "human@example.com"},
                              {"COMMITTER": "human@example.com"}, {"COUNT": "2"},
                              {"MISSING_BASE": "1"}, {"PARENTS": "merge parent1 parent2"},
                              {"SUBJECT": "source update [skip ci]"}):
                with self.subTest(overrides=overrides):
                    self.assertNotEqual(subprocess.run(command, env={**env, **overrides}).returncode, 0)
            self.assertNotEqual(subprocess.run(command[:2], env=env).returncode, 0)


if __name__ == "__main__":
    unittest.main()
