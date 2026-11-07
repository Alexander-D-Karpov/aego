#!/usr/bin/env python3
"""Run all native Go tests, isolating integration groups without skipping tests.

The Go test timeout applies to an entire package, not to each test. The editor
integration package contains hundreds of independent tests and can exceed that
package-wide limit on hosted machines. Splitting its discovered top-level tests
keeps each default timeout and all assertions, and keeps its performance tests
separate from other simultaneously running packages. Each group is a fresh
process, so groups do not test for accidental global-state dependencies.
"""

import re
import subprocess
import sys

INTEGRATION = "aego/tests/integration"
GROUPS = 4


def command_output(args):
    result = subprocess.run(args, text=True, stdout=subprocess.PIPE, check=True)
    return result.stdout


def packages(output):
    items = output.splitlines()
    if len(items) < 2 or len(items) != len(set(items)) or items.count(INTEGRATION) != 1:
        raise ValueError("go list did not return exactly one integration package")
    if any(not item or any(c.isspace() for c in item) for item in items):
        raise ValueError("go list returned an invalid package name")
    return [item for item in items if item != INTEGRATION]


def tests(output):
    names = []
    for line in output.splitlines():
        if line.startswith(("Test", "Example", "Fuzz")):
            if not line.isidentifier():
                raise ValueError("invalid discovered Go test name: " + line)
            names.append(line)
    if not names or len(names) != len(set(names)):
        raise ValueError("integration test inventory is empty or has duplicates")
    return names


def groups(names, count=GROUPS):
    if count < 1 or not names or len(names) != len(set(names)):
        raise ValueError("cannot partition an empty, duplicate or invalid inventory")
    # Contiguous slices preserve discovery order within each subprocess.
    count = min(count, len(names))
    parts = [names[len(names) * i // count : len(names) * (i + 1) // count] for i in range(count)]
    if [name for part in parts for name in part] != names or any(not part for part in parts):
        raise ValueError("test partition lost or duplicated a test")
    return parts


def test_pattern(part):
    pattern = "^(?:" + "|".join(re.escape(name) for name in part) + ")$"
    # Stay below Windows' command-line size limit; fail rather than drop tests.
    if len(pattern.encode("utf-16-le")) // 2 > 30000:
        raise ValueError("integration group exceeds the Windows command-line budget")
    return pattern


def verify_patterns(parts, patterns, output):
    for part, pattern in zip(parts, patterns):
        actual = tests(output(["go", "test", "-list", pattern, "./tests/integration"]))
        if actual != part:
            raise ValueError("Go did not match the exact planned integration tests")


def run(execute=subprocess.run, output=command_output):
    other = packages(output(["go", "list", "./..."]))
    names = tests(output(["go", "test", "-list", "^(Test|Example|Fuzz)", "./tests/integration"]))
    parts = groups(names)
    patterns = [test_pattern(part) for part in parts]
    verify_patterns(parts, patterns, output)
    print(f"Native test inventory: {len(other) + 1} packages, {len(names)} integration tests in {len(parts)} groups", flush=True)
    result = execute(["go", "test", "-count=1", *other]).returncode
    for i, (part, pattern) in enumerate(zip(parts, patterns), 1):
        print(f"Integration group {i}/{len(parts)}: {len(part)} tests, {part[0]} ... {part[-1]}", flush=True)
        code = execute(["go", "test", "-count=1", "-v", "-run", pattern, "./tests/integration"]).returncode
        if code:
            result = code
    return 1 if result else 0


if __name__ == "__main__":
    try:
        sys.exit(run())
    except (OSError, ValueError, subprocess.CalledProcessError) as exc:
        print(f"Native test inventory failed: {exc}", file=sys.stderr)
        sys.exit(1)
