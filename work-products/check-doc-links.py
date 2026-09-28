import os, re, sys
files = [
 "README.md","PRODUCT.md","ARCHITECTURE.md","TODO.md",
 "docs/operations.md","docs/revisions.md","docs/runtime.md",
 "docs/tui.md","docs/trackers.md",
 "internal/core/skill/workspace/SKILL.md",".agents/skills/workspace/SKILL.md",
]
root = os.getcwd()
bad = []
checked = 0
link_re = re.compile(r'\]\(([^)]+)\)')
for f in files:
    base = os.path.dirname(os.path.join(root, f))
    with open(os.path.join(root,f), encoding="utf-8") as fh:
        text = fh.read()
    for m in link_re.finditer(text):
        target = m.group(1).strip()
        if target.startswith(("http://","https://","mailto:","#")) or target.startswith("data:"):
            continue
        path = target.split("#",1)[0]
        if not path:
            continue
        checked += 1
        resolved = os.path.normpath(os.path.join(base, path))
        if not os.path.exists(resolved):
            bad.append(f"{f}: {target}")
print(f"checked={checked} broken={len(bad)}")
for b in bad:
    print("BROKEN", b)
sys.exit(1 if bad else 0)
