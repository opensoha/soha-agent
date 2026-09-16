#!/usr/bin/env python3
"""Final-result CLI adapter for supplied-context Soha analysis tasks."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


def main():
    if len(sys.argv) != 2 or len(sys.argv[1]) > 131072:
        raise ValueError('one bounded Soha analysis prompt is required')
    schema = {'type': 'object', 'properties': {
        'summary': {'type': 'string'},
        'contractEcho': {'type': 'string'},
        'markerEcho': {'type': 'string'},
    }, 'required': ['summary', 'contractEcho', 'markerEcho'], 'additionalProperties': False}
    environment = {key: value for key, value in os.environ.items() if key in (
        'HOME', 'PATH', 'LANG', 'TMPDIR', 'HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'NO_PROXY')}
    with tempfile.TemporaryDirectory(prefix='soha-codex-') as directory:
        schema_path = Path(directory) / 'output.json'
        schema_path.write_text(json.dumps(schema))
        command = ['codex', 'exec', '--ignore-user-config', '--ephemeral', '--json',
                   '--skip-git-repo-check', '--sandbox', 'read-only', '--disable', 'shell_tool',
                   '-c', 'tools.view_image=false', '-c', 'web_search="disabled"',
                   '-c', 'features.apps=false', '-c', 'mcp_servers={}',
                   '--output-schema', str(schema_path), '-']
        prompt = sys.argv[1] + '\nUse only the supplied context. Do not call tools. Return contractEcho from contract and markerEcho from input.marker (empty if absent).'
        result = subprocess.run(command, input=prompt, capture_output=True, text=True,
                                cwd=directory, env=environment, timeout=120)
        if result.returncode:
            raise RuntimeError('Codex CLI failed; check local authentication and model availability')
        answer, usage, run_id = None, {}, ''
        for line in result.stdout.splitlines():
            event = json.loads(line)
            if event.get('type') == 'thread.started':
                run_id = event.get('thread_id', '')
            if event.get('type') == 'turn.completed':
                usage = event.get('usage', {})
            item = event.get('item', {})
            if item.get('type') in ('command_execution', 'file_change', 'mcp_tool_call', 'web_search'):
                raise RuntimeError('supplied-context adapter received an unexpected tool event')
            if event.get('type') == 'item.completed' and item.get('type') == 'agent_message':
                answer = json.loads(item['text'])
        if not isinstance(answer, dict) or not answer.get('summary') or not run_id:
            raise RuntimeError('Codex returned no structured final result')
        answer.update(externalRunId=run_id, usage=usage, provider='codex', toolCalls=0)
        print(json.dumps(answer, ensure_ascii=False))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
