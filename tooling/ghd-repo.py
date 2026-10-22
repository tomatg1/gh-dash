#!/usr/bin/env python3
"""ghd-repo - manage gh-dash's per-repo tabs.

The "All (org)" section is the single source of truth for the filter. Every
per-repo tab is regenerated from it, so editing that one line and running
`ghd-repo sync` propagates the change (window, sort, author) to every tab.

  ghd-repo ls                list managed repo tabs
  ghd-repo add <repo>...     add repo tab(s), kept alphabetical
  ghd-repo rm  <repo>...     remove repo tab(s)
  ghd-repo sync              regenerate tabs from the "All (org)" filter

Repos may be given bare (`deploys`) or qualified (`Org/deploys`).
Config: $GH_DASH_CONFIG, else ~/.config/gh-dash/config.yml
Writes a .bak first and restores it if the result isn't valid YAML.

gh-dash has no hot reload: press q, then `ghd`, to pick up changes.
"""

import os
import re
import shutil
import subprocess
import sys

CONFIG = os.environ.get("GH_DASH_CONFIG") or os.path.expanduser(
    "~/.config/gh-dash/config.yml"
)

BEGIN = re.compile(r"^\s*#\s*>>> ghd-repo:begin")
END = re.compile(r"^\s*#\s*>>> ghd-repo:end")
TITLE = re.compile(r"^\s*-\s*title:\s*(.+?)\s*$")
FILTERS = re.compile(r"^\s*filters:\s*(.+?)\s*$")
ORG = re.compile(r"\borg:([A-Za-z0-9._-]+)")
ALL_TAB = "All (org)"


def die(msg):
    print("ghd-repo: " + msg, file=sys.stderr)
    sys.exit(1)


def read_lines():
    if not os.path.exists(CONFIG):
        die("no config at " + CONFIG)
    with open(CONFIG) as fh:
        return fh.read().splitlines()


def bounds(lines):
    begin = end = None
    for i, line in enumerate(lines):
        if BEGIN.match(line):
            begin = i
        elif END.match(line):
            end = i
    if begin is None or end is None or end <= begin:
        die(
            "managed-block markers not found - expected '# >>> ghd-repo:begin' "
            "... '# >>> ghd-repo:end' inside prSections"
        )
    return begin, end


def all_filter(lines):
    """The 'All (org)' section's filter string - the source of truth."""
    for i, line in enumerate(lines):
        m = TITLE.match(line)
        if m and m.group(1) == ALL_TAB:
            for j in range(i + 1, min(i + 4, len(lines))):
                f = FILTERS.match(lines[j])
                if f:
                    return f.group(1)
    die("could not find the '%s' section's filters: line" % ALL_TAB)


def split_org(flt):
    """Peel the org: token off, keeping the rest of the filter verbatim.

    Verbatim matters: `created:>={{ nowModify "-1w" }}` contains spaces, so the
    filter cannot be naively tokenized on whitespace.
    """
    m = ORG.search(flt)
    if not m:
        die("'%s' filter has no org: token - cannot derive per-repo filters" % ALL_TAB)
    org = m.group(1)
    tail = (flt[: m.start()] + flt[m.end() :]).strip()
    return org, re.sub(r"\s{2,}", " ", tail)


def managed(lines, begin, end):
    return [
        m.group(1) for m in (TITLE.match(l) for l in lines[begin + 1 : end]) if m
    ]


def render(org, tail, repos):
    return "\n\n".join(
        "  - title: {r}\n    filters: repo:{o}/{r} {t}".format(r=r, o=org, t=tail)
        for r in sorted(repos, key=str.lower)
    )


def valid_yaml():
    if not shutil.which("ruby"):
        return True  # nothing to validate with; assume caller knows
    return (
        subprocess.run(
            ["ruby", "-ryaml", "-e", "YAML.load_file(ARGV[0])", CONFIG],
            capture_output=True,
        ).returncode
        == 0
    )


def write(lines, begin, end, body):
    new = lines[: begin + 1] + (body.split("\n") if body else []) + lines[end:]
    backup = CONFIG + ".bak"
    shutil.copy2(CONFIG, backup)
    with open(CONFIG, "w") as fh:
        fh.write("\n".join(new) + "\n")
    if not valid_yaml():
        shutil.copy2(backup, CONFIG)
        die("generated YAML was invalid - restored backup, nothing changed")


def main(argv):
    if not argv or argv[0] in ("-h", "--help", "help"):
        print(__doc__.strip())
        return 0

    cmd = argv[0]
    args = [a.split("/")[-1].strip() for a in argv[1:]]

    lines = read_lines()
    begin, end = bounds(lines)
    org, tail = split_org(all_filter(lines))
    repos = managed(lines, begin, end)

    if cmd == "ls":
        for r in sorted(repos, key=str.lower):
            print("%s/%s" % (org, r))
        return 0

    if cmd == "add":
        if not args:
            die("add needs at least one <repo>")
        repos = sorted(set(repos) | set(args), key=str.lower)
    elif cmd == "rm":
        if not args:
            die("rm needs at least one <repo>")
        unknown = [a for a in args if a not in repos]
        if unknown:
            die("not managed: " + ", ".join(unknown))
        repos = [r for r in repos if r not in args]
    elif cmd != "sync":
        die("unknown command '%s' (ls | add | rm | sync)" % cmd)

    write(lines, begin, end, render(org, tail, repos))
    listing = ", ".join(sorted(repos, key=str.lower)) or "(none)"
    print("ghd-repo: %d repo tab(s): %s" % (len(repos), listing))
    print("restart gh dash (press q, then `ghd`) - gh-dash has no hot reload")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
