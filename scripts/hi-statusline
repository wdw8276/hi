#!/usr/bin/env python3
# hi-statusline — standalone Claude Code statusline script
#
# Usage:
#   { "statusLine": { "type": "command", "command": "hi-statusline" } }
#
# Displays: directory | project_dir | effort | model | context | cache hit | cost

import sys, json

sys.stdout.reconfigure(encoding='utf-8')


def main():
    raw = sys.stdin.read()
    try:
        data = json.loads(raw)
    except Exception:
        data = {}

    # --- Basic fields ---
    cwd = (data.get('workspace') or {}).get('current_dir', '') or data.get('cwd', '') or ''
    project_dir = (data.get('workspace') or {}).get('project_dir', '') or ''
    model_info = data.get('model') or {}
    model = model_info.get('display_name', '') or model_info.get('id', '') or ''

    effort_info = data.get('effort') or {}
    effort_level = effort_info.get('level', '') or ''

    # --- Context window ---
    ctx = data.get('context_window') or {}
    used_pct = ctx.get('used_percentage')
    ctx_size = ctx.get('context_window_size', 0)
    current_usage = ctx.get('current_usage') or {}
    cu_input = current_usage.get('input_tokens', 0)
    cu_cache_create = current_usage.get('cache_creation_input_tokens', 0)
    cu_cache_read = current_usage.get('cache_read_input_tokens', 0)
    # Official definition: input tokens only (excludes output_tokens)
    current_tokens_sum = cu_input + cu_cache_create + cu_cache_read
    if current_tokens_sum > 0:
        current_tokens = current_tokens_sum
    elif used_pct is not None and ctx_size > 0:
        current_tokens = int(round(used_pct / 100 * ctx_size))
    else:
        current_tokens = None

    # --- Cost ---
    cost_info = data.get('cost') or {}
    cost = cost_info.get('total_cost_usd')

    # --- Session cache hit rate from transcript ---
    transcript_path = data.get('transcript_path', '')
    if transcript_path:
        transcript_path = transcript_path.replace('\\', '/')

    cache_hit_pct = None
    if transcript_path:
        seen_ids = set()
        acc_input = acc_create = acc_read = 0
        try:
            with open(transcript_path, encoding='utf-8', errors='replace') as f:
                for line in f:
                    try:
                        obj = json.loads(line)
                    except Exception:
                        continue
                    if obj.get('type') == 'progress':
                        continue
                    msg = obj.get('message') or {}
                    msg_id = msg.get('id', '')
                    if msg_id:
                        if msg_id in seen_ids:
                            continue
                        seen_ids.add(msg_id)
                    usage = _extract_usage(obj)
                    if usage:
                        acc_input += usage.get('input_tokens', 0)
                        acc_create += usage.get('cache_creation_input_tokens', 0)
                        acc_read += usage.get('cache_read_input_tokens', 0)
        except Exception:
            pass
        acc_total = acc_input + acc_create + acc_read
        if acc_total > 0:
            cache_hit_pct = acc_read / acc_total * 100

    # --- Format ---
    def fmt_k(n):
        return f"{n/1000:.1f}k" if n >= 1000 else str(n)

    if cwd:
        short_cwd = cwd.replace('\\', '/').rstrip('/').split('/')[-1] or cwd
    else:
        short_cwd = '?'

    if project_dir:
        proj = project_dir.replace('\\', '/').rstrip('/').split('/')[-1] or project_dir
    else:
        proj = ''

    if used_pct is not None:
        pct_int = int(used_pct)
        if current_tokens is not None and ctx_size > 0:
            compact_win = int(round(current_tokens / used_pct * 100)) if used_pct > 0 else ctx_size
            ctx_str = f"ctx:{fmt_k(current_tokens)}/{fmt_k(ctx_size)} | compact:{pct_int}%({fmt_k(compact_win)})"
        else:
            ctx_str = f"ctx:{pct_int}%"
    else:
        ctx_str = "ctx:--"

    cost_str = f"${cost:.3f}" if cost is not None else ""
    cache_str = f"cache:{cache_hit_pct:.0f}%" if cache_hit_pct is not None else ""

    # --- Output ---
    parts = []
    if short_cwd:
        parts.append(f"\033[36m📁 {short_cwd}\033[0m")
    if proj and proj != short_cwd:
        parts.append(f"\033[90m📂 {proj}\033[0m")
    if effort_level:
        parts.append(f"\033[35m🔧 {effort_level}\033[0m")
    if model:
        parts.append(f"\033[33m🤖 {model}\033[0m")
    parts.append(f"\033[32m🧠 {ctx_str}\033[0m")
    if cache_str:
        parts.append(f"\033[96m⚡ {cache_str}\033[0m")
    if cost_str:
        parts.append(f"\033[35m💰 {cost_str}\033[0m")

    sys.stdout.write(" | ".join(parts))


def _extract_usage(obj):
    """Recursively find the deepest usage dict with cache fields."""
    if isinstance(obj, dict):
        if 'cache_read_input_tokens' in obj and 'input_tokens' in obj:
            return obj
        for v in obj.values():
            r = _extract_usage(v)
            if r:
                return r
    elif isinstance(obj, list):
        for v in obj:
            r = _extract_usage(v)
            if r:
                return r
    return None


if __name__ == "__main__":
    main()
