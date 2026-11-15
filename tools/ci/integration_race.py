#!/usr/bin/env python3
"""Run all integration Test/Example/Fuzz seeds under -race in bounded groups.

The normal 30-minute Go timeout is retained for each group. The binary is
compiled once into a new evidence directory, never into the checkout root.
Skips retain Go's ordinary exit semantics but are reported as UNVERIFIED.
"""
import argparse
from collections import Counter
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys
import tempfile
import time
import traceback

from worktree_manifest import inventory

PACKAGE = 'aego/tests/integration'
TEST_NAME = re.compile(r'^(?:Test|Example|Fuzz)\w*$')
GROUP_COUNT = 8
GO_TIMEOUT = '30m'


def names_from(text):
    names = [line.strip() for line in text.splitlines() if TEST_NAME.fullmatch(line.strip())]
    if not names or len(names) != len(set(names)):
        raise ValueError('empty or duplicate top-level test enumeration')
    return sorted(names)


def groups_for(names):
    if not names or names != sorted(set(names)):
        raise ValueError('groups require the complete sorted unique test enumeration')
    return [names[i::GROUP_COUNT] for i in range(min(len(names), GROUP_COUNT))]


def selector(names):
    if not names:
        raise ValueError('an empty selector must not accidentally run the entire package')
    return '^(' + '|'.join(re.escape(name) for name in names) + ')$'


def check_events(events, expected):
    selected = set(expected)
    starts, ends = Counter(), Counter()
    results, outputs, problems = {}, {}, []
    for event in events:
        if not isinstance(event, dict) or event.get('Package') != PACKAGE:
            problems.append('unexpected event package/schema')
            continue
        name, action = event.get('Test', ''), event.get('Action', '')
        if name and name.split('/', 1)[0] not in selected:
            problems.append('unselected test executed: ' + name)
        if name and event.get('Output'):
            outputs.setdefault(name, []).append(event['Output'])
        if name and '/' not in name and action == 'run':
            starts[name] += 1
        if name and action in ('pass', 'fail', 'skip'):
            if name in results:
                problems.append('duplicate terminal result: ' + name)
            results[name] = action.upper()
            if '/' not in name:
                ends[name] += 1
    for name in expected:
        if starts[name] != 1 or ends[name] != 1:
            problems.append(f'{name}: expected one start and terminal result, got {starts[name]}/{ends[name]}')
    skipped = [{'name': name, 'result': 'SKIP', 'acceptance': 'UNVERIFIED', 'reasonOutput': ''.join(outputs.get(name, []))}
               for name, result in results.items() if result == 'SKIP']
    return {'coverageComplete': not problems, 'problems': problems,
            'topLevel': {name: results.get(name, 'NOT_RUN') for name in expected},
            'failures': [name for name, result in results.items() if result == 'FAIL'], 'skipped': skipped}


def write_json(path, data):
    with path.open('x', encoding='utf-8') as stream:
        json.dump(data, stream, indent=2, ensure_ascii=True)
        stream.write('\n')


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def command(argv, cwd, log, timeout=None, stdin=None, check=True):
    write_json(log.with_suffix('.command.json'), {'argv': argv, 'cwd': str(cwd)})
    with log.open('xb') as output:
        proc = subprocess.Popen(argv, cwd=cwd, stdin=stdin, stdout=output, stderr=subprocess.STDOUT)
        try:
            code = proc.wait(timeout=timeout)
        except BaseException as error:
            # This is the Popen handle spawned above, never an arbitrary PID
            # lookup. Incomplete execution is never a successful group.
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait(timeout=10)
            if isinstance(error, subprocess.TimeoutExpired):
                log.with_suffix('.exit').write_text('TIMEOUT\n')
                raise RuntimeError(f'owned process exceeded deadline: {argv[0]}') from error
            raise
    log.with_suffix('.exit').write_text(str(code) + '\n')
    if code != 0 and check:
        raise RuntimeError(f'command exited {code}: {argv[0]}; see {log}')
    return code


def run_group(binary, names, cwd, directory):
    directory.mkdir()
    argv = [str(binary), '-test.v=test2json', '-test.paniconexit0=true', '-test.count=1',
            '-test.timeout=' + GO_TIMEOUT, '-test.run=' + selector(names)]
    result = {'selected': names, 'status': 'FAIL', 'timeout': GO_TIMEOUT}
    start = time.monotonic()
    try:
        # Convert the output even on failure: otherwise the artifact contains
        # the raw log but the report loses the identity of the failed test.
        test_exit = command(argv, cwd, directory / 'test.log', timeout=30 * 60 + 45, check=False)
        with (directory / 'test.log').open('rb') as raw, (directory / 'events.jsonl').open('xb') as output, (directory / 'converter.log').open('xb') as errors:
            converted = subprocess.run(['go', 'tool', 'test2json', '-t', '-p', PACKAGE], cwd=cwd,
                                       stdin=raw, stdout=output, stderr=errors, timeout=60)
        result['converterExitCode'] = converted.returncode
        if converted.returncode != 0:
            raise RuntimeError('go tool test2json failed')
        with (directory / 'events.jsonl').open(encoding='utf-8') as stream:
            result.update(check_events((json.loads(line) for line in stream), names))
        if test_exit == 0 and result['coverageComplete'] and not result['failures']:
            result['status'] = 'COMPLETED_WITH_SKIPS' if result['skipped'] else 'PASS'
        elif test_exit != 0:
            result['error'] = f'test binary exited {test_exit}; see {directory / "test.log"}'
    except Exception as error:
        result['error'] = f'{type(error).__name__}: {error}'
        result['traceback'] = traceback.format_exc()
    result['elapsedSeconds'] = round(time.monotonic() - start, 6)
    exit_file = directory / 'test.exit'
    if exit_file.is_file():
        exit_text = exit_file.read_text().strip()
        result['testExitCode'] = int(exit_text) if re.fullmatch(r'-?\d+', exit_text) else exit_text
    else:
        result['testExitCode'] = 'UNVERIFIED'
    for name in ('test.log', 'test.exit', 'events.jsonl', 'converter.log'):
        if (directory / name).is_file():
            result[name + 'SHA256'] = digest(directory / name)
    write_json(directory / 'result.json', result)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument('--evidence', type=Path)
    parser.add_argument('--allow-unborn', action='store_true')
    parser.add_argument('--require-no-skip', action='store_true')
    args = parser.parse_args()
    root = args.root.resolve(strict=True)
    if args.evidence:
        requested = args.evidence.absolute()
        parent = requested.parent.resolve(strict=True)
        evidence = parent / requested.name
        if evidence == root or root in evidence.parents:
            rel = evidence.relative_to(root).as_posix()
            if subprocess.run(['git', '-C', str(root), 'check-ignore', '-q', '--', rel]).returncode != 0:
                raise SystemExit('evidence inside checkout must be ignored')
        evidence.mkdir()
    else:
        parent = root / '.cache/integration-race'
        parent.mkdir(parents=True, exist_ok=True)
        evidence = Path(tempfile.mkdtemp(prefix='run-', dir=parent))
    temporary = evidence / 'tmp'
    temporary.mkdir(mode=0o700)
    os.environ['TMPDIR'] = str(temporary)
    # Running scenes may contain music. Always use the dummy audio device.
    os.environ['SDL_AUDIO_DRIVER'] = 'dummy'
    os.environ['SDL_AUDIODRIVER'] = 'dummy'
    report = {'schema': 1, 'package': PACKAGE, 'status': 'FAIL', 'race': True,
              'count': 1, 'groupCount': GROUP_COUNT, 'timeoutPerGroup': GO_TIMEOUT, 'groups': [],
              'runner': platform.platform(),
              'limits': ['Every ordinary Test/Example/Fuzz seed runs once; skips are UNVERIFIED.',
                         'Benchmark bodies are not part of go test and are not run here.']}
    (evidence / 'exit').write_text('RUNNING\n')
    code = 1
    try:
        before = inventory(root, allow_unborn=args.allow_unborn)
        write_json(evidence / 'source-before.json', before)
        report.update(sourceFingerprint=before['worktreeSHA256'], head=before['head'], audio='dummy', tempDirectory=str(temporary))
        command(['go', 'version'], root, evidence / 'go-version.log')
        report['goVersion'] = (evidence / 'go-version.log').read_text().strip()
        command(['go', 'env', 'GOOS', 'GOARCH', 'CGO_ENABLED', 'GOFLAGS'], root, evidence / 'go-env.log')
        command(['go', 'test', '-race', '-count=1', '-list', '^(Test|Example|Fuzz)', './tests/integration'], root, evidence / 'go-list.log')
        names = names_from((evidence / 'go-list.log').read_text(errors='replace'))
        binary = evidence / 'integration.test'
        command(['go', 'test', '-race', '-c', '-o', str(binary), './tests/integration'], root, evidence / 'build.log')
        report['testBinarySHA256'] = digest(binary)
        command([str(binary), '-test.list=^(Test|Example|Fuzz)'], root / 'tests/integration', evidence / 'binary-list.log')
        if names != names_from((evidence / 'binary-list.log').read_text(errors='replace')):
            raise RuntimeError('Go and compiled binary enumerate different tests')
        groups = groups_for(names)
        write_json(evidence / 'groups.json', groups)
        for i, group in enumerate(groups, 1):
            result = run_group(binary, group, root / 'tests/integration', evidence / f'group-{i:02d}')
            report['groups'].append(result)
            detail = ''
            if result.get('failures'):
                detail = '; failed=' + ', '.join(result['failures'])
            elif result['status'] == 'FAIL':
                detail = '; ' + result.get('error', 'incomplete test coverage')
            print(f'group {i}/{len(groups)}: {result["status"]}; tests={len(group)}{detail}', flush=True)
        after = inventory(root, allow_unborn=args.allow_unborn)
        write_json(evidence / 'source-after.json', after)
        report['sourceUnchanged'] = before == after
        report['coverageComplete'] = all(g.get('coverageComplete', False) for g in report['groups'])
        report['skipped'] = [skip for g in report['groups'] for skip in g.get('skipped', [])]
        passed = all(g['status'] in ('PASS', 'COMPLETED_WITH_SKIPS') for g in report['groups'])
        if report['sourceUnchanged'] and report['coverageComplete'] and passed:
            report['status'] = 'COMPLETED_WITH_SKIPS' if report['skipped'] else 'PASS'
            code = 1 if args.require_no_skip and report['skipped'] else 0
    except Exception as error:
        report['error'] = f'{type(error).__name__}: {error}'
        report['traceback'] = traceback.format_exc()
        print(report['error'], file=sys.stderr)
    except KeyboardInterrupt:
        code = 130
        report['status'] = 'INTERRUPTED'
    report['exitCode'] = code
    write_json(evidence / 'summary.json', report)
    (evidence / 'exit').write_text(str(code) + '\n')
    print(f'{report["status"]}: {evidence}', flush=True)
    return code


if __name__ == '__main__':
    raise SystemExit(main())
