#!/usr/bin/env python3
"""Guard manual, non-publishing native artifact builds against stale CI."""

import json
import os
from pathlib import Path
import re
import subprocess
import sys
import urllib.request

import mirror_releases

ROOT = Path(__file__).resolve().parents[2]
REQUIRED_JOBS = {"checks", "native-authoring", "build"}
RUN_URL = "https://git.akarpov.ru/sanspie/aego/actions/runs/"


class GuardError(Exception):
    pass


def validate_context(env, source, checkout):
    required = {
        "GITHUB_ACTIONS": "true",
        "GITHUB_SERVER_URL": "https://github.com",
        "GITHUB_REPOSITORY": "Alexander-D-Karpov/aego",
        "GITHUB_EVENT_NAME": "workflow_dispatch",
        "GITHUB_WORKFLOW": "aego-native-artifacts-only",
        "GITHUB_REF": "refs/heads/master",
        "RUNNER_ENVIRONMENT": "github-hosted",
    }
    if any(env.get(key) != value for key, value in required.items()):
        raise GuardError("artifact builds require the manual GitHub-hosted master workflow")
    version = env.get("AEGO_CI_PACKAGE_VERSION", "")
    if not re.fullmatch(r"(0|[1-9][0-9]{0,4})\.(0|[1-9][0-9]{0,4})\.(0|[1-9][0-9]{0,4})", version):
        raise GuardError("invalid stable package version")
    if any(int(part) > 65535 for part in version.split(".")):
        raise GuardError("package version exceeds the resource limit")
    versions = re.findall(r'^var Version = "([^"\r\n]+)"[ \t\r]*$', source, re.MULTILINE)
    if versions != [version]:
        raise GuardError("package version does not match engine.Version")
    commit = env.get("AEGO_CI_PACKAGE_COMMIT", "")
    if not re.fullmatch(r"[0-9a-f]{40}", commit) or checkout != commit or env.get("GITHUB_SHA") != commit:
        raise GuardError("package commit does not match the event and checkout")
    run_id = env.get("AEGO_GITEA_RUN_ID", "")
    if not re.fullmatch(r"[1-9][0-9]{0,9}", run_id):
        raise GuardError("invalid Gitea run ID")
    return version, commit, run_id


def validate_gitea_run(run, run_id, commit):
    if not isinstance(run, dict) or run.get("link") != "/sanspie/aego/actions/runs/" + run_id:
        raise GuardError("Gitea run identity mismatch")
    source = run.get("commit")
    if not isinstance(source, dict) or source.get("link") != "/sanspie/aego/commit/" + commit:
        raise GuardError("Gitea run commit mismatch")
    branch = source.get("branch")
    if not isinstance(branch, dict) or branch.get("name") != "master" or branch.get("isDeleted") is not False:
        raise GuardError("Gitea run is not on master")
    if run.get("workflowID") != "ci.yml" or run.get("triggerEvent") != "push" or run.get("done") is not True or run.get("status") != "success":
        raise GuardError("Gitea CI has not completed successfully")
    jobs = run.get("jobs")
    if not isinstance(jobs, list) or any(not isinstance(job, dict) for job in jobs):
        raise GuardError("invalid Gitea jobs")
    ids = [job.get("jobId") for job in jobs]
    if any(not isinstance(job_id, str) or not job_id for job_id in ids) or len(ids) != len(set(ids)) or not REQUIRED_JOBS.issubset(ids):
        raise GuardError("Gitea required jobs are missing or ambiguous")
    if any(job.get("status") != "success" for job in jobs if job.get("jobId") in REQUIRED_JOBS):
        raise GuardError("a required Gitea job did not pass")


def fetch_run(run_id):
    request = urllib.request.Request(RUN_URL + run_id, data=b"{}", headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)["state"]["run"]


def check_refs(commit):
    source = mirror_releases.source("branches/master")
    github = mirror_releases.github("branches/master")
    if source.get("commit", {}).get("id") != commit or github.get("commit", {}).get("sha") != commit:
        raise GuardError("master refs do not match the requested commit")


def require_ci(run_id, commit):
    validate_gitea_run(fetch_run(run_id), run_id, commit)
    check_refs(commit)
    if not mirror_releases.ci_ready():
        raise GuardError("the required GitHub and Gitea CI results are not green")
    check_refs(commit)


def git(*args):
    result = subprocess.run(["git", *args], cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if result.returncode:
        raise GuardError("git source inspection failed")
    return result.stdout


def check_clean_source():
    flags = git("ls-files", "-v").splitlines()
    if not flags or any(not row.startswith(b"H ") for row in flags):
        raise GuardError("unexpected Git index flags")
    if git("status", "--porcelain", "--untracked-files=all"):
        raise GuardError("package source is not clean")
    if git("ls-files", "--others", "--ignored", "--exclude-standard", "--", ".", ":(exclude).cache/**"):
        raise GuardError("ignored source files are present")


def main(args):
    if args not in (["--source-only"], ["--require-ci"]):
        raise GuardError("expected --source-only or --require-ci")
    checkout = git("rev-parse", "HEAD").decode("ascii").strip()
    version, commit, run_id = validate_context(os.environ, (ROOT / "engine/config.go").read_text(encoding="utf-8"), checkout)
    check_clean_source()
    if args == ["--require-ci"]:
        require_ci(run_id, commit)
    print("Artifact preflight passed for", version, commit)


if __name__ == "__main__":
    try:
        main(sys.argv[1:])
    except (GuardError, KeyError, OSError, RuntimeError, TypeError, ValueError, subprocess.SubprocessError) as exc:
        print("Artifact preflight failed:", exc, file=sys.stderr)
        sys.exit(1)
