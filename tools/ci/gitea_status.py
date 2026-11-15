#!/usr/bin/env python3
"""Report trusted GitHub native runs to the corresponding Gitea commit."""

import json
import os
import re
import sys
import urllib.error
import urllib.request

GITHUB_REPOSITORY = "Alexander-D-Karpov/aego"
GITEA_API = "https://git.akarpov.ru/api/v1/repos/sanspie/aego"
JOBS = (
    "windows amd64 native",
    "darwin arm64 native",
    "darwin amd64 native",
    "linux amd64 native",
)


def request_json(url, token, payload=None):
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    headers = {"Authorization": "token " + token, "Accept": "application/json"}
    if data is not None:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=data, headers=headers)
    with urllib.request.urlopen(request, timeout=15) as response:
        body = response.read()
        return json.loads(body) if body else {}


def post_status(token, sha, name, state, target):
    request_json(
        f"{GITEA_API}/statuses/{sha}",
        token,
        {
            "context": "GitHub / " + name,
            "state": state,
            "target_url": target,
            "description": "GitHub-hosted native CI: " + state,
        },
    )


def main(args):
    if args not in (["pending"], ["finished"]):
        raise ValueError("expected pending or finished")
    env = os.environ
    if (
        env.get("GITHUB_SERVER_URL") != "https://github.com"
        or env.get("GITHUB_REPOSITORY") != GITHUB_REPOSITORY
        or env.get("GITHUB_REF") != "refs/heads/master"
        or env.get("GITHUB_EVENT_NAME") not in ("push", "workflow_dispatch")
    ):
        print("Gitea status reporting skipped for an untrusted event")
        return 0
    token = env.get("GITEA_STATUS_TOKEN", "")
    if not token:
        print("::warning::GITEA_STATUS_TOKEN is not set; Gitea statuses were not published")
        return 0
    sha, run_id = env.get("GITHUB_SHA", ""), env.get("GITHUB_RUN_ID", "")
    attempt = env.get("GITHUB_RUN_ATTEMPT", "1")
    if not re.fullmatch(r"[0-9a-fA-F]{40}", sha) or not run_id.isdecimal() or not attempt.isdecimal():
        raise ValueError("invalid GitHub commit or run identifier")
    run_url = f"https://github.com/{GITHUB_REPOSITORY}/actions/runs/{run_id}"
    if args[0] == "pending":
        for name in JOBS:
            post_status(token, sha, name, "pending", run_url)
        return 0

    github_token = env.get("GITHUB_TOKEN", "")
    if not github_token:
        raise ValueError("GITHUB_TOKEN is missing")
    # A rerun of failed jobs may omit the platforms that succeeded on an older
    # attempt. Keep each platform's latest result up to this attempt.
    by_name = {}
    for page in range(1, 21):
        url = f"https://api.github.com/repos/{GITHUB_REPOSITORY}/actions/runs/{run_id}/jobs?filter=all&per_page=100&page={page}"
        jobs = request_json(url, github_token).get("jobs", [])
        for job in jobs:
            name = job.get("name")
            number = job.get("run_attempt", 1)
            if name not in JOBS or not isinstance(number, int) or number > int(attempt):
                continue
            if name not in by_name or number > by_name[name].get("run_attempt", 1):
                by_name[name] = job
        if len(jobs) < 100:
            break
    else:
        raise ValueError("too many GitHub job pages")
    failed = False
    for name in JOBS:
        job = by_name.get(name, {})
        conclusion = job.get("conclusion") if job.get("status") == "completed" else None
        state = "success" if conclusion == "success" else "failure" if conclusion in ("failure", "timed_out", "action_required", "startup_failure") else "error"
        job_id = job.get("id")
        target = f"{run_url}/job/{job_id}" if isinstance(job_id, int) and job_id > 0 else run_url
        post_status(token, sha, name, state, target)
        if conclusion is None:
            failed = True
            print(f"::error::Missing completed native job: {name}")
    return 1 if failed else 0


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv[1:]))
    except urllib.error.HTTPError as exc:
        print(f"::error::CI status API returned HTTP {exc.code}", file=sys.stderr)
        sys.exit(1)
    except (OSError, ValueError) as exc:
        print(f"::error::CI status reporting failed: {type(exc).__name__}", file=sys.stderr)
        sys.exit(1)
